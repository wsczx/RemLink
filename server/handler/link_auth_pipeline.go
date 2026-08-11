// 认证管道交互层，将 auth.PipelineResult 映射为 HTTP 响应。

package handler

import (
	"net/http"
	"strings"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 认证阶段名前缀，用于过滤错误信息
var stepNamePrefixes = []string{
	"cert:", "local:", "ldap:", "radius:", "otp:", "wxwork:", "feishu:", "saml:", "admin:",
}

// 认证失败时，将错误信息映射为用户可读的文案。
func stripStepPrefix(msg string) string {
	for _, p := range stepNamePrefixes {
		if strings.HasPrefix(msg, p) {
			return msg[len(p):]
		}
	}
	return msg
}

// 认证失败时，根据错误类型返回面向用户的错误信息。
func authFailMessage(err error) string {
	if err == nil {
		return "认证失败"
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "cert:"):
		return "客户端证书认证失败，请检查客户端证书"
	case strings.HasPrefix(msg, "wxwork:"), strings.HasPrefix(msg, "feishu:"):
		return "单点登录认证失败"
	case strings.HasPrefix(msg, "otp:"):
		return "动态码验证失败"
	default:
		// local/ldap/radius 等凭据认证：统一文案，避免用户枚举
		return "用户名或密码错误"
	}
}

// 从 ClientRequest 构建认证上下文（首次认证用）。
func newAuthContext(cr *ClientRequest, r *http.Request) *auth.Context {
	return &auth.Context{
		Conn: auth.ConnInfo{
			Username:    cr.Auth.Username,
			Password:    cr.Auth.Password,
			GroupName:   cr.GroupSelect,
			RemoteAddr:  r.RemoteAddr,
			UserAgent:   cr.UserAgent,
			MacAddr:     cr.MacAddressList.MacAddress,
			DeviceID:    cr.DeviceId.UniqueIdGlobal,
			DeviceType:  cr.DeviceId.DeviceType,
			PlatformVer: cr.DeviceId.PlatformVersion,
			TLS:         r.TLS,
		},
	}
}

// 从已保存的认证会话恢复管道执行。
// 调用前需将当前请求的密码/OTP 码写入 ctx。
func resumeAuthSession(w http.ResponseWriter, r *http.Request,
	sess *AuthSession) {

	ctx := sess.Ctx
	// TLS 需从当前请求重新注入
	ctx.Conn.TLS = r.TLS
	ctx.Conn.RemoteAddr = r.RemoteAddr
	// 重新加载用户信息，用户在断点期间可能已改密或清除强制改密标记
	authsrv.ReloadUserInfo(ctx)

	state := auth.PipelineState{
		StepIdx:     sess.Ctx.StepIdx(),
		PassedSteps: sess.Ctx.PassedSteps(),
	}
	result := authsrv.Resume(ctx, state)
	handlePipelineResult(w, r, result, sess)
}

