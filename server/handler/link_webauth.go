// WebAuth 认证端点：通过浏览器完成完整认证管道。
//
// 流程概览：
//   1. 客户端弹出浏览器 → SAML v2 XML → /web-auth/sp/login → SPA
//   2. WebAuthStart：证书自动识别组，或返回组列表
//   3. WebAuthSelectGroup：SSO 首步立即执行，否则返回凭据输入
//   4. WebAuthStep：提交凭据/OTP，推进管道（首次用 Authenticate，恢复用 Resume）
//   5. webAuthHandleResult 统一出口：
//      StepPass → 签发完成标记 + 门户 Cookie → /web-auth/complete
//      StepFail → 错误 JSON
//      StepPending → OTP/Radius/SSO 各自的 UI 状态
//   6. SSO 子流程：webAuthBuildSSOURL → 直接跳转 → OAuth 回调(302) → WebAuthContinue
//   7. WebAuthComplete → 设 acSamlv2Token Cookie → /+CSCOE+/saml_ac_login.html

package handler

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

// AnyConnect 弹出系统浏览器打开认证页面 sso-v2-login URL 指向 WebAuth 认证地址
func handlerWebAuth(w http.ResponseWriter, r *http.Request, cr *ClientRequest, ua *dbdata.UserActLog) {
	// 手机端 SSO 组：无法在内置浏览器完成企微/飞书扫码，拒绝
	if cr.GroupSelect != "" && authsrv.GetSSOType(cr.GroupSelect) != "" && isMobileDevice(r) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	state := GenerateSessionID()

	// 构建 Context，保存客户端证书信息到 TLS 字段（供后续证书自动认证恢复）
	certTLS := r.TLS

	pending := &AuthSession{
		UserActLog: ua,
		Ctx: &auth.Context{
			WebAuth: true,
			Conn: auth.ConnInfo{
				GroupName:   cr.GroupSelect,
				RemoteAddr:  r.RemoteAddr,
				UserAgent:   cr.UserAgent,
				MacAddr:     cr.MacAddressList.MacAddress,
				DeviceID:    cr.DeviceId.UniqueIdGlobal,
				DeviceType:  cr.DeviceId.DeviceType,
				PlatformVer: cr.DeviceId.PlatformVersion,
				TLS:         certTLS,
			},
		},
	}
	AuthSessionManager.Save(state, pending)

	serverAddr := getServerAddr(r)

	loginURL := fmt.Sprintf("%s/+CSCOE+/web-auth/sp/login?state=%s", serverAddr, url.QueryEscape(state))
	completeURL := serverAddr + "/+CSCOE+/saml_ac_login.html"

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<config-auth client="vpn" type="auth-request" aggregate-auth-version="2">
    <opaque is-for="sg">
        <tunnel-group>` + cr.GroupSelect + `</tunnel-group>
        <group-alias>` + cr.GroupSelect + `</group-alias>
        <aggauth-handle>168179266</aggauth-handle>
        <config-hash>1595829378234</config-hash>
        <auth-method>single-sign-on-v2</auth-method>
    </opaque>
    <auth id="main">
        <title>SAML SSO Login</title>
        <message>请完成SAML单点登录认证</message>
        <banner></banner>
        <sso-v2-login>` + loginURL + `</sso-v2-login>
        <sso-v2-login-final>` + completeURL + `</sso-v2-login-final>
        <sso-v2-token-cookie-name>acSamlv2Token</sso-v2-token-cookie-name>
        <sso-v2-browser-mode>` + webAuthBrowserMode(r) + `</sso-v2-browser-mode>
        <form>
            <input type="sso" name="sso-token"></input>
        </form>
    </auth>
</config-auth>`

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(xml))
}

// SAML SP 登录入口，302 到 SPA 前端。
func WebAuthSPLogin(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "缺少认证参数", http.StatusBadRequest)
		return
	}

	// 校验 state 有效
	if _, err := AuthSessionManager.Get(state); err != nil {
		base.Error("[WebAuth-2:sp/login] 会话不存在或已过期: state=", state)
		http.Error(w, "认证会话已过期", http.StatusBadRequest)
		return
	}

	redirectURL := "/ui/#/web-auth?state=" + url.QueryEscape(state)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// Web 认证入口：检查证书自动识别组，或返回可选组列表。
func WebAuthStart(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableWebAuth {
		http.NotFound(w, r)
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		webAuthError(w, "缺少认证参数")
		return
	}

	// 校验 state 有效
	pending, err := AuthSessionManager.Get(state)
	if err != nil {
		webAuthError(w, "认证会话已过期")
		return
	}

	// 证书自动识别组（从原始 AnyConnect 连接继承的证书信息）
	// 仅当存在启用 cert 认证的组时才尝试从证书恢复身份
	var certCN, certOU string
	var certTLS *tls.ConnectionState
	if dbdata.AnyGroupHasCertAuth() {
		certCN, certOU, certTLS = webAuthRecoverCert(pending)
	}

	// 回退到组选择流程，让用户可切换组或重试。
	attemptCertAuto := certCN != "" && certOU != "" && certTLS != nil &&
		authsrv.CertAutoAuth(certOU)

	certErrMsg := ""
	if attemptCertAuto {
		pending.Ctx.Conn.Username = certCN
		pending.Ctx.Conn.GroupName = certOU
		pending.UserActLog.Username = certCN
		pending.UserActLog.GroupName = certOU

		result := authsrv.Authenticate(pending.Ctx)
		if result.Result != auth.StepFail {
			webAuthHandleResult(w, r, state, pending, result, certCN)
			return
		}
		// 证书自动认证失败：回退到组选择流程，清除本次临时写入的用户名
		pending.Ctx.Conn.Username = ""
		pending.UserActLog.Username = ""
		certErrMsg = "证书自动认证失败，请选择其他组登录"
		base.Info("[WebAuth-1:start] 证书自动认证失败，回退组选择 ou=", certOU, " err=", result.Err)
	}

	groups := dbdata.GetGroupNamesNormal()
	resp := map[string]any{
		"status": "select_group",
		"groups": groups,
	}
	if certErrMsg != "" {
		resp["message"] = certErrMsg
	}
	// 开启组过滤开关时，先要求输入用户名，再按所属组过滤可选组清单
	// 关闭（默认）则直接展示全部启用组
	if base.GetCfg().EnableWebAuthGroupFilter {
		resp["require_identify"] = true
	}
	webAuthJSON(w, http.StatusOK, resp)
}

// 选定组并执行认证管道。首步非 SSO 时返回凭据输入界面。
func WebAuthSelectGroup(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		webAuthError(w, "缺少认证参数")
		return
	}

	pending, err := AuthSessionManager.Get(state)
	if err != nil {
		webAuthError(w, "认证会话已过期")
		return
	}

	var req struct {
		Group    string `json:"group"`
		Username string `json:"username"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Group == "" {
		webAuthError(w, "请选择用户组")
		return
	}

	groupData := &dbdata.Group{}
	if err := dbdata.One("Name", req.Group, groupData); err != nil {
		if dbdata.CheckErrNotFound(err) {
			webAuthError(w, "用户组不存在")
			return
		}
		webAuthError(w, "系统繁忙，请稍后重试")
		return
	}

	pending.Ctx.Conn.GroupName = req.Group
	if req.Username != "" {
		pending.Ctx.Conn.Username = req.Username
		pending.UserActLog.Username = req.Username
	}
	pending.UserActLog.GroupName = req.Group
	AuthSessionManager.Save(state, pending)

	profile, pErr := auth.ParseAuthProfile(groupData.AuthProfile)
	if pErr != nil || len(profile.Step) == 0 {
		webAuthError(w, "该组未配置认证方式")
		return
	}

	firstStepType := profile.Step[0].Type

	// SSO 认证：立即运行管道获取跳转地址
	if auth.Registry.IsSSOType(firstStepType) {
		// 手机端内置浏览器无法完成企微/飞书扫码，默认拒绝；开启 allow_mobile_sso 后放行
		if isMobileDevice(r) && !base.GetCfg().AllowMobileSSO {
			webAuthError(w, "手机端不支持企微/飞书扫码认证，请选择其他组或联系管理员")
			return
		}
		ctx := &auth.Context{
			Conn: auth.ConnInfo{
				Username:   req.Username,
				GroupName:  req.Group,
				RemoteAddr: r.RemoteAddr,
				UserAgent:  r.UserAgent(),
				TLS:        r.TLS,
			},
		}
		// 浏览器侧无证书，须从会话恢复
		if _, _, selCertTLS := webAuthRecoverCert(pending); selCertTLS != nil {
			ctx.Conn.TLS = selCertTLS
		}
		result := authsrv.Authenticate(ctx)
		pending.Ctx = ctx
		webAuthHandleResult(w, r, state, pending, result, req.Username)
		return
	}

	// 纯证书组（或证书为首步）：从会话恢复 TLS 证书立即运行管道
	if firstStepType == "cert" {
		_, _, selCertTLS := webAuthRecoverCert(pending)
		ctx := &auth.Context{
			Conn: auth.ConnInfo{
				Username:   req.Username,
				GroupName:  req.Group,
				RemoteAddr: r.RemoteAddr,
				UserAgent:  r.UserAgent(),
				TLS:        selCertTLS,
			},
		}
		if ctx.Conn.TLS == nil {
			ctx.Conn.TLS = r.TLS
		}
		result := authsrv.Authenticate(ctx)
		pending.Ctx = ctx
		webAuthHandleResult(w, r, state, pending, result, req.Username)
		return
	}

	if firstStepType == "sms" {
		webAuthJSON(w, http.StatusOK, map[string]any{
			"status": "sms_phone",
			"hint":   "请输入手机号以接收短信验证码",
		})
		return
	}

	// 其他认证方式：返回凭据输入界面（预填已识别的用户名，避免重复输入）
	// 仅组过滤模式（用户已在 identify 步骤主动输入用户名）才预填，避免会话里残留的
	// 用户名（如证书 CN、历史污染）在非组过滤场景下被回传并锁死输入框
	resp := map[string]any{
		"status": "credentials",
		"hint":   "请输入登录凭据",
	}
	if base.GetCfg().EnableWebAuthGroupFilter && pending.Ctx.Conn.Username != "" {
		resp["username"] = pending.Ctx.Conn.Username
	}
	webAuthJSON(w, http.StatusOK, resp)
}

