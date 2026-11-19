package handler

import (
	"encoding/json"
	"net/http"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/jwt"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/service"
)

func (p *PortalService) Login(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		portalError(w, "参数错误")
		dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: req.Username, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "[门户]参数解析失败"}, r.UserAgent(), true)
		return
	}
	resp := p.portalStartAuth(w, req.Username, req.Password, r)
	if resp.Code != 0 {
		portalError(w, resp.Msg)
		dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: req.Username, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "[门户]" + resp.Msg, IsLockedFail: resp.IsLocked}, r.UserAgent(), true)
		return
	}
	portalOK(w, resp.Data)
}

func (p *PortalService) Verify(w http.ResponseWriter, r *http.Request) {
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
	resp := p.portalResumeAuth(w, req.SessionID, req.Code, r)
	if resp.Code != 0 {
		portalError(w, resp.Msg)
		dbdata.UserActLogIns.Add(dbdata.UserActLog{RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "[门户]" + resp.Msg}, r.UserAgent(), true)
		return
	}
	portalOK(w, resp.Data)
}

func (p *PortalService) Logout(w http.ResponseWriter, r *http.Request) {
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

	if base.GetCfg().WebVpnDomain != "" {
		user, ok := portalCurrentUser(r)
		if !ok {
			user, ok = p.webvpn.Session().UserFromRequest(r)
		}
		if ok && user != nil {
			p.webvpn.Session().RevokeUser(user.Username)
		}
	}

	p.webvpn.Session().ClearGrantCookie(w, r)
	p.webvpn.Session().ClearCookie(w, r)
	if c, err := r.Cookie(portalCookieName); err == nil && c.Value != "" {
		jwt.RevokeJwtToken(c.Value)
	}
	portalOK(w, map[string]string{"message": "已退出"})
}

type portalAuthResponse struct {
	Code     int
	Msg      string
	Data     map[string]any
	IsLocked bool `json:"-"` // 是否因账号/IP 被锁定而失败，用于日志限频
}

func (p *PortalService) portalStartAuth(w http.ResponseWriter, username, password string, r *http.Request) portalAuthResponse {
	if username == "" || len(password) < 1 {
		return portalAuthError("用户名或密码错误")
	}

	if !p.locks.Check(username, r.RemoteAddr) {
		recordFailAudit(auth.ConnInfo{Username: username, RemoteAddr: r.RemoteAddr, UserAgent: r.UserAgent()},
			username, r.RemoteAddr, "[门户]账号已被锁定，请稍后重试", true)
		resp := portalAuthError("账号已被锁定，请稍后重试")
		resp.IsLocked = true
		return resp
	}

	pre, err := service.PreparePortalLogin(username)
	if err != nil {
		return portalAuthError(err.Error())
	}
	user, userExists := pre.User, pre.UserExists

	var lastErr error
	var finalResp portalAuthResponse
	for _, group := range pre.Groups {
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
			Ctx:          ctx,
			Username:     username,
			Source:       "门户",
			locks:        p.locks,
			authSessions: p.authSessions,
			Callbacks: FlowCallbacks{
				OnPass: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
					portalUser, err := service.ResolvePortalUser(username, group, user, userExists)
					if err != nil {
						lastErr = err
						return
					}
					finalResp = p.portalIssueLoginResponse(w, r, portalUser, ctx.LogInfo())
				},
				OnFail: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
					if fl.Result.Err != nil {
						lastErr = fl.Result.Err
						base.Warn("用户门户认证失败:", username, group, fl.Result.Err)
					}
				},
				OnChallenge: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
					sessionID := p.portalSaveChallengeSession(fl.Result, ctx)
					finalResp = portalChallengeResponse(sessionID, fl.Result, ctx)
				},
			},
		}
		flow.Run(w, r)

		if flow.Result.Result == auth.StepPass && finalResp.Code == 0 {
			return finalResp
		}

		if flow.Result.Result == auth.StepPending {
			return finalResp
		}

	}
	p.locks.Fail(username, r.RemoteAddr)
	if lastErr != nil && base.GetCfg().DisplayError {
		return portalAuthError(lastErr.Error())
	}
	return portalAuthError("用户名或密码错误")
}