// 将管道执行结果映射为 HTTP 响应。
func handlePipelineResult(w http.ResponseWriter, r *http.Request,
	result *auth.PipelineResult, sessionData *AuthSession) {

	ctx := sessionData.Ctx
	username := ctx.Conn.Username
	if result.Username != "" {
		username = result.Username
	}

	flow := &Flow{
		Ctx:      ctx,
		Username: username,
		Callbacks: FlowCallbacks{
			OnPass: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
				// 恢复场景：清除旧认证会话（新建会话在 CreateSession 内另发 cookie）
				if sessionData.SessionID != "" {
					AuthSessionManager.Delete(sessionData.SessionID)
					DeleteCookie(w, "auth-session-id")
				}
				sessionData.UserActLog.Username = fl.Result.Username
				sessionData.UserActLog.GroupName = fl.Result.GroupName
				sessionData.UserActLog.Info = fl.Result.Info
				if sessionData.UserActLog.Info == "" {
					sessionData.UserActLog.Info = "认证成功"
				}
				sessionData.UserActLog.Status = dbdata.UserAuthSuccess
				dbdata.UserActLogIns.Add(*sessionData.UserActLog, ctx.Conn.UserAgent)

				ctx.Conn.Username = fl.Result.Username
				ctx.Conn.GroupName = fl.Result.GroupName
				CreateSession(w, sessionData)
			},
			OnFail: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
				// 恢复场景：清除旧认证会话
				if sessionData.SessionID != "" {
					AuthSessionManager.Delete(sessionData.SessionID)
					DeleteCookie(w, "auth-session-id")
				}
				errMsg := "认证失败"
				if fl.Result.Err != nil {
					errMsg = fl.Result.Err.Error()
				}
				base.Warn("认证失败:", fl.Result.Err, r.RemoteAddr)
				sessionData.UserActLog.Info = errMsg
				sessionData.UserActLog.Status = dbdata.UserAuthFail
				dbdata.UserActLogIns.Add(*sessionData.UserActLog, ctx.Conn.UserAgent)

				w.WriteHeader(http.StatusOK)
				data := RequestData{
					Group:  ctx.Conn.GroupName,
					Groups: dbdata.GetGroupNamesNormal(),
					Error:  authFailMessage(fl.Result.Err),
				}
				if base.GetCfg().DisplayError && fl.Result.Err != nil {
					data.Error = stripStepPrefix(errMsg)
				}
				tplRequest(tpl_request, w, data)
			},
			OnChallenge: func(fl *Flow, w http.ResponseWriter, r *http.Request) {
				handleChallengeResult(w, r, fl, sessionData)
			},
		},
	}

	// 分发已计算的管道结果
	flow.Dispatch(w, r, result)
}

// 处理 StepPending：保存/更新会话状态 + 按挑战视图返回模板
// 由 Flow.OnChallenge 回调调用，flow 已统一处理锁定计数窗口
func handleChallengeResult(w http.ResponseWriter, r *http.Request,
	flow *Flow, sessionData *AuthSession) {

	result := flow.Result

	if result.Challenge == nil && result.Result == auth.StepPending {
		base.Error("StepPending 但无 Challenge 信息")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ctx := sessionData.Ctx
	isResume := sessionData.SessionID != ""

	// 保存断点信息到 Ctx，供下次 Resume 恢复
	flow.savePendingState()

	if isResume {
		// 恢复场景：更新已有会话
		AuthSessionManager.Save(sessionData.SessionID, sessionData)
	} else {
		// 首次认证：创建新会话
		sid := GenerateSessionID()
		sessionData.SessionID = sid
		AuthSessionManager.Save(sid, sessionData)
		SetCookie(w, "auth-session-id", sid, 0)
	}
	// 清除强制改密标记
	sessionData.ForcePwd = false

	view := BuildChallengeView(result, ctx, result.IsChallengeRetry())

	w.WriteHeader(http.StatusOK)
	switch view.Type {
	case auth.ChallengeSSO:
		// 手机端无法完成企微/飞书扫码，默认拒绝；开启 allow_mobile_sso 后放行
		if isMobileDevice(r) && !base.GetCfg().AllowMobileSSO {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		data := view.ToXML()
		data.ServerAddr = getServerAddr(r) // ServerAddr 由接入请求按 r 注入，ToXML 不预设
		data.BrowserMode = samlBrowserMode(r, view.SsoType, result.GroupName)
		tplRequest(tpl_request_saml, w, data)

	case auth.ChallengeForcePwd:
		data := view.ToXML()
		data.ServerAddr = getServerAddr(r) // ServerAddr 由接入请求按 r 注入，ToXML 不预设
		data.State = sessionData.SessionID
		sessionData.ForcePwd = true
		tplRequest(tpl_request_force_pwd, w, data)

	case auth.ChallengeOTP:
		tplRequest(tpl_otp, w, view.ToXML())

	case auth.ChallengeRADIUS:
		tplRequest(tpl_accept_challenge, w, view.ToXML())

	case auth.ChallengeSMS:
		// 原生客户端复用二次验证表单输入短信码；脱敏手机号随模板提示展示
		tplRequest(tpl_accept_challenge, w, view.ToXML())

	default:
		// credentials 等：返回凭据输入界面
		if view.Type != "" {
			base.Warn("原生管道收到未预期挑战类型: ", view.Type, " user=", ctx.Conn.Username, " group=", result.GroupName)
		}
		tplRequest(tpl_request, w, view.ToXML())
	}
}