// 返回该用户可见的启用组（status=1）：以用户所属组与全部启用组求交集。
// 调用方需先确认用户存在、已启用（Status==1）且已分配组（否则在 WebAuthIdentify 中已拦截）。
func filterGroupsByUser(rawGroups []string, user *dbdata.User) []string {
	allowed := make(map[string]struct{}, len(user.Groups))
	for _, g := range user.Groups {
		allowed[g] = struct{}{}
	}
	var filtered []string
	for _, g := range rawGroups {
		if _, ok := allowed[g]; ok {
			filtered = append(filtered, g)
		}
	}
	if len(filtered) == 0 {
		return []string{}
	}
	return filtered
}

// 在选组前先收集用户名，按 User.Groups 过滤后返回可选组列表。仅支持「本地用户认证」
func WebAuthIdentify(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		webAuthError(w, "缺少认证参数")
		return
	}
	pending, err := AuthSessionManager.Get(state)
	if err != nil {
		webAuthError(w, "认证会话已过期")
		return
	}

	var req struct {
		Username string `json:"username"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" {
		webAuthError(w, "请输入用户名")
		return
	}

	// 防暴力/枚举：单 IP 高频探查多用户名可形成枚举攻击
	if !lockManager.Check(req.Username, r.RemoteAddr) {
		webAuthError(w, "操作过于频繁，请稍后重试")
		return
	}
	// 任何「已拿到有效用户名」的 identify 请求都累加 IP 计数
	// decode 失败 / session 过期等前置错误不累加。
	defer lockManager.Fail(req.Username, r.RemoteAddr)

	// 仅支持本地用户认证：用户名必须存在于本地 User 表，否则直接报错、不返回任何组。
	user := &dbdata.User{}
	if err := dbdata.One("Username", req.Username, user); err != nil {
		webAuthError(w, "用户不存在")
		return
	}
	if user.Status != 1 {
		webAuthError(w, "用户已被禁用")
		return
	}
	if len(user.Groups) == 0 {
		webAuthError(w, "该用户未分配用户组")
		return
	}

	// 记住用户名到会话：后续选组/凭据步骤据此预填，避免用户重复输入。
	pending.Ctx.Conn.Username = req.Username
	pending.UserActLog.Username = req.Username
	AuthSessionManager.Save(state, pending)

	groups := filterGroupsByUser(dbdata.GetGroupNamesNormal(), user)
	webAuthJSON(w, http.StatusOK, map[string]any{
		"status": "select_group",
		"groups": groups,
	})
}

// WebAuthStep 提交认证凭据或验证码，推进管道。
// 根据管道状态自动选择首次执行（Authenticate）或恢复执行（Resume）。
func WebAuthStep(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		webAuthError(w, "缺少认证参数")
		return
	}

	pending, err := AuthSessionManager.Get(state)
	if err != nil {
		webAuthError(w, "认证会话已过期")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Phone    string `json:"phone"`
		OtpCode  string `json:"otp_code"`
		SmsCode  string `json:"sms_code"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	json.NewDecoder(r.Body).Decode(&req)

	// SMS ：手机号输入 → 查用户名
	if req.Phone != "" {
		user := &dbdata.User{}
		if err := dbdata.One("Phone", req.Phone, user); err != nil || user.Status != 1 {
			webAuthError(w, "手机号未注册或账号异常")
			return
		}
		pending.Ctx.Conn.Username = user.Username
		pending.UserActLog.Username = user.Username

		if !lockManager.Check(user.Username, r.RemoteAddr) {
			webAuthError(w, "账号已被锁定，请稍后重试")
			return
		}

		_, _, certTLS := webAuthRecoverCert(pending)
		ctx := pending.Ctx
		ctx.Conn.Username = user.Username
		ctx.Conn.TLS = certTLS
		if certTLS == nil {
			ctx.Conn.TLS = r.TLS
		}
		ctx.Conn.RemoteAddr = r.RemoteAddr
		ctx.GetSMS().Phone = req.Phone
		authsrv.LoadUserInfo(ctx)
		result := authsrv.Authenticate(ctx)
		webAuthHandleResult(w, r, state, pending, result, user.Username)
		return
	}

	// 更新凭据到会话
	if req.Password != "" {
		pending.Ctx.Conn.Password = req.Password
	}
	if req.Username != "" {
		pending.Ctx.Conn.Username = req.Username
	}

	// 锁定检查（与 LinkAuth 一致）
	username := pending.Ctx.Conn.Username
	if username != "" && !lockManager.Check(username, r.RemoteAddr) {
		webAuthError(w, "账号已被锁定，请稍后重试")
		return
	}

	// 恢复从 AnyConnect 连接继承的客户端证书
	_, _, certTLS := webAuthRecoverCert(pending)

	// 复用已有 Context（恢复场景）或首次场景
	ctx := pending.Ctx
	if ctx == nil {
		ctx = &auth.Context{}
		pending.Ctx = ctx
	}
	ctx.Conn.TLS = certTLS
	if certTLS == nil {
		ctx.Conn.TLS = r.TLS
	}
	ctx.Conn.RemoteAddr = r.RemoteAddr

	// 注入挑战响应码
	if req.OtpCode != "" {
		ctx.GetOTP().Code = req.OtpCode
		ctx.GetRADIUS().ChallengeCode = req.OtpCode
	}
	if req.SmsCode != "" {
		ctx.GetSMS().Code = req.SmsCode
	}

	// 根据管道状态决定首次执行还是恢复
	result := webAuthRunOrResume(ctx, pending, req.OtpCode != "" || req.SmsCode != "")

	// 挑战码错误，计入锁定计数
	if result.IsChallengeRetry() && username != "" {
		lockManager.Fail(username, r.RemoteAddr)
	}

	webAuthHandleResult(w, r, state, pending, result, username)
}

