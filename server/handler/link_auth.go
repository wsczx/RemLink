package handler

import (
	"encoding/base64"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/sessdata"
)

// 处理 AnyConnect XML 认证
type LinkAuthService struct {
	certHash     atomic.Value
	sessions     *sessdata.SessionManager
	authSessions *AuthSessionManager
	locks        *auth.LockManager
	webAuth      *WebAuthService
}

func NewLinkAuthService(sessions *sessdata.SessionManager, authSessions *AuthSessionManager, locks *auth.LockManager, webAuth *WebAuthService) *LinkAuthService {
	s := &LinkAuthService{sessions: sessions, authSessions: authSessions, locks: locks, webAuth: webAuth}
	s.certHash.Store("")
	return s
}

// 写入主证书 SHA1 指纹
func (s *LinkAuthService) SetCertHash(hash string) { s.certHash.Store(hash) }

// 返回主证书 SHA1 指纹
func (s *LinkAuthService) CertHash() string { return s.certHash.Load().(string) }

// 认证入口
func (s *LinkAuthService) Handle(w http.ResponseWriter, r *http.Request) {
	cr, userAgent, ok := parseLinkAuthRequest(w, r)
	if !ok {
		return
	}
	ua := newLinkAuthLog(cr, r.RemoteAddr)

	// 退出登录
	if cr.Type == "logout" {
		s.logout(w, cr)
		return
	}

	sessionData := &AuthSession{
		UserActLog: ua,
		Ctx:        newAuthContext(cr, r),
	}

	// 空组处理
	if cr.GroupSelect == "" {
		if cr.Auth.SsoToken != "" || cr.Type == "init" {
			base.Trace("允许 SSO 认证请求通过，访问IP", r.RemoteAddr)
		} else if !strings.Contains(userAgent, "openconnect") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// 锁定状态判断
	if s.checkLocked(w, r, cr, ua) {
		return
	}

	// SSO Token 认证
	if cr.Auth.SsoToken != "" {
		s.handleSSOToken(w, r, cr, sessionData)
		return
	}

	// init 请求处理
	if cr.Type == "init" {
		// OpenConnect 不支持 WebAuth SAML 流程，走标准 init
		if base.GetCfg().EnableWebAuth &&
			strings.Contains(userAgent, "anyconnect") &&
			!strings.Contains(userAgent, "openconnect") &&
			!strings.Contains(userAgent, "open anyconnect") {
			s.webAuth.handle(w, r, cr, ua)
			return
		}
		if s.handleCertAutoAuth(w, r, cr, sessionData) {
			return
		}
		s.handleInit(w, r, cr, "")
		return
	}

	// auth-reply：执行 Pipeline
	if cr.Type != "auth-reply" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if checkUnsupported(w, r, cr, ua) {
		return
	}

	// 尝试从已保存会话恢复管道
	if s.resumeSavedAuth(w, r, cr, sessionData) {
		return
	}

	// 首次认证：通过认证服务加载组配置并执行管道
	result := authsrv.Authenticate(sessionData.Ctx)
	s.handlePipelineResult(w, r, result, sessionData)
}

// 当组配置了证书认证且 TLS 连接包含有效客户端证书时，直接从证书提取身份信息，跳过组选择表单
// 证书无效（过期/吊销/不匹配）时回退到 handleInit 组选择
func (s *LinkAuthService) handleCertAutoAuth(w http.ResponseWriter, r *http.Request, cr *ClientRequest, sessionData *AuthSession) bool {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return false
	}

	clientCert := r.TLS.PeerCertificates[0]
	username := clientCert.Subject.CommonName
	if len(clientCert.Subject.OrganizationalUnit) == 0 {
		return false
	}
	groupname := clientCert.Subject.OrganizationalUnit[0]
	if username == "" || groupname == "" {
		return false
	}

	if !authsrv.CertAutoAuth(groupname) {
		return false
	}

	sessionData.Ctx.Conn.Username = username
	sessionData.Ctx.Conn.GroupName = groupname
	sessionData.UserActLog.Username = username
	sessionData.UserActLog.GroupName = groupname

	base.Info("证书自动认证：用户", username, "组", groupname)

	result := authsrv.Authenticate(sessionData.Ctx)
	if result.Result == auth.StepFail {
		// 证书自动认证失败：回退到组选择流程，不强制留在证书组
		certErr := authFailMessage(result.Err)
		if base.GetCfg().DisplayError && result.Err != nil {
			certErr = removeStepPrefix(result.Err.Error())
		}
		base.Info("证书自动认证失败，回退组选择 ou=", groupname, " err=", result.Err)
		s.handleInit(w, r, cr, certErr)
		return true
	}
	s.handlePipelineResult(w, r, result, sessionData)
	return true
}

