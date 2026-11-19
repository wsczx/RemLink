package handler

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

// SAML SP 登录入口，302 到 SPA 前端
func (s *WebAuthService) SPLogin(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "缺少认证参数", http.StatusBadRequest)
		return
	}

	if _, err := s.sessions.Get(state); err != nil {
		base.Error("[WebAuth-2:sp/login] 会话不存在或已过期: state=", state)
		http.Error(w, "认证会话已过期", http.StatusBadRequest)
		return
	}

	redirectURL := "/ui/#/web-auth?state=" + url.QueryEscape(state)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// Web 认证入口：检查证书自动识别组，或返回可选组列表
func (s *WebAuthService) Start(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableWebAuth {
		http.NotFound(w, r)
		return
	}

	pending, state, ok := s.Pending(w, r)
	if !ok {
		return
	}

	// 证书自动识别组（从原始 AnyConnect 连接继承的证书信息）
	// 仅当存在启用 cert 认证的组时才尝试从证书恢复身份
	var certCN, certOU string
	var certTLS *tls.ConnectionState
	if dbdata.Groups.AnyGroupHasCertAuth() {
		certCN, certOU, certTLS = s.RecoverCert(pending)
	}

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
			s.Dispatch(w, r, state, pending, certCN)
			return
		}

		pending.Ctx.Conn.Username = ""
		pending.UserActLog.Username = ""
		certErrMsg = "证书自动认证失败，请选择其他组登录"
		base.Info("[WebAuth-1:start] 证书自动认证失败，回退组选择 ou=", certOU, " err=", result.Err)
	}

	groups := dbdata.Groups.GetGroupNamesNormal()
	resp := map[string]any{
		"status": "select_group",
		"groups": groups,
	}
	if certErrMsg != "" {
		resp["message"] = certErrMsg
	}

	if base.GetCfg().EnableWebAuthGroupFilter {
		resp["require_identify"] = true
	}
	s.WriteJSON(w, http.StatusOK, resp)
}

// 选定组并执行认证管道。首步非 SSO 时返回凭据输入界面
func (s *WebAuthService) SelectGroup(w http.ResponseWriter, r *http.Request) {
	pending, state, ok := s.Pending(w, r)
	if !ok {
		return
	}

	var req struct {
		Group    string `json:"group"`
		Username string `json:"username"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Group == "" {
		s.WriteError(w, "请选择用户组")
		return
	}

	groupData, err := dbdata.Groups.ByName(req.Group)
	if err != nil {
		if utils.CheckErrNotFound(err) {
			s.WriteError(w, "用户组不存在")
			return
		}
		s.WriteError(w, "系统繁忙，请稍后重试")
		return
	}

	pending.Ctx.Conn.GroupName = req.Group
	if req.Username != "" {
		pending.Ctx.Conn.Username = req.Username
		pending.UserActLog.Username = req.Username
	}
	pending.UserActLog.GroupName = req.Group
	s.sessions.Save(state, pending)

	profile, pErr := auth.ParseAuthProfile(groupData.AuthProfile)
	if pErr != nil || len(profile.Step) == 0 {
		s.WriteError(w, "该组未配置认证方式")
		return
	}

	firstStepType := profile.Step[0].Type

	if auth.Registry.IsSSOType(firstStepType) {

		if isMobileDevice(r) && !base.GetCfg().AllowMobileSSO {
			s.WriteError(w, "手机端不支持企微/飞书扫码认证，请选择其他组或联系管理员")
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

		if _, _, selCertTLS := s.RecoverCert(pending); selCertTLS != nil {
			ctx.Conn.TLS = selCertTLS
		}
		pending.Ctx = ctx
		s.Dispatch(w, r, state, pending, req.Username)
		return
	}

	if firstStepType == "cert" {
		_, _, selCertTLS := s.RecoverCert(pending)
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
		pending.Ctx = ctx
		s.Dispatch(w, r, state, pending, req.Username)
		return
	}

	if firstStepType == "sms" {
		s.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "sms_phone",
			"hint":   webAuthEntrySMSPhoneHint,
		})
		return
	}

	resp := map[string]any{
		"status": "credentials",
		"hint":   webAuthEntryCredentialsHint,
	}
	if base.GetCfg().EnableWebAuthGroupFilter && pending.Ctx.Conn.Username != "" {
		resp["username"] = pending.Ctx.Conn.Username
	}
	s.WriteJSON(w, http.StatusOK, resp)
}

// 返回该用户可见的启用组（status=1）：以用户所属组与全部启用组求交集
// 调用方需先确认用户存在、已启用（Status==1）且已分配组（否则在 WebAuthIdentify 中已拦截）
func (s *WebAuthService) filterGroupsByUser(rawGroups []string, user *dbdata.User) []string {
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
func (s *WebAuthService) Identify(w http.ResponseWriter, r *http.Request) {
	pending, state, ok := s.Pending(w, r)
	if !ok {
		return
	}

	var req struct {
		Username string `json:"username"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" {
		s.WriteError(w, "请输入用户名")
		return
	}

	if !s.locks.Check(req.Username, r.RemoteAddr) {
		recordFailAudit(pending.Ctx.Conn, req.Username, r.RemoteAddr, "[WebAuth]账号已被锁定，请稍后重试")
		s.WriteError(w, "操作过于频繁，请稍后重试")
		return
	}

	defer s.locks.Fail(req.Username, r.RemoteAddr)

	user := &dbdata.User{}
	if err := dbdata.Users.GetByUsername(req.Username, user); err != nil {
		s.WriteError(w, "用户不存在")
		return
	}
	if user.Status != 1 {
		s.WriteError(w, "用户已被禁用")
		return
	}
	if len(user.Groups) == 0 {
		s.WriteError(w, "该用户未分配用户组")
		return
	}

	pending.Ctx.Conn.Username = req.Username
	pending.UserActLog.Username = req.Username
	s.sessions.Save(state, pending)

	groups := s.filterGroupsByUser(dbdata.Groups.GetGroupNamesNormal(), user)
	s.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "select_group",
		"groups": groups,
	})
}