// 管道结果统一处理入口。
func webAuthHandleResult(w http.ResponseWriter, r *http.Request,
	state string, pending *AuthSession, result *auth.PipelineResult, username string) {

	ctx := pending.Ctx

	switch result.Result {
	case auth.StepPass:
		if username != "" {
			lockManager.Success(username, r.RemoteAddr)
		}
		webAuthOnPass(w, r, state, pending, result)

	case auth.StepFail:
		if username != "" {
			lockManager.Fail(username, r.RemoteAddr)
		}
		errMsg := "认证失败"
		if result.Err != nil {
			errMsg = stripStepPrefix(result.Err.Error())
		}
		base.Warn("WebAuth 认证失败:", result.Err)
		webAuthError(w, errMsg)

	case auth.StepPending:
		// 保存管道断点信息
		ctx.SetStepIdx(result.State.StepIdx)
		ctx.SetPassedSteps(result.State.PassedSteps)
		AuthSessionManager.Save(state, pending)

		challenge := result.Challenge
		if challenge == nil {
			// NopChallenger（如 ldap/radius 缺少凭据）→ 显示凭据输入界面
			resp := map[string]any{
				"status": "credentials",
				"hint":   "请输入登录凭据",
			}
			if pending.Ctx.Conn.Username != "" {
				resp["username"] = pending.Ctx.Conn.Username
			}
			webAuthJSON(w, http.StatusOK, resp)
			return
		}

		switch challenge.Type {
		case auth.ChallengeOTP:
			// 首次挑战：前序凭据已通过，重置计数器给挑战阶段独立计数窗口
			if !result.IsChallengeRetry() && result.Username != "" {
				lockManager.Success(result.Username, r.RemoteAddr)
			}
			webAuthJSON(w, http.StatusOK, map[string]any{
				"status": "otp",
				"hint":   "请输入 6 位动态验证码",
			})

		case auth.ChallengeSMS:
			// 首次挑战：重置计数器（同 OTP）
			if !result.IsChallengeRetry() && result.Username != "" {
				lockManager.Success(result.Username, r.RemoteAddr)
			}
			resp := map[string]any{
				"status": "sms",
				"hint":   "请输入短信验证码",
			}
			if ctx != nil && ctx.SMS != nil && ctx.SMS.Phone != "" {
				phone := ctx.SMS.Phone
				if len(phone) > 4 {
					resp["phone_masked"] = phone[:3] + "****" + phone[len(phone)-4:]
				} else {
					resp["phone_masked"] = phone
				}
			}
			webAuthJSON(w, http.StatusOK, resp)

		case auth.ChallengeRADIUS:
			// 首次挑战：重置计数器（同 OTP）
			if !result.IsChallengeRetry() && result.Username != "" {
				lockManager.Success(result.Username, r.RemoteAddr)
			}
			msg := "请输入二次验证码"
			if ctx != nil && ctx.RADIUS != nil && ctx.RADIUS.ChallengeMsg != "" {
				msg = ctx.RADIUS.ChallengeMsg
			}
			webAuthJSON(w, http.StatusOK, map[string]any{
				"status":        "radius",
				"challenge_msg": msg,
			})

		case auth.ChallengeForcePwd:
			// 首次挑战：前序凭据已通过，重置计数器（同 OTP）
			if !result.IsChallengeRetry() && result.Username != "" {
				lockManager.Success(result.Username, r.RemoteAddr)
			}
			webAuthJSON(w, http.StatusOK, map[string]any{
				"status":   "change_pwd",
				"username": ctx.Conn.Username,
			})

		case auth.ChallengeSSO:
			// 手机端内置浏览器无法完成企微/飞书扫码，默认拒绝；开启 allow_mobile_sso 后放行
			if isMobileDevice(r) && !base.GetCfg().AllowMobileSSO {
				webAuthError(w, "手机端不支持企微/飞书扫码认证，请使用其他认证方式")
				return
			}
			ssoType, _ := challenge.Data["sso_type"].(string)
			ssoURL := webAuthBuildSSOURL(r, ssoType, ctx.Conn.GroupName, state)
			webAuthJSON(w, http.StatusOK, map[string]any{
				"status":       "sso",
				"sso_type":     ssoType,
				"redirect_url": ssoURL,
			})

		default:
			webAuthJSON(w, http.StatusOK, map[string]any{
				"status": "credentials",
				"hint":   "请继续输入验证信息",
			})
		}
	}
}

