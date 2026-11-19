package handler

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/jwt"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/service"
)

// 执行首次认证（Authenticate）分发
func (s *WebAuthService) Dispatch(w http.ResponseWriter, r *http.Request,
	state string, pending *AuthSession, username string) {
	s.newFlow(state, pending, username).Run(w, r)
}

// 从已保存会话恢复管道（Resume）分发
func (s *WebAuthService) ResumeDispatch(w http.ResponseWriter, r *http.Request,
	state string, pending *AuthSession, username string) {
	if err := authsrv.ReloadAndCheckUser(pending.Ctx); err != nil {
		s.WriteError(w, err.Error())
		return
	}
	flow := s.newFlow(state, pending, username)
	flow.Resume(w, r, auth.PipelineState{
		StepIdx:     pending.Ctx.StepIdx(),
		PassedSteps: pending.Ctx.PassedSteps(),
	})
}

// 处理管道 StepPending 结果，渲染为 WebAuth JSON
func (s *WebAuthService) Challenge(w http.ResponseWriter, r *http.Request,
	state string, pending *AuthSession, flow *Flow) {

	ctx := pending.Ctx
	result := flow.Result

	flow.savePendingState()

	challenge := result.Challenge
	if challenge == nil {

		view := BuildChallengeView(result, ctx, result.IsChallengeRetry())
		resp := view.ToWebAuthJSON()
		if base.GetCfg().EnableWebAuthGroupFilter && pending.Ctx.Conn.Username != "" {
			resp["username"] = pending.Ctx.Conn.Username
		}
		s.WriteJSON(w, http.StatusOK, resp)
		return
	}

	if challenge.Type == auth.ChallengeSSO {

		if isMobileDevice(r) && !base.GetCfg().AllowMobileSSO {
			s.WriteError(w, "手机端不支持SSO扫码认证，请使用其他认证方式")
			return
		}
		ssoURL := s.BuildSSOURL(r, ssoTypeOf(challenge), ctx.Conn.GroupName, state)
		s.WriteJSON(w, http.StatusOK, map[string]any{
			"status":       "sso",
			"sso_type":     ssoTypeOf(challenge),
			"redirect_url": ssoURL,
		})
		return
	}

	view := BuildChallengeView(result, ctx, result.IsChallengeRetry())
	s.WriteJSON(w, http.StatusOK, view.ToWebAuthJSON())
}

// 保存认证完成标记并签发门户会话
func (s *WebAuthService) OnPass(w http.ResponseWriter, r *http.Request,
	state string, pending *AuthSession, result *auth.PipelineResult) {

	username := result.Username
	groupName := result.GroupName
	if pending.Ctx == nil {
		base.Error("WebAuth 认证会话状态异常（Ctx 为 nil），state:", state[:min(16, len(state))])
		s.WriteError(w, "认证会话状态异常")
		return
	}

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

	pending.Ctx.GetSSO().WebAuthCompleted = true
	pending.Ctx.SSO.WebAuthUsername = username
	pending.Ctx.SSO.WebAuthNickname = pending.Ctx.Conn.Nickname
	pending.Ctx.SSO.WebAuthGroup = groupName
	pending.Kind = AuthKindWebAuth
	s.sessions.Save(state, pending)

	user := &dbdata.User{}
	found := false
	if err := dbdata.Users.GetByUsername(username, user); err == nil && user.Status == 1 {
		found = true
	}
	if !found {
		user = &dbdata.User{
			Type:     "external",
			Username: username,
			Groups:   s.ExternalGroups(groupName),
			Status:   1,
		}
	}
	var webVPNGrant string
	if token, pErr := service.IssuePortalToken(user); pErr == nil {
		http.SetCookie(w, &http.Cookie{
			Name:     portalCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   utils.IsTLS(r),
			SameSite: http.SameSiteLaxMode,
		})
		if base.GetCfg().WebVpnDomain != "" {
			if jti, jerr := jwt.JtiOf(token); jerr == nil {
				var gerr error
				webVPNGrant, gerr = s.webvpn.Session().IssueGrant(w, r, user, jti)
				if gerr != nil {
					base.Warn("WebVPN 免登授权签发失败:", gerr)
					webVPNGrant = ""
				}
			}
		}
	}

	resp := map[string]any{
		"status":       "done",
		"complete_url": fmt.Sprintf("/web-auth/complete?state=%s", url.QueryEscape(state)),
		"portal_url":   "/ui/#/portal",
		"success":      true,
	}
	if webVPNGrant != "" {
		resp["webvpn_grant"] = webVPNGrant
	}
	s.WriteJSON(w, http.StatusOK, resp)
}

// 将 SSO 授权的单组名转为门户 JWT 所需的组列表，空组会导致门户侧拒绝该会话
func (s *WebAuthService) ExternalGroups(groupName string) []string {
	if groupName == "" {
		return nil
	}
	return []string{groupName}
}

// SSO 回调后恢复管道
func (s *WebAuthService) Continue(w http.ResponseWriter, r *http.Request) {
	pending, state, ok := s.Pending(w, r)
	if !ok {
		return
	}
	if pending.Ctx == nil {
		s.sessions.Delete(state)
		s.WriteError(w, "认证会话已过期")
		return
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

	s.ResumeDispatch(w, r, state, pending, username)
}

// 设置 acSamlv2Token Cookie，302 到 SAML 完成端点
func (s *WebAuthService) Complete(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "缺少认证参数", http.StatusBadRequest)
		return
	}

	pending, err := s.sessions.Get(state)
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

	encodeState := base64.StdEncoding.EncodeToString([]byte(state))
	SetCookie(w, "acSamlv2Token", encodeState, 0)

	http.Redirect(w, r, "/+CSCOE+/saml_ac_login.html", http.StatusFound)
}