func (p *PortalService) portalResumeAuth(w http.ResponseWriter, sessionID, code string, r *http.Request) portalAuthResponse {
	if sessionID == "" || code == "" {
		return portalAuthError("参数不能为空")
	}
	sess, err := p.authSessions.Get(sessionID)
	if err != nil || sess.Ctx == nil {
		return portalAuthError("认证会话已过期，请重新登录")
	}

	username := sess.Ctx.Conn.Username
	ctx := sess.Ctx
	ctx.Conn.RemoteAddr = r.RemoteAddr

	ctx.GetOTP().Code = code
	ctx.GetRADIUS().ChallengeCode = code
	ctx.GetSMS().Code = code

	if err := authsrv.ReloadAndCheckUser(ctx); err != nil {
		return portalAuthError(err.Error())
	}

	state := auth.PipelineState{
		StepIdx:     sess.Ctx.StepIdx(),
		PassedSteps: sess.Ctx.PassedSteps(),
	}

	var resp portalAuthResponse
	flow := &Flow{
		Ctx:          ctx,
		Username:     username,
		Source:       "门户",
		Session:      sess,
		locks:        p.locks,
		authSessions: p.authSessions,
		Callbacks: FlowCallbacks{
			OnPass: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
				p.authSessions.Delete(sessionID)
				user, userExists, err := service.LoadPortalUser(fl.Result.Username)
				if err != nil {
					resp = portalAuthError(err.Error())
					return
				}
				portalUser, err := service.ResolvePortalUser(fl.Result.Username, fl.Result.GroupName, user, userExists)
				if err != nil {
					resp = portalAuthError(err.Error())
					return
				}
				resp = p.portalIssueLoginResponse(w, r, portalUser, ctx.LogInfo())
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
	if resp.Data != nil || resp.Code != 0 {
		return resp
	}
	return portalAuthError("验证失败")
}

func (p *PortalService) portalSaveChallengeSession(result *auth.PipelineResult, ctx *auth.Context) string {
	sessionID := GenerateSessionID()
	if ctx == nil {
		ctx = &auth.Context{}
	}
	ctx.PortalLogin = true
	ctx.SetStepIdx(result.State.StepIdx)
	ctx.SetPassedSteps(result.State.PassedSteps)
	p.authSessions.Save(sessionID, &AuthSession{
		Ctx: ctx,
	})
	return sessionID
}

func portalChallengeResponse(sessionID string, result *auth.PipelineResult, ctx *auth.Context) portalAuthResponse {
	if result.Challenge != nil && result.Challenge.Type == auth.ChallengeSSO {
		return portalAuthResponse{Data: map[string]any{
			"status":     "sso",
			"session_id": sessionID,
			"message":    "请完成第三方登录",
		}}
	}

	view := BuildChallengeView(result, ctx, result.IsChallengeRetry())
	return portalAuthResponse{Data: view.ToPortalJSON(sessionID)}
}

func (p *PortalService) portalIssueLoginResponse(w http.ResponseWriter, r *http.Request, user *dbdata.User, logInfo string) portalAuthResponse {
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

	token, err := service.IssuePortalToken(user)
	if err != nil {
		return portalAuthError("登录失败")
	}
	var webVPNGrant string
	_, fromWebVpn := webVpnHostPrefix(r.Host)
	if !fromWebVpn {
		http.SetCookie(w, &http.Cookie{
			Name:     portalCookieName,
			Value:    token,
			Path:     "/",
			Domain:   "",
			HttpOnly: true,
			Secure:   utils.IsTLS(r),
			SameSite: http.SameSiteLaxMode,
		})
	}
	if base.GetCfg().WebVpnDomain != "" {
		if jti, jerr := jwt.JtiOf(token); jerr == nil {
			var gerr error
			webVPNGrant, gerr = p.webvpn.Session().IssueGrant(w, r, user, jti)
			if gerr != nil {
				base.Warn("WebVPN 免登授权签发失败:", gerr)
				webVPNGrant = ""
			}
		}
	}
	data := map[string]any{
		"status": "pass",
		"token":  token,
		"user":   service.PortalUserInfo(user, getServerAddr(r)),
	}
	if webVPNGrant != "" {
		data["webvpn_grant"] = webVPNGrant
	}
	return portalAuthResponse{Data: data}
}

func portalAuthError(msg string) portalAuthResponse {
	return portalAuthResponse{Code: 1, Msg: msg}
}