// 认证通过：保存完成标记并签发门户 Cookie。VPN 会话由 handleSsoToken 创建。
func webAuthOnPass(w http.ResponseWriter, r *http.Request,
	state string, pending *AuthSession, result *auth.PipelineResult) {

	username := result.Username
	groupName := result.GroupName

	// 更新 pending 会话信息
	pending.Ctx.Conn.Username = username
	pending.Ctx.Conn.GroupName = groupName
	if pending.UserActLog != nil {
		pending.UserActLog.Username = username
		pending.UserActLog.GroupName = groupName
		pending.UserActLog.Info = result.Info
		if pending.UserActLog.Info == "" {
			pending.UserActLog.Info = "WebAuth 认证成功"
		}
		pending.UserActLog.Status = dbdata.UserAuthSuccess
		dbdata.UserActLogIns.Add(*pending.UserActLog, pending.Ctx.Conn.UserAgent)
	}

	// 保存完成标记到 SSO 状态
	if pending.Ctx == nil {
		pending.Ctx = &auth.Context{}
	}
	pending.Ctx.GetSSO().WebAuthCompleted = true
	pending.Ctx.SSO.WebAuthUsername = username
	pending.Ctx.SSO.WebAuthGroup = groupName
	AuthSessionManager.Save(state, pending)

	// 签发门户 JWT cookie（浏览器端可直接访问 /ui/#/portal）
	user := &dbdata.User{}
	if err := dbdata.One("Username", username, user); err == nil && user.Status == 1 {
		token, pErr := portalIssueToken(user)
		if pErr == nil {
			http.SetCookie(w, &http.Cookie{
				Name:     portalCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
				SameSite: http.SameSiteLaxMode,
			})
		}
	}

	webAuthJSON(w, http.StatusOK, map[string]any{
		"status":       "done",
		"complete_url": fmt.Sprintf("/web-auth/complete?state=%s", url.QueryEscape(state)),
		"portal_url":   "/ui/#/portal",
		"success":      true,
	})
}