// 提交认证凭据或验证码，推进管道
// 根据管道状态自动选择首次执行（Authenticate）或恢复执行（Resume）
func (s *WebAuthService) Step(w http.ResponseWriter, r *http.Request) {
	pending, state, ok := s.Pending(w, r)
	if !ok {
		return
	}
	if pending.Ctx == nil {
		s.sessions.Delete(state)
		s.WriteError(w, "认证会话已过期")
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

	if req.Phone != "" {
		user := &dbdata.User{}
		if err := dbdata.Users.GetByPhone(req.Phone, user); err != nil || user.Status != 1 {
			s.WriteError(w, "手机号未注册或账号异常")
			return
		}
		pending.Ctx.Conn.Username = user.Username
		pending.UserActLog.Username = user.Username

		if !s.locks.Check(user.Username, r.RemoteAddr) {
			recordFailAudit(pending.Ctx.Conn, user.Username, r.RemoteAddr, "[WebAuth]账号已被锁定，请稍后重试")
			s.WriteError(w, "账号已被锁定，请稍后重试")
			return
		}

		_, _, certTLS := s.RecoverCert(pending)
		ctx := pending.Ctx
		ctx.Conn.Username = user.Username
		ctx.Conn.TLS = certTLS
		if certTLS == nil {
			ctx.Conn.TLS = r.TLS
		}
		ctx.Conn.RemoteAddr = r.RemoteAddr
		ctx.GetSMS().Phone = req.Phone
		authsrv.LoadUserInfo(ctx)
		pending.Ctx = ctx
		s.Dispatch(w, r, state, pending, user.Username)
		return
	}

	if req.Password != "" {
		pending.Ctx.Conn.Password = req.Password
	}
	if req.Username != "" {
		pending.Ctx.Conn.Username = req.Username
	}

	username := pending.Ctx.Conn.Username
	if username != "" && !s.locks.Check(username, r.RemoteAddr) {
		recordFailAudit(pending.Ctx.Conn, username, r.RemoteAddr, "[WebAuth]账号已被锁定，请稍后重试")
		s.WriteError(w, "账号已被锁定，请稍后重试")
		return
	}

	_, _, certTLS := s.RecoverCert(pending)

	ctx := pending.Ctx
	ctx.Conn.TLS = certTLS
	if certTLS == nil {
		ctx.Conn.TLS = r.TLS
	}
	ctx.Conn.RemoteAddr = r.RemoteAddr

	if req.OtpCode != "" {
		ctx.GetOTP().Code = req.OtpCode
		ctx.GetRADIUS().ChallengeCode = req.OtpCode
	}
	if req.SmsCode != "" {
		ctx.GetSMS().Code = req.SmsCode
	}

	pending.Ctx = ctx
	if req.OtpCode != "" || req.SmsCode != "" || len(pending.Ctx.PassedSteps()) > 0 || pending.Ctx.StepIdx() > 0 {
		s.ResumeDispatch(w, r, state, pending, username)
	} else {
		s.Dispatch(w, r, state, pending, username)
	}
}

// 构造 WebAuth 端 Flow（统一封装三端回调：通过/失败/挑战）
func (s *WebAuthService) newFlow(state string, pending *AuthSession, username string) *Flow {
	return &Flow{
		Ctx:          pending.Ctx,
		Username:     username,
		Source:       "WebAuth",
		Session:      pending,
		locks:        s.locks,
		authSessions: s.sessions,
		Callbacks: FlowCallbacks{
			OnPass: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
				s.OnPass(w, r, state, pending, fl.Result)
			},
			OnFail: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
				errMsg := "认证失败"
				if fl.Result.Err != nil {
					errMsg = removeStepPrefix(fl.Result.Err.Error())
				}
				base.Warn("WebAuth 认证失败:", fl.Result.Err)
				fl.RecordFail()
				s.WriteError(w, errMsg)
			},
			OnChallenge: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
				s.Challenge(w, r, state, pending, fl)
			},
		},
	}
}
