package handler

import (
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/wsczx/remlink/admin"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/notify"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/webvpn"
)

const portalCookieName = "portal_session"

func PortalHome(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	target := "/ui/#/portal"
	if redirect := r.URL.Query().Get("redirect"); redirect != "" {
		target += "?redirect=" + url.QueryEscape(redirect)
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func PortalLogin(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16) // 64KB
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		portalError(w, "参数错误")
		dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: req.Username, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "[门户]参数解析失败"}, r.UserAgent(), true)
		return
	}
	resp := portalStartAuth(w, req.Username, req.Password, r)
	if resp.Code != 0 {
		portalError(w, resp.Msg)
		dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: req.Username, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "[门户]" + resp.Msg, IsLockedFail: resp.IsLocked}, r.UserAgent(), true)
		return
	}
	portalOK(w, resp.Data)
}

func PortalVerify(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		Code      string `json:"code"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		portalError(w, "参数错误")
		return
	}
	resp := portalResumeAuth(w, req.SessionID, req.Code, r)
	if resp.Code != 0 {
		portalError(w, resp.Msg)
		dbdata.UserActLogIns.Add(dbdata.UserActLog{RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "[门户]" + resp.Msg}, r.UserAgent(), true)
		return
	}
	portalOK(w, resp.Data)
}

// 发送短信验证码
func PortalSmsSend(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal || !notify.IsSmsConfigured() {
		portalError(w, "短信登录未启用")
		return
	}
	var req struct {
		Phone string `json:"phone"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<12)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Phone == "" {
		portalError(w, "请输入手机号")
		return
	}

	// 查找手机号对应的本地用户
	user := &dbdata.User{}
	if err := dbdata.One("Phone", req.Phone, user); err != nil || user.Status != 1 {
		portalError(w, "手机号未注册或账号异常")
		return
	}

	// 发送验证码（传入来源 IP 做窗口限流，防多目标短信轰炸）
	_, err := authsrv.SendSmsCode(req.Phone, r.RemoteAddr)
	if err != nil {
		base.Warn("Portal短信登录发送失败:", err)
		portalError(w, "验证码发送失败，请稍后重试")
		return
	}

	portalOK(w, map[string]any{
		"message":    "验证码已发送",
		"phone_tail": req.Phone[len(req.Phone)-4:],
	})
}

// 验证短信验证码并登录
func PortalSmsVerify(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal || !notify.IsSmsConfigured() {
		portalError(w, "短信登录未启用")
		return
	}
	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<12)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Phone == "" || req.Code == "" {
		portalError(w, "参数错误")
		return
	}

	// 防暴力破解
	if !lockManager.Check(req.Phone, r.RemoteAddr) {
		portalError(w, "验证过于频繁，请稍后重试")
		dbdata.UserActLogIns.Add(dbdata.UserActLog{RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "验证过于频繁，请稍后重试", IsLockedFail: true}, r.UserAgent(), true)
		return
	}

	// 验证短信验证码
	_, err := authsrv.VerifySmsCode(req.Phone, req.Code)
	if err != nil {
		lockManager.Fail(req.Phone, r.RemoteAddr)
		portalError(w, err.Error())
		dbdata.UserActLogIns.Add(dbdata.UserActLog{RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "门户登录失败"}, r.UserAgent(), true)
		return
	}

	// 查找用户
	user := &dbdata.User{}
	if err := dbdata.One("Phone", req.Phone, user); err != nil || user.Status != 1 {
		portalError(w, "用户不存在或已禁用")
		dbdata.UserActLogIns.Add(dbdata.UserActLog{RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "门户登录失败"}, r.UserAgent(), true)
		return
	}

	lockManager.Success(req.Phone, r.RemoteAddr)
	resp := portalIssueLoginResponse(w, r, user, "短信验证码登录成功")
	if resp.Code != 0 {
		portalError(w, resp.Msg)
		dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: user.Username, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "门户登录失败"}, r.UserAgent(), true)
		return
	}
	portalOK(w, resp.Data)
}