// webAuthBuildSSOURL 为 WebAuth 流程构建 SSO OAuth 跳转 URL。
// 生成子会话（ssoState）关联回当前 WebAuth 会话，回调到 /web-auth/sso-callback。
func webAuthBuildSSOURL(r *http.Request, ssoType, groupName, webAuthState string) string {
	// 生成 SSO 子状态，关联回当前 WebAuth 会话
	ssoState := GenerateSessionID()
	pending := &AuthSession{
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{GroupName: groupName},
			SSO: &auth.SSOState{
				Type: ssoType,
				From: "web_auth",
			},
		},
	}
	// webAuthState 存入 SSO.From 供回调后关联（此处复用 From 字段记录关联状态）
	_ = webAuthState
	AuthSessionManager.Save(ssoState, pending)

	redirectUri := fmt.Sprintf("%s/web-auth/sso-callback?web_state=%s&sso_state=%s",
		getServerAddr(r), webAuthState, ssoState)

	authURL, err := ssoBuildAuthURL(ssoType, groupName, redirectUri, ssoState)
	if err != nil {
		base.Error("生成 SSO 授权地址失败:", err)
		return ""
	}
	return authURL
}

// WebAuthSSOCallback SSO OAuth 回调端点：企微/飞书扫码授权后回调到此。
// 将认证结果写入 WebAuth 会话 SSO 状态，然后 302 回到 SPA 继续管道。
func WebAuthSSOCallback(w http.ResponseWriter, r *http.Request) {
	webState := r.URL.Query().Get("web_state")
	ssoState := r.URL.Query().Get("sso_state")

	if webState == "" || ssoState == "" {
		base.Error("WebAuth SSO 回调缺少参数")
		http.Error(w, "缺少认证参数", http.StatusBadRequest)
		return
	}

	// 校验 SSO state
	pending, err := AuthSessionManager.Get(ssoState)
	if err != nil {
		base.Error("WebAuth SSO 非法 state")
		http.Error(w, "认证会话已过期", http.StatusBadRequest)
		return
	}

	ssoType := ""
	if pending.Ctx != nil && pending.Ctx.SSO != nil {
		ssoType = pending.Ctx.SSO.Type
	}
	code := r.URL.Query().Get("code")

	var username string
	switch ssoType {
	case "wxwork":
		cfg, cErr := dbdata.GetAuthWework(pending.Ctx.Conn.GroupName)
		if cErr != nil {
			http.Error(w, "获取企微配置失败", http.StatusInternalServerError)
			return
		}
		username, err = cfg.GetWeworkUser(code)
	case "feishu":
		cfg, cErr := dbdata.GetAuthFeishu(pending.Ctx.Conn.GroupName)
		if cErr != nil {
			http.Error(w, "获取飞书配置失败", http.StatusInternalServerError)
			return
		}
		username, err = cfg.GetFeishuUser(code)
	case "dingtalk":
		cfg, cErr := dbdata.GetAuthDingtalk(pending.Ctx.Conn.GroupName)
		if cErr != nil {
			http.Error(w, "获取钉钉配置失败", http.StatusInternalServerError)
			return
		}
		var userid string
		userid, _, err = cfg.GetDingtalkUser(code)
		username = userid
	default:
		http.Error(w, "不支持的 SSO 类型", http.StatusBadRequest)
		return
	}

	if err != nil || username == "" {
		AuthSessionManager.Delete(ssoState)
		http.Error(w, "获取用户信息失败", http.StatusInternalServerError)
		return
	}

	// 将 SSO 结果写入 WebAuth 会话
	webPending, werr := AuthSessionManager.Get(webState)
	if werr != nil {
		http.Error(w, "认证会话已过期", http.StatusBadRequest)
		return
	}

	if webPending.Ctx == nil {
		webPending.Ctx = &auth.Context{}
	}
	sso := webPending.Ctx.GetSSO()
	sso.Type = ssoType
	sso.Authenticated = true
	sso.UserID = username
	webPending.Ctx.PortalLogin = true
	webPending.Ctx.Conn.Username = username
	if webPending.UserActLog != nil {
		webPending.UserActLog.Username = username
	}
	AuthSessionManager.Save(webState, webPending)

	// 清理 SSO 临时会话
	AuthSessionManager.Delete(ssoState)

	// 重定向回 WebAuth 流程，恢复管道
	redirectURL := fmt.Sprintf("/ui/#/web-auth/continue?state=%s", webState)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// SSO 回调后恢复管道。
func WebAuthContinue(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		webAuthError(w, "缺少认证参数")
		return
	}

	pending, err := AuthSessionManager.Get(state)
	if err != nil {
		webAuthError(w, "认证会话已过期")
		return
	}

	// 锁定检查
	username := pending.Ctx.Conn.Username
	if username != "" && !lockManager.Check(username, r.RemoteAddr) {
		webAuthError(w, "账号已被锁定，请稍后重试")
		return
	}

	// 恢复从 AnyConnect 连接继承的客户端证书
	_, _, certTLS := webAuthRecoverCert(pending)

	ctx := pending.Ctx
	if ctx == nil {
		ctx = &auth.Context{}
		pending.Ctx = ctx
	}
	ctx.Conn.TLS = certTLS
	if certTLS == nil {
		ctx.Conn.TLS = r.TLS
	}
	ctx.Conn.RemoteAddr = r.RemoteAddr

	// 根据管道状态决定恢复执行还是首次执行
	result := webAuthRunOrResume(ctx, pending, false)

	webAuthHandleResult(w, r, state, pending, result, username)
}

