package handler

import (
	"fmt"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"net"
	"net/http"
)

// 为 WebAuth 流程构建 SSO OAuth 跳转 URL
// 生成子会话（ssoState）关联回当前 WebAuth 会话，回调到 /web-auth/sso-callback
func (s *WebAuthService) BuildSSOURL(r *http.Request, ssoType, groupName, webAuthState string) string {

	ssoState := GenerateSessionID()
	pending := &AuthSession{
		Kind: AuthKindSSOPending,
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{GroupName: groupName},
			SSO: &auth.SSOState{
				Type:     ssoType,
				From:     "web_auth",
				ClientIP: r.RemoteAddr,
			},
		},
	}
	s.sessions.Save(ssoState, pending)

	redirectUri := fmt.Sprintf("%s/web-auth/sso-callback?web_state=%s&sso_state=%s",
		getServerAddr(r), webAuthState, ssoState)

	authURL, err := s.sso.BuildAuthURL(ssoType, groupName, redirectUri, ssoState)
	if err != nil {
		base.Error("生成 SSO 授权地址失败:", err)
		return ""
	}
	return authURL
}

// SSO OAuth 回调端点：企微/飞书扫码授权后回调到此
// 将认证结果写入 WebAuth 会话 SSO 状态，然后 302 回到 SPA 继续管道
func (s *WebAuthService) SSOCallback(w http.ResponseWriter, r *http.Request) {
	webState := r.URL.Query().Get("web_state")
	ssoState := r.URL.Query().Get("sso_state")

	if webState == "" || ssoState == "" {
		base.Error("WebAuth SSO 回调缺少参数")
		http.Error(w, "缺少认证参数", http.StatusBadRequest)
		return
	}

	pending, err := s.sessions.Get(ssoState)
	if err != nil {
		base.Error("WebAuth SSO 非法 state")
		http.Error(w, "认证会话已过期", http.StatusBadRequest)
		return
	}

	if pending.Kind != AuthKindSSOPending || pending.Ctx.SSO == nil || pending.Ctx.SSO.From != "web_auth" {
		base.Error("WebAuth SSO state 类型非法")
		http.Error(w, "认证会话已过期", http.StatusBadRequest)
		return
	}

	if pending.Ctx.SSO.Authenticated {
		base.Error("WebAuth SSO state 已被使用（疑似重放）")
		s.sessions.Delete(ssoState)
		http.Error(w, "认证会话已过期", http.StatusBadRequest)
		return
	}

	if ip := pending.Ctx.SSO.ClientIP; ip != "" {
		want, _, _ := net.SplitHostPort(ip)
		got, _, _ := net.SplitHostPort(r.RemoteAddr)
		if want != "" && got != "" && want != got {
			base.Error("WebAuth SSO state 来源 IP 不匹配:", want, got)
			s.sessions.Delete(ssoState)
			http.Error(w, "认证会话来源异常", http.StatusForbidden)
			return
		}
	}

	ssoType := pending.Ctx.SSO.Type
	code := r.URL.Query().Get("code")

	cb, cbErr := s.sso.CallbackFor(ssoType, pending.Ctx.Conn.GroupName)
	if cbErr != nil {
		s.sessions.Delete(ssoState)
		http.Error(w, "获取用户信息失败", http.StatusInternalServerError)
		return
	}
	username, nickname, err := cb.getUser(code)
	if err != nil {
		s.sessions.Delete(ssoState)
		http.Error(w, "获取用户信息失败", http.StatusInternalServerError)
		return
	}
	pending.Ctx.Conn.Nickname = nickname
	if username == "" {
		s.sessions.Delete(ssoState)
		http.Error(w, "获取用户信息失败", http.StatusInternalServerError)
		return
	}

	if aerr := ssoAuthorize(cb, username); aerr != nil {
		base.Error("WebAuth SSO 访问控制拒绝:", username, aerr)
		s.sessions.Delete(ssoState)
		http.Error(w, "用户未通过访问控制校验", http.StatusForbidden)
		return
	}

	webPending, werr := s.sessions.Get(webState)
	if werr != nil || webPending == nil || webPending.Ctx == nil ||
		webPending.Kind != AuthKindPipeline ||
		webPending.Ctx.Conn.GroupName != pending.Ctx.Conn.GroupName {
		base.Error("WebAuth SSO 回调目标会话非法（疑似跨组冒用）: webState=", webState)
		s.sessions.Delete(ssoState)
		http.Error(w, "认证会话已过期", http.StatusBadRequest)
		return
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
	s.sessions.Save(webState, webPending)

	s.sessions.Delete(ssoState)

	redirectURL := fmt.Sprintf("/ui/#/web-auth/continue?state=%s", webState)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}