// 处理 init 请求：返回组选择登录表单或 SSO 扫码模板
func (s *LinkAuthService) handleInit(w http.ResponseWriter, r *http.Request, cr *ClientRequest, errMsg string) {
	// OpenConnect 组选择优化
	if cr.GroupSelect != "" && strings.Contains(cr.UserAgent, "openconnect") {
		data := RequestData{
			Group:  cr.GroupSelect,
			Groups: []string{cr.GroupSelect},
			Error:  errMsg,
		}
		w.WriteHeader(http.StatusOK)
		tplRequest(tpl_request, w, data)
		return
	}

	// SSO 组：返回 SAML 模板让客户端弹扫码窗口
	if ssoType := authsrv.GetSSOType(cr.GroupSelect); ssoType != "" {
		browserMode := samlBrowserMode(r, ssoType, cr.GroupSelect)
		data := RequestData{
			Group:       cr.GroupSelect,
			Groups:      dbdata.Groups.GetGroupNamesNormal(),
			ServerAddr:  getServerAddr(r),
			BrowserMode: browserMode,
			SsoType:     ssoType,
		}
		w.WriteHeader(http.StatusOK)
		tplRequest(tpl_request_saml, w, data)
		return
	}

	data := RequestData{Group: cr.GroupSelect, Groups: dbdata.Groups.GetGroupNamesNormal(), Error: errMsg}
	w.WriteHeader(http.StatusOK)
	tplRequest(tpl_request, w, data)
}