// 设置 acSamlv2Token Cookie，302 到 SAML 完成端点。
func WebAuthComplete(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "缺少认证参数", http.StatusBadRequest)
		return
	}

	// 校验会话存在且已完成
	pending, err := AuthSessionManager.Get(state)
	if err != nil {
		base.Error("[WebAuth-5:complete] 会话不存在: state=", state)
		http.Error(w, "认证会话已过期", http.StatusBadRequest)
		return
	}
	completed := pending.Ctx != nil && pending.Ctx.SSO != nil && pending.Ctx.SSO.WebAuthCompleted
	if !completed {
		base.Error("[WebAuth-5:complete] 认证尚未完成: state=", state)
		http.Error(w, "认证尚未完成", http.StatusBadRequest)
		return
	}

	// 设置 Cookie（Base64 编码 state），客户端通过 localhost API 读取
	encodeState := base64.StdEncoding.EncodeToString([]byte(state))
	SetCookie(w, "acSamlv2Token", encodeState, 0)

	// 重定向到 saml_ac_login.html — 与 SAML 流程完全一致的端点
	http.Redirect(w, r, "/+CSCOE+/saml_ac_login.html", http.StatusFound)
}

// webAuthRecoverCert 从 pending 会话恢复 AnyConnect 客户端证书信息。
// handlerWebAuth 在初始化时将 TLS 状态（含 PeerCertificates）存入 Ctx.Conn.TLS。
func webAuthRecoverCert(pending *AuthSession) (cn, ou string, tlsState *tls.ConnectionState) {
	if pending.Ctx == nil || pending.Ctx.Conn.TLS == nil || len(pending.Ctx.Conn.TLS.PeerCertificates) == 0 {
		return "", "", nil
	}
	cert := pending.Ctx.Conn.TLS.PeerCertificates[0]
	cn = cert.Subject.CommonName
	if len(cert.Subject.OrganizationalUnit) > 0 {
		ou = cert.Subject.OrganizationalUnit[0]
	}
	if cn == "" || ou == "" {
		return "", "", nil
	}
	return cn, ou, pending.Ctx.Conn.TLS
}