func PortalSSO(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	ssoType := r.URL.Query().Get("type")
	if !ssoTypeEnabled(ssoType) {
		http.Error(w, "不支持的第三方登录类型", http.StatusBadRequest)
		return
	}
	redirect := r.URL.Query().Get("redirect")

	// WebVPN 子域的第三方登录先在配置的门户域名完成，再兑换独立会话
	if _, ok := webVpnHostPrefix(r.Host); ok {
		main := portalMainDomain(r)
		if main == "" {
			http.Error(w, "未配置 WebVPN 第三方登录专用门户域名，请先在系统设置中填写 webvpn_sso_domain", http.StatusBadRequest)
			return
		}
		target := main + "/portal/api/sso?type=" + url.QueryEscape(ssoType)
		if redirect != "" {
			target += "&redirect=" + url.QueryEscape(redirect)
		}
		http.Redirect(w, r, target, http.StatusFound)
		return
	}

	group, err := portalSSOGroup(ssoType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	target := fmt.Sprintf("/+CSCOE+/saml/sp/login?tgname=%s&ssotype=%s&from=portal",
		url.QueryEscape(group), url.QueryEscape(ssoType))
	if redirect != "" {
		target += "&redirect=" + url.QueryEscape(redirect)
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// 返回 WebVPN 第三方登录使用的门户域名，未配置时返回空字符串
func portalMainDomain(r *http.Request) string {
	domain := base.GetCfg().WebVpnSsoDomain
	if domain == "" {
		return ""
	}
	host := stripPort(domain)
	port := portOf(domain)
	if port == "" {
		port = portOf(r.Host)
	}
	main := "https://" + host
	if port != "" {
		main += ":" + port
	}
	return main
}

func PortalMe(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}
	portalOK(w, portalUserInfo(user, r))
}

func PortalChangePassword(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}
	if user.Type != "" && user.Type != "local" {
		portalError(w, "外部认证用户请到对应身份源修改密码")
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		portalError(w, "参数错误")
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		portalError(w, "新旧密码不能为空")
		return
	}
	if err := utils.CheckPasswordPolicy(req.NewPassword); err != nil {
		portalError(w, err.Error())
		return
	}
	if err := portalCheckLocalPassword(user, req.OldPassword); err != nil {
		portalError(w, "旧密码错误")
		return
	}

	hashed, err := utils.PasswordHash(req.NewPassword)
	if err != nil {
		base.Error("用户门户密码哈希失败:", err)
		portalError(w, "修改密码失败")
		return
	}
	if _, err := dbdata.GetXdb().Where("username = ?", user.Username).Cols("pin_code", "change_pwd").
		Update(&dbdata.User{PinCode: hashed, ForcePwd: false}); err != nil {
		base.Error("用户门户修改密码失败:", err)
		portalError(w, "修改密码失败")
		return
	}
	portalOK(w, map[string]string{"message": "密码修改成功"})
}

// 处理门户首次登录的强制改密，并继续认证流程
func PortalForceChangePassword(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
		Confirm     string `json:"new_password_confirm"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		portalError(w, "参数错误")
		return
	}
	if req.Token == "" || req.NewPassword == "" {
		portalError(w, "参数不能为空")
		return
	}
	if req.NewPassword != req.Confirm {
		portalError(w, "两次输入的密码不一致")
		return
	}
	if err := utils.CheckPasswordPolicy(req.NewPassword); err != nil {
		portalError(w, err.Error())
		return
	}

	sess, err := AuthSessionManager.Get(req.Token)
	if err != nil || sess.Ctx == nil {
		portalError(w, "改密会话已过期，请重新登录")
		return
	}
	username := sess.Ctx.Conn.Username
	if username == "" {
		portalError(w, "改密会话数据异常")
		return
	}
	user := &dbdata.User{}
	if err := dbdata.One("Username", username, user); err != nil || user.Status != 1 {
		portalError(w, "用户不存在或已停用")
		return
	}
	if user.Type != "" && user.Type != "local" {
		portalError(w, "外部认证用户请到对应身份源修改密码")
		return
	}
	if !user.ForcePwd {
		portalError(w, "无需修改密码或会话已失效")
		return
	}

	// 改密核心操作（策略校验 + 哈希 + 写库 + 重载）由共享函数统一处理
	if err := RunForcePwdChange(sess.Ctx, username, req.NewPassword); err != nil {
		base.Error("用户门户强制改密失败:", err)
		portalError(w, "修改密码失败")
		return
	}

	sess.Ctx.Conn.RemoteAddr = r.RemoteAddr
	result := authsrv.Resume(sess.Ctx, auth.PipelineState{
		StepIdx:     sess.Ctx.StepIdx(),
		PassedSteps: sess.Ctx.PassedSteps(),
	})
	switch result.Result {
	case auth.StepPass:
		AuthSessionManager.Delete(req.Token)
		portalUser, err := portalResolveUser(username, result.GroupName, user, true)
		if err != nil {
			portalError(w, err.Error())
			return
		}
		lockManager.Success(username, r.RemoteAddr)
		resp := portalIssueLoginResponse(w, r, portalUser, "首次登录修改密码成功")
		if resp.Code != 0 {
			portalError(w, resp.Msg)
			return
		}
		portalOK(w, resp.Data)
	case auth.StepPending:
		sess.Ctx.SetStepIdx(result.State.StepIdx)
		sess.Ctx.SetPassedSteps(result.State.PassedSteps)
		AuthSessionManager.Save(req.Token, sess)
		portalOK(w, portalChallengeResponse(req.Token, result, sess.Ctx).Data)
	case auth.StepFail:
		if result.Err != nil {
			base.Warn("用户门户强制改密后续认证失败:", result.Err)
		}
		lockManager.Fail(username, r.RemoteAddr)
		if base.GetCfg().DisplayError && result.Err != nil {
			portalError(w, result.Err.Error())
			return
		}
		portalError(w, "修改密码失败")
	}
}

func PortalLogout(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     portalCookieName,
		Value:    "",
		Path:     "/",
		Domain:   "",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})
	// 门户登出同时吊销该用户的 WebVPN 会话，避免旧会话继续使用
	if base.GetCfg().WebVpnDomain != "" {
		if user, ok := portalCurrentUser(r); ok && user != nil {
			webvpn.GetManager().Session().RevokeUser(user.Username)
		}
	}
	// 清除一次性免登授权
	webvpn.GetManager().Session().ClearGrantCookie(w, r)
	webvpn.GetManager().Session().ClearCookie(w, r)
	portalOK(w, map[string]string{"message": "已退出"})
}

// 返回登录页品牌与认证方式开关，供门户、WebAuth、管理后台登录页在未登录时渲染。
func PortalLoginConfig(w http.ResponseWriter, r *http.Request) {
	brand := dbdata.SettingPortalBrand{}
	_ = dbdata.SettingGet(&brand)
	portalOK(w, map[string]any{
		"title":             brand.Title,
		"logo":              brand.Logo,
		"favicon":           brand.Favicon,
		"desc":              brand.Desc,
		"footer":            brand.Footer,
		"features_enabled":  brand.FeaturesEnabled,
		"features":          brand.Features,
		"issuer":            base.GetCfg().Issuer,
		"domain":            base.GetCfg().WebVpnDomain,
		"webvpn_sso_domain": base.GetCfg().WebVpnSsoDomain,
		"sso_types":         portalEnabledSSOTypes(),
		"sms_enabled":       notify.IsSmsConfigured(),
	})
}

// 返回门户支持且至少存在一个组已配置的 SSO 类型。
func portalEnabledSSOTypes() []string {
	types := make([]string, 0, len(ssoProviders))
	for t := range ssoProviders {
		if _, err := portalSSOGroup(t); err == nil {
			types = append(types, t)
		}
	}
	return types
}

// 判断该 SSO 类型是否在 ssoProviders 注册
func ssoTypeEnabled(ssoType string) bool {
	if ssoType == "" {
		return false
	}
	_, ok := ssoProviders[ssoType]
	return ok
}

var portalForgotLimiter = struct {
	mu   sync.Mutex
	next map[string]time.Time
}{next: make(map[string]time.Time)}

var portalResetTokens = struct {
	mu     sync.Mutex
	used   map[string]int64 // jti -> used_at unix
	inited bool
}{used: make(map[string]int64)}

func portalInitResetTokens() {
	portalResetTokens.mu.Lock()
	if portalResetTokens.inited {
		portalResetTokens.mu.Unlock()
		return
	}
	portalResetTokens.inited = true
	portalResetTokens.mu.Unlock()

	data := dbdata.SettingPortalResetTokens{}
	if err := dbdata.SettingGet(&data); err == nil && data.Tokens != nil {
		portalResetTokens.mu.Lock()
		now := time.Now().Unix()
		for jti, ts := range data.Tokens {
			if now-ts < 900 {
				portalResetTokens.used[jti] = ts
			}
		}
		portalResetTokens.mu.Unlock()
	}
}

func portalIsTokenUsed(jti string) bool {
	portalInitResetTokens()
	portalResetTokens.mu.Lock()
	defer portalResetTokens.mu.Unlock()
	_, ok := portalResetTokens.used[jti]
	return ok
}

// 返回 true 表示 token 已经被消费过。
func portalTryMarkToken(jti string) bool {
	portalInitResetTokens()
	portalResetTokens.mu.Lock()
	defer portalResetTokens.mu.Unlock()

	if _, ok := portalResetTokens.used[jti]; ok {
		return true
	}

	now := time.Now().Unix()
	portalResetTokens.used[jti] = now

	cutoff := now - 900
	clean := make(map[string]int64, len(portalResetTokens.used))
	for j, ts := range portalResetTokens.used {
		if ts >= cutoff {
			clean[j] = ts
		}
	}
	portalResetTokens.used = clean
	if err := dbdata.SettingSave(&dbdata.SettingPortalResetTokens{Tokens: clean}); err != nil {
		base.Error("用户门户保存重置token状态失败:", err)
	}
	return false
}

func PortalForgotPassword(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Email == "" {
		portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
		return
	}

	limiterKey := host + ":" + req.Username
	now := time.Now()
	portalForgotLimiter.mu.Lock()
	if t, ok := portalForgotLimiter.next[limiterKey]; ok && now.Before(t) {
		portalForgotLimiter.mu.Unlock()
		portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
		return
	}
	portalForgotLimiter.next[limiterKey] = now.Add(60 * time.Second)
	for k, t := range portalForgotLimiter.next {
		if now.After(t) {
			delete(portalForgotLimiter.next, k)
		}
	}
	portalForgotLimiter.mu.Unlock()

	user := &dbdata.User{}
	err = dbdata.One("Username", req.Username, user)
	if err != nil || user.Status != 1 {
		portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
		return
	}
	if user.Type != "" && user.Type != "local" {
		portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
		return
	}
	if user.Email != req.Email {
		portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
		return
	}

	jti := fmt.Sprintf("%d_%d", user.Id, time.Now().UnixNano())
	token, err := admin.SetJwtData(map[string]any{
		"purpose": "portal_reset_password",
		"user_id": user.Id,
		"jti":     jti,
	}, time.Now().Unix()+900)
	if err != nil {
		base.Error("用户门户生成重置token失败:", err)
		portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
		return
	}

	resetURL := getServerAddr(r) + "/ui/#/portal?reset_token=" + url.QueryEscape(token)
	htmlBody := fmt.Sprintf(`<html><body>
<h2>RemLink 密码重置</h2>
<p>您好 %s，</p>
<p>请点击下方链接重置您的密码（15 分钟内有效）：</p>
<p><a href="%s">%s</a></p>
	<p>如果您未发起此请求，请忽略此邮件。</p>
	</body></html>`, html.EscapeString(user.Username), html.EscapeString(resetURL), html.EscapeString(resetURL))

	smtpCfg := &dbdata.SettingSmtp{}
	if err := dbdata.SettingGet(smtpCfg); err != nil || smtpCfg.Host == "" {
		base.Warn("用户门户SMTP未配置，跳过发送重置邮件:", user.Username)
	} else {
		go func() {
			if err := notify.GetNotify().SendEmail(notify.Message{
				Subject: "RemLink 密码重置",
				To:      user.Email,
				Body:    htmlBody,
			}); err != nil {
				base.Error("用户门户发送重置邮件失败:", user.Username, err)
			}
		}()
	}

	portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
}

func PortalResetPasswordVerify(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		portalOK(w, map[string]any{"valid": false})
		return
	}

	data, err := admin.GetJwtData(token)
	if err != nil {
		portalOK(w, map[string]any{"valid": false})
		return
	}

	purpose, _ := data["purpose"].(string)
	if purpose != "portal_reset_password" {
		portalOK(w, map[string]any{"valid": false})
		return
	}

	jti, _ := data["jti"].(string)
	if jti == "" || portalIsTokenUsed(jti) {
		portalOK(w, map[string]any{"valid": false})
		return
	}

	userId, _ := data["user_id"].(float64)
	user := &dbdata.User{}
	if err := dbdata.One("Id", int(userId), user); err != nil || user.Status != 1 {
		portalOK(w, map[string]any{"valid": false})
		return
	}
	if user.Type != "" && user.Type != "local" {
		portalOK(w, map[string]any{"valid": false})
		return
	}

	portalOK(w, map[string]any{
		"valid":    true,
		"username": user.Username,
	})
}

func PortalResetPassword(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}

	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		portalError(w, "参数错误")
		return
	}
	if req.Token == "" || req.NewPassword == "" {
		portalError(w, "参数不能为空")
		return
	}
	if err := utils.CheckPasswordPolicy(req.NewPassword); err != nil {
		portalError(w, err.Error())
		return
	}

	data, err := admin.GetJwtData(req.Token)
	if err != nil {
		portalError(w, "重置链接已过期，请重新申请")
		return
	}
	purpose, _ := data["purpose"].(string)
	if purpose != "portal_reset_password" {
		portalError(w, "无效的重置链接")
		return
	}

	jti, _ := data["jti"].(string)
	if jti == "" {
		portalError(w, "无效的重置链接")
		return
	}

	userId, _ := data["user_id"].(float64)
	user := &dbdata.User{}
	if err := dbdata.One("Id", int(userId), user); err != nil || user.Status != 1 {
		portalError(w, "用户不存在或已停用")
		return
	}
	if user.Type != "" && user.Type != "local" {
		portalError(w, "外部认证用户请到对应身份源修改密码")
		return
	}

	hashed, err := utils.PasswordHash(req.NewPassword)
	if err != nil {
		base.Error("用户门户重置密码哈希失败:", err)
		portalError(w, "重置密码失败")
		return
	}

	if portalTryMarkToken(jti) {
		portalError(w, "重置链接已失效，请重新申请")
		return
	}

	// 重置密码清除强制改密标记
	if _, err := dbdata.GetXdb().Where("username = ?", user.Username).Cols("pin_code", "change_pwd").
		Update(&dbdata.User{PinCode: hashed, ForcePwd: false}); err != nil {
		base.Error("用户门户重置密码失败:", err)
		portalError(w, "重置密码失败")
		return
	}

	portalOK(w, map[string]string{"message": "密码重置成功，请使用新密码登录"})
}

type portalAuthResponse struct {
	Code     int
	Msg      string
	Data     map[string]any
	IsLocked bool `json:"-"` // 是否因账号/IP 被锁定而失败，用于日志限频
}

func portalStartAuth(w http.ResponseWriter, username, password string, r *http.Request) portalAuthResponse {
	if username == "" || len(password) < 1 {
		return portalAuthError("用户名或密码错误")
	}

	// 防暴力破解：检查锁定状态
	if !lockManager.Check(username, r.RemoteAddr) {
		recordFailAudit(auth.ConnInfo{Username: username, RemoteAddr: r.RemoteAddr, UserAgent: r.UserAgent()},
			username, r.RemoteAddr, "[门户]账号已被锁定，请稍后重试", true)
		resp := portalAuthError("账号已被锁定，请稍后重试")
		resp.IsLocked = true
		return resp
	}

	user, userExists, err := portalLoadUser(username)
	if err != nil {
		return portalAuthError(err.Error())
	}
	// 未同步用户使用外部认证
	if !userExists {
		user = &dbdata.User{Username: username}
	}

	var lastErr error
	var finalResp portalAuthResponse
	for _, group := range portalCandidateGroups(user, userExists) {
		ctx := &auth.Context{
			Conn: auth.ConnInfo{
				Username:   username,
				Password:   password,
				GroupName:  group,
				RemoteAddr: r.RemoteAddr,
				UserAgent:  r.UserAgent(),
			},
			PortalLogin: true,
		}
		flow := &Flow{
			Ctx:      ctx,
			Username: username,
			Source:   "门户",
			Callbacks: FlowCallbacks{
				OnPass: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
					portalUser, err := portalResolveUser(username, group, user, userExists)
					if err != nil {
						lastErr = err
						return
					}
					finalResp = portalIssueLoginResponse(w, r, portalUser, ctx.LogInfo())
				},
				OnFail: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
					if fl.Result.Err != nil {
						lastErr = fl.Result.Err
						base.Warn("用户门户认证失败:", username, group, fl.Result.Err)
					}
				},
				OnChallenge: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
					sessionID := portalSaveChallengeSession(fl.Result, ctx)
					finalResp = portalChallengeResponse(sessionID, fl.Result, ctx)
				},
			},
		}
		flow.Run(w, r)
		// OnPass：直接签发登录响应并返回
		if flow.Result.Result == auth.StepPass && finalResp.Code == 0 {
			return finalResp
		}
		// OnChallenge：挑战响应已构造，直接返回（由 PortalLogin 调 portalOK 写出）
		if flow.Result.Result == auth.StepPending {
			return finalResp
		}
		// OnFail：继续尝试下一个候选组
	}
	lockManager.Fail(username, r.RemoteAddr)
	if lastErr != nil && base.GetCfg().DisplayError {
		return portalAuthError(lastErr.Error())
	}
	return portalAuthError("用户名或密码错误")
}

func portalResumeAuth(w http.ResponseWriter, sessionID, code string, r *http.Request) portalAuthResponse {
	if sessionID == "" || code == "" {
		return portalAuthError("参数不能为空")
	}
	sess, err := AuthSessionManager.Get(sessionID)
	if err != nil || sess.Ctx == nil {
		return portalAuthError("认证会话已过期，请重新登录")
	}

	username := sess.Ctx.Conn.Username
	ctx := sess.Ctx
	if ctx == nil {
		ctx = &auth.Context{
			Conn: auth.ConnInfo{
				Username:   sess.Ctx.Conn.Username,
				Password:   sess.Ctx.Conn.Password,
				GroupName:  sess.Ctx.Conn.GroupName,
				RemoteAddr: r.RemoteAddr,
				UserAgent:  r.UserAgent(),
			},
			PortalLogin: true,
		}
		sess.Ctx = ctx
	} else {
		ctx.Conn.RemoteAddr = r.RemoteAddr
	}
	// 注入挑战响应码（OTP / RADIUS / SMS 复用同一 code 字段）
	ctx.GetOTP().Code = code
	ctx.GetRADIUS().ChallengeCode = code
	ctx.GetSMS().Code = code

	state := auth.PipelineState{
		StepIdx:     sess.Ctx.StepIdx(),
		PassedSteps: sess.Ctx.PassedSteps(),
	}

	var resp portalAuthResponse
	flow := &Flow{
		Ctx:      ctx,
		Username: username,
		Source:   "门户",
		Session:  sess,
		Callbacks: FlowCallbacks{
			OnPass: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
				AuthSessionManager.Delete(sessionID)
				user, userExists, err := portalLoadUser(fl.Result.Username)
				if err != nil {
					resp = portalAuthError(err.Error())
					return
				}
				portalUser, err := portalResolveUser(fl.Result.Username, fl.Result.GroupName, user, userExists)
				if err != nil {
					resp = portalAuthError(err.Error())
					return
				}
				resp = portalIssueLoginResponse(w, r, portalUser, ctx.LogInfo())
			},
			OnFail: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
				if fl.Result.Err != nil {
					base.Warn("用户门户二次认证失败:", fl.Result.Err)
				}
				if base.GetCfg().DisplayError && fl.Result.Err != nil {
					resp = portalAuthError(fl.Result.Err.Error())
					return
				}
				resp = portalAuthError("验证失败")
			},
			OnChallenge: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
				fl.savePendingState()
				resp = portalChallengeResponse(sessionID, fl.Result, ctx)
			},
		},
	}
	flow.Resume(w, r, state)

	// 回调已设置 resp（成功或失败），直接返回
	if resp.Data != nil || resp.Code != 0 {
		return resp
	}
	return portalAuthError("验证失败")
}

func portalSaveChallengeSession(result *auth.PipelineResult, ctx *auth.Context) string {
	sessionID := GenerateSessionID()
	if ctx == nil {
		ctx = &auth.Context{}
	}
	ctx.PortalLogin = true
	ctx.SetStepIdx(result.State.StepIdx)
	ctx.SetPassedSteps(result.State.PassedSteps)
	AuthSessionManager.Save(sessionID, &AuthSession{
		Ctx: ctx,
	})
	return sessionID
}

func portalChallengeResponse(sessionID string, result *auth.PipelineResult, ctx *auth.Context) portalAuthResponse {
	// SSO 挑战由门户独立 SSO 入口处理，管道内不产出 ChallengeSSO，此处仅占位返回
	if result.Challenge != nil && result.Challenge.Type == auth.ChallengeSSO {
		return portalAuthResponse{Data: map[string]any{
			"status":     "sso",
			"session_id": sessionID,
			"message":    "请完成第三方登录",
		}}
	}

	// OTP/SMS/RADIUS/ForcePwd/credentials：统一挑战视图序列化为门户 JSON
	// （status/message 来源唯一，session_id/token 由 ToPortalJSON 统一追加）
	view := BuildChallengeView(result, ctx, result.IsChallengeRetry())
	return portalAuthResponse{Data: view.ToPortalJSON(sessionID)}
}

func portalIssueLoginResponse(w http.ResponseWriter, r *http.Request, user *dbdata.User, logInfo string) portalAuthResponse {
	// 写入用户活动日志
	groupName := ""
	if len(user.Groups) > 0 {
		groupName = user.Groups[0]
	}
	if logInfo == "" {
		logInfo = "门户登录成功"
	}
	dbdata.UserActLogIns.Add(dbdata.UserActLog{
		Username:   user.Username,
		GroupName:  groupName,
		RemoteAddr: r.RemoteAddr,
		Status:     dbdata.UserAuthSuccess,
		Info:       logInfo,
	}, r.UserAgent(), true)

	token, err := portalIssueToken(user)
	if err != nil {
		return portalAuthError("登录失败")
	}
	if w != nil {
		// WebVPN 子域登录场景：只签发 webvpn 免登授权
		// 不写门户 portal_session。否则门户登录态会被父域共享 cookie 污染，
		// 导致用户在父域门户无法切换到别的账号登录
		_, fromWebVpn := webVpnHostPrefix(r.Host)
		if !fromWebVpn {
			http.SetCookie(w, &http.Cookie{
				Name:     portalCookieName,
				Value:    token,
				Path:     "/",
				Domain:   "",
				HttpOnly: true,
				Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
				SameSite: http.SameSiteLaxMode,
			})
		}
		// 若 WebVPN 已启用，登录后签发一次性免登授权 webvpn_grant，
		// 用户在 WebVPN 子域首次访问时凭此兑换正式会话，无需重复登录
		if base.GetCfg().WebVpnDomain != "" {
			if jti, jerr := admin.JtiOf(token); jerr == nil {
				if _, gerr := webvpn.GetManager().Session().IssueGrant(w, r, user, jti); gerr != nil {
					base.Warn("WebVPN 免登授权签发失败:", gerr)
				}
			}
		}
	}
	return portalAuthResponse{Data: map[string]any{
		"status": "pass",
		"token":  token,
		"user":   portalUserInfo(user, r),
	}}
}

func portalAuthError(msg string) portalAuthResponse {
	return portalAuthResponse{Code: 1, Msg: msg}
}

func portalLoadUser(username string) (*dbdata.User, bool, error) {
	user := &dbdata.User{}
	if err := dbdata.One("Username", username, user); err != nil {
		if dbdata.CheckErrNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("用户查询失败")
	}
	switch user.Status {
	case 1:
		return user, true, nil
	case 2:
		return nil, true, fmt.Errorf("用户已过期")
	default:
		return nil, true, fmt.Errorf("用户不存在或已停用")
	}
}

func portalCandidateGroups(user *dbdata.User, userExists bool) []string {
	groups, err := dbdata.GetAllGroups()
	if err != nil {
		base.Error("用户门户获取用户组失败:", err)
		return nil
	}

	// 构建组名到 auth profile 的映射
	groupMap := make(map[string]json.RawMessage, len(groups))
	for _, g := range groups {
		groupMap[g.Name] = g.AuthProfile
	}

	candidateNames := user.Groups
	if !userExists || len(candidateNames) == 0 {
		// 未注册用户或用户未分配组：汇总所有支持 ldap/radius 的组
		candidateNames = make([]string, 0, len(groups))
		for _, g := range groups {
			candidateNames = append(candidateNames, g.Name)
		}
	}

	// 按用户类型过滤：仅返回 AuthProfile 匹配的组
	names := make([]string, 0, len(candidateNames))
	for _, name := range candidateNames {
		profile := groupMap[name]
		switch user.Type {
		case "ldap":
			if dbdata.HasAuthType(profile, "ldap") {
				names = append(names, name)
			}
		case "radius":
			if dbdata.HasAuthType(profile, "radius") {
				names = append(names, name)
			}
		default:
			// 未同步用户只尝试有外部认证的组
			if !userExists {
				if dbdata.HasAuthType(profile, "ldap") || dbdata.HasAuthType(profile, "radius") {
					names = append(names, name)
				}
			} else {
				// 本地用户仅保留含 local 步骤的组，避免给密码登录用户弹出 SSO 等挑战
				if dbdata.HasAuthType(profile, "local") {
					names = append(names, name)
				}
			}
		}
	}
	return names
}

func portalResolveUser(username, group string, user *dbdata.User, userExists bool) (*dbdata.User, error) {
	if userExists {
		return user, nil
	}
	return &dbdata.User{
		Type:     "external",
		Username: username,
		Groups:   []string{group},
		Status:   1,
	}, nil
}