// 处理 SSO Token 认证：恢复 SSO 已认证身份，执行后续管道步骤
func (s *LinkAuthService) handleSSOToken(w http.ResponseWriter, r *http.Request, cr *ClientRequest, sessionData *AuthSession) {
	rawToken := cr.Auth.SsoToken

	// 先尝试原始令牌直接查找（外部浏览器模式：客户端从 localhost:29786/api/sso/{state} 提取原始 state）
	sessionKey := rawToken
	samlSession, err := s.authSessions.Get(sessionKey)
	if err != nil {
		// 回退：尝试 Base64 解码后查找（内置浏览器模式：客户端直接发送 cookie 中的 Base64 编码值）
		// 手机端 AnyConnect 客户端会去掉 base64 padding（=），需要补齐
		if padLen := len(rawToken) % 4; padLen != 0 {
			rawToken += strings.Repeat("=", 4-padLen)
		}
		decodedBytes, decErr := base64.StdEncoding.DecodeString(rawToken)
		if decErr != nil {
			base.Error("SSO 会话不存在: token=", rawToken[:min(16, len(rawToken))], ", err=", decErr)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		sessionKey = string(decodedBytes)
		samlSession, err = s.authSessions.Get(sessionKey)
		if err != nil {
			base.Error("SSO 会话不存在: token=", rawToken[:min(16, len(rawToken))], ", err=", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	// SSO token 仅能兑换 SSO 流程产生的会话
	// 拒绝把 OTP-pending 等认证管道会话的 id 当作 SSO token 重放冒用
	if !isSSOTokenSessionAllowed(samlSession) {
		base.Error("SSO token 指向非 SSO 会话，已拒绝: kind=", samlSession.Kind)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// 强制改密会话
	if samlSession.ForcePwd {
		s.resumeAuthSession(w, r, samlSession)
		return
	}

	// WebAuth完成直接创建会话（要求会话确为 WebAuth 流程完成，防止管道会话冒用）
	if samlSession.Kind == AuthKindWebAuth && samlSession.Ctx != nil &&
		samlSession.Ctx.SSO != nil && samlSession.Ctx.SSO.WebAuthCompleted {
		username := samlSession.Ctx.SSO.WebAuthUsername
		groupName := samlSession.Ctx.SSO.WebAuthGroup
		nickname := samlSession.Ctx.SSO.WebAuthNickname
		base.Info("WebAuth认证已完成", dbdata.Users.Label(username, nickname), "group:", groupName)

		if username == "" || groupName == "" {
			base.Error("[handleSSOToken] WebAuth 会话缺少用户名或组名")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// 将用户信息写入会话
		if samlSession.Ctx.UserInfo != nil {
			sessionData.Ctx.SetUserInfo(samlSession.Ctx.UserInfo)
		}

		sessionData.Ctx.Conn.Username = username
		sessionData.Ctx.Conn.GroupName = groupName
		sessionData.UserActLog.Username = username
		sessionData.UserActLog.GroupName = groupName
		sessionData.UserActLog.Info = "WebAuth 认证成功"
		sessionData.UserActLog.Status = dbdata.UserAuthSuccess
		dbdata.UserActLogIns.Add(*sessionData.UserActLog, sessionData.Ctx.Conn.UserAgent)

		s.locks.Success(username, r.RemoteAddr)
		s.CreateSession(w, sessionData)
		s.authSessions.Delete(sessionKey)
		return
	}

	ssoCtx := samlSession.Ctx
	var userID string
	if ssoCtx != nil && ssoCtx.SSO != nil {
		if ssoCtx.SSO.Authenticated {
			userID = ssoCtx.SSO.UserID
		}
	}

	// 将已验证身份写回会话，供后续 OTP 等步骤使用
	sessionData.Ctx.Conn.Username = userID
	sessionData.Ctx.Conn.GroupName = samlSession.Ctx.Conn.GroupName
	sessionData.UserActLog.Username = userID
	sessionData.UserActLog.GroupName = samlSession.Ctx.Conn.GroupName

	// 执行认证管道：优先从已保存管道会话恢复，跳过已通过的步骤
	if sessionID, cerr := GetCookie(r, "auth-session-id"); cerr == nil && sessionID != "" {
		if sess, serr := s.authSessions.Get(sessionID); serr == nil {
			// 将 SSO 已验证身份注入已保存会话
			sess.Ctx.Conn.Username = userID
			sess.Ctx.Conn.GroupName = samlSession.Ctx.Conn.GroupName
			// 合并 SSO 状态
			if ssoCtx != nil && ssoCtx.SSO != nil {
				sess.Ctx.SSO = ssoCtx.SSO
			}
			sess.Ctx.Conn.TLS = r.TLS
			sess.Ctx.Conn.RemoteAddr = r.RemoteAddr
			sess.UserActLog = sessionData.UserActLog
			// 清除旧管道会话
			s.authSessions.Delete(sessionID)
			sess.SessionID = ""
			s.resumeAuthSession(w, r, sess)
			s.authSessions.Delete(sessionKey)
			return
		}
		DeleteCookie(w, "auth-session-id")
	}

	// 回退：无管道会话时从头执行管道
	if ssoCtx != nil && ssoCtx.SSO != nil {
		sessionData.Ctx.SSO = ssoCtx.SSO
	}
	authsrv.LoadUserInfo(sessionData.Ctx)

	result := authsrv.Authenticate(sessionData.Ctx)
	s.handlePipelineResult(w, r, result, sessionData)

	// 清理 SSO 临时会话
	s.authSessions.Delete(sessionKey)
}