// 重新发送短信验证码
func WebAuthSmsResend(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		webAuthError(w, "缺少认证参数")
		return
	}

	pending, err := AuthSessionManager.Get(state)
	if err != nil {
		webAuthError(w, "认证会话已过期")
		return
	}

	phone := ""
	if pending.Ctx != nil && pending.Ctx.SMS != nil {
		phone = pending.Ctx.SMS.Phone
	}
	if phone == "" {
		webAuthError(w, "未找到手机号信息")
		return
	}

	// 重新发送验证码
	_, err = authsrv.SendSmsCode(phone)
	if err != nil {
		webAuthError(w, err.Error())
		return
	}

	// 更新会话标记，允许 SmsAuth 重新发送
	if pending.Ctx != nil && pending.Ctx.SMS != nil {
		pending.Ctx.SMS.Sent = false
	}
	AuthSessionManager.Save(state, pending)

	webAuthJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

func webAuthJSON(w http.ResponseWriter, statusCode int, data map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

func webAuthError(w http.ResponseWriter, msg string) {
	webAuthJSON(w, http.StatusOK, map[string]any{
		"status":  "error",
		"message": msg,
	})
}

// 返回 SAML XML 中 sso-v2-browser-mode 的值。
// 手机端强制使用内置浏览器（外部浏览器的 localhost:29786 回调在手机上不可用）
func webAuthBrowserMode(r *http.Request) string {
	if isMobileDevice(r) {
		return "internal"
	}
	mode := base.GetCfg().WebAuthBrowserMode
	if mode == "internal" {
		return "internal"
	}
	return "external"
}

// 根据管道状态决定首次执行还是恢复。
func webAuthRunOrResume(ctx *auth.Context, sess *AuthSession, hasChallengeResponse bool) *auth.PipelineResult {
	if hasChallengeResponse || len(sess.Ctx.PassedSteps()) > 0 || sess.Ctx.StepIdx() > 0 {
		return authsrv.Resume(ctx, auth.PipelineState{
			StepIdx:     sess.Ctx.StepIdx(),
			PassedSteps: sess.Ctx.PassedSteps(),
		})
	}
	authsrv.LoadUserInfo(ctx)
	return authsrv.Authenticate(ctx)
}

// 处理 WebAuth 强制改密提交（POST /web-auth/change_password）。
// 校验强度并更新密码、清除 ForcePwd 后续跑管道（forcepwd 步直接通过，继续后续 otp 等步骤）。
func WebAuthChangePassword(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		webAuthError(w, "缺少认证参数")
		return
	}
	pending, err := AuthSessionManager.Get(state)
	if err != nil {
		webAuthError(w, "认证会话已过期")
		return
	}

	var req struct {
		NewPassword string `json:"new_password"`
		Confirm     string `json:"new_password_confirm"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webAuthError(w, "参数错误")
		return
	}
	if req.NewPassword == "" || req.NewPassword != req.Confirm {
		webAuthError(w, "两次输入的密码不一致")
		return
	}
	if err := utils.CheckPasswordPolicy(req.NewPassword); err != nil {
		webAuthError(w, err.Error())
		return
	}

	username := pending.Ctx.Conn.Username
	hashed, err := utils.PasswordHash(req.NewPassword)
	if err != nil {
		webAuthError(w, "修改密码失败")
		return
	}
	// 按用户名直接更新，避免先查全量用户仅取 Id 的重复查库。
	if _, err := dbdata.GetXdb().Where("username = ?", username).Cols("pin_code", "change_pwd").
		Update(&dbdata.User{PinCode: hashed, ForcePwd: false}); err != nil {
		webAuthError(w, "修改密码失败")
		return
	}
	lockManager.Success(username, r.RemoteAddr)

	// 强制以数据库为准重载用户信息
	authsrv.ReloadUserInfo(pending.Ctx)

	// 续跑管道：forcepwd 步见 ForcePwd=false 直接通过，继续后续步骤（如 otp）
	result := webAuthRunOrResume(pending.Ctx, pending, true)
	if result.IsChallengeRetry() && username != "" {
		lockManager.Fail(username, r.RemoteAddr)
	}
	webAuthHandleResult(w, r, state, pending, result, username)
}
