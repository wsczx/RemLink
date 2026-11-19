package handler

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn"
)

const (
	webAuthEntrySMSPhoneHint    = "请输入手机号以接收短信验证码" // 用户选择 SMS 组但未提交手机号时的引导提示
	webAuthEntryCredentialsHint = "请输入登录凭据"        //常规组凭据输入界面引导提示
)

// WebAuthService 所需的会话存储契约；*AuthSessionManager 隐式实现
// 依赖接口而非具体管理器，便于替换为测试桩或换实现
type authSessionStore interface {
	Save(id string, data *AuthSession)
	Get(id string) (*AuthSession, error)
	Delete(id string)
}

// 处理浏览器认证端点
type WebAuthService struct {
	sessions authSessionStore
	locks    *auth.LockManager
	webvpn   *webvpn.WebVPNManager
	sso      *SSOHandler
}

func NewWebAuthService(sessions authSessionStore, locks *auth.LockManager, webvpnMgr *webvpn.WebVPNManager, sso *SSOHandler) *WebAuthService {
	return &WebAuthService{sessions: sessions, locks: locks, webvpn: webvpnMgr, sso: sso}
}

// AnyConnect 弹出系统浏览器打开认证页面 sso-v2-login URL 指向 WebAuth 认证地址
func (s *WebAuthService) handle(w http.ResponseWriter, r *http.Request, cr *ClientRequest, ua *dbdata.UserActLog) {

	if cr.GroupSelect != "" && authsrv.GetSSOType(cr.GroupSelect) != "" && isMobileDevice(r) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	state := GenerateSessionID()

	certTLS := r.TLS

	pending := &AuthSession{
		Kind:       AuthKindPipeline,
		UserActLog: ua,
		Ctx: &auth.Context{
			WebAuth: true,
			Conn: auth.ConnInfo{
				GroupName:   cr.GroupSelect,
				RemoteAddr:  r.RemoteAddr,
				UserAgent:   cr.UserAgent,
				MacAddr:     cr.MacAddressList.MacAddress,
				DeviceID:    cr.DeviceId.UniqueIdGlobal,
				DeviceType:  cr.DeviceId.Type(),
				PlatformVer: cr.DeviceId.PlatformVersion,
				TLS:         certTLS,
			},
		},
	}
	s.sessions.Save(state, pending)

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
        <sso-v2-browser-mode>` + s.BrowserMode(r) + `</sso-v2-browser-mode>
        <form>
            <input type="sso" name="sso-token"></input>
        </form>
    </auth>
</config-auth>`

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(xml))
}

// 从 pending 会话恢复 AnyConnect 客户端证书信息
// 在初始化时将 TLS 状态（含 PeerCertificates）存入 Ctx.Conn.TLS
func (s *WebAuthService) RecoverCert(pending *AuthSession) (cn, ou string, tlsState *tls.ConnectionState) {
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

func (s *WebAuthService) WriteJSON(w http.ResponseWriter, statusCode int, data map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

func (s *WebAuthService) WriteError(w http.ResponseWriter, msg string) {
	s.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "error",
		"message": msg,
	})
}

// 读取并校验 WebAuth 认证会话调用方直接 return
func (s *WebAuthService) Pending(w http.ResponseWriter, r *http.Request) (*AuthSession, string, bool) {
	state := r.URL.Query().Get("state")
	if state == "" {
		s.WriteError(w, "缺少认证参数")
		return nil, "", false
	}
	pending, err := s.sessions.Get(state)
	if err != nil {
		s.WriteError(w, "认证会话已过期")
		return nil, "", false
	}
	return pending, state, true
}

// 返回 SAML XML 中 sso-v2-browser-mode 的值
// 手机端强制使用内置浏览器（外部浏览器的 localhost:29786 回调在手机上不可用）
func (s *WebAuthService) BrowserMode(r *http.Request) string {
	if isMobileDevice(r) {
		return "internal"
	}
	mode := base.GetCfg().WebAuthBrowserMode
	if mode == "internal" {
		return "internal"
	}
	return "external"
}
