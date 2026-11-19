package handler

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

// 校验 AnyConnect 请求头、读取并解析 XML 请求体
// 失败时已写错误响应；返回小写 UA 供后续的设备/浏览器判定使用
func parseLinkAuthRequest(w http.ResponseWriter, r *http.Request) (*ClientRequest, string, bool) {
	if base.GetLogLevel() == base.LogLevelTrace {
		hd, err := httputil.DumpRequest(r, true)
		if err == nil {
			base.Trace("LinkAuth: ", utils.RedactSensitive(string(hd)))
		}
	}

	userAgent := strings.ToLower(r.UserAgent())
	xAggregateAuth := r.Header.Get("X-Aggregate-Auth")
	xTranscendVersion := r.Header.Get("X-Transcend-Version")
	if !((strings.Contains(userAgent, "anyconnect") || strings.Contains(userAgent, "openconnect") || strings.Contains(userAgent, "anylink")) &&
		xAggregateAuth == "1" && xTranscendVersion == "1") {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "error request")
		return nil, "", false
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, "", false
	}
	// Body 已读到 EOF，后续流程只使用解析结果，此处即可关闭
	defer r.Body.Close()

	cr := &ClientRequest{
		RemoteAddr: r.RemoteAddr,
		UserAgent:  userAgent,
	}
	if err := xml.Unmarshal(body, cr); err != nil {
		base.Error(err)
		w.WriteHeader(http.StatusBadRequest)
		return nil, "", false
	}
	if base.GetLogLevel() == base.LogLevelTrace {
		if b, err := xml.MarshalIndent(cr, "", "  "); err == nil {
			base.Trace("LinkAuth parsed request:\n" + utils.RedactSensitive(string(b)))
		} else {
			base.Trace("LinkAuth: 结构化日志序列化失败，已跳过")
		}
	}
	return cr, userAgent, true
}

// 由认证请求构造用户活动日志
func newLinkAuthLog(cr *ClientRequest, remoteAddr string) *dbdata.UserActLog {
	return &dbdata.UserActLog{
		Username:        cr.Auth.Username,
		GroupName:       cr.GroupSelect,
		RemoteAddr:      remoteAddr,
		Status:          dbdata.UserAuthSuccess,
		DeviceType:      cr.DeviceId.Type(),
		PlatformVersion: cr.DeviceId.PlatformVersion,
		Os:              dbdata.UserActLogIns.OSIdx(cr.DeviceId.OS()),
	}
}

// 账号或 IP 被锁定时写审计日志并拒绝，返回是否已处理
func (s *LinkAuthService) checkLocked(w http.ResponseWriter, r *http.Request, cr *ClientRequest, ua *dbdata.UserActLog) bool {
	if s.locks.Check(cr.Auth.Username, r.RemoteAddr) {
		return false
	}
	ua.Status = dbdata.UserAuthFail
	ua.IsLockedFail = true
	if cr.Auth.Username == "" {
		if s.locks.InBlackList(r.RemoteAddr) {
			ua.Info = "IP在黑名单中，拒绝访问"
		} else {
			ua.Info = "IP已被锁定，请稍后重试"
		}
	} else {
		ua.Info = "账号已被锁定，请稍后重试"
	}
	dbdata.UserActLogIns.Add(*ua, cr.UserAgent)
	w.WriteHeader(http.StatusBadRequest)
	return true
}

// 拒绝原生客户端无法完成的认证方式：
// 手机端 SSO 组（扫码无法在内置浏览器完成）与 SMS 认证
func checkUnsupported(w http.ResponseWriter, r *http.Request, cr *ClientRequest, ua *dbdata.UserActLog) bool {
	// allow_mobile_sso 开启时放行移动端 SSO 组
	if cr.GroupSelect != "" && authsrv.GetSSOType(cr.GroupSelect) != "" && isMobileDevice(r) && !base.GetCfg().AllowMobileSSO {
		w.WriteHeader(http.StatusForbidden)
		return true
	}

	// SMS 认证仅支持 WebAuth/Portal，原生客户端不支持
	if cr.GroupSelect == "" {
		return false
	}
	groupData, err := dbdata.Groups.ByName(cr.GroupSelect)
	if err != nil {
		return false
	}
	if !dbdata.Groups.HasAuthType(groupData.AuthProfile, "sms") {
		return false
	}
	ua.Status = dbdata.UserAuthFail
	ua.Info = "原生客户端不支持 SMS 认证"
	dbdata.UserActLogIns.Add(*ua, cr.UserAgent)
	base.Warn("原生客户端不支持 SMS 认证，已拒绝: user=", cr.Auth.Username, "group=", cr.GroupSelect)
	w.WriteHeader(http.StatusForbidden)
	return true
}

// 处理客户端主动登出
func (s *LinkAuthService) logout(w http.ResponseWriter, cr *ClientRequest) {
	if cr.SessionToken != "" {
		s.sessions.DelSessByStoken(cr.SessionToken)
	}
	w.WriteHeader(http.StatusOK)
}

// 尝试从 cookie 中的会话恢复认证管道，返回是否已处理
// 会话无效时清除 cookie，让调用方继续走首次认证
func (s *LinkAuthService) resumeSavedAuth(w http.ResponseWriter, r *http.Request, cr *ClientRequest, sessionData *AuthSession) bool {
	sessionID, cerr := GetCookie(r, "auth-session-id")
	if cerr != nil || sessionID == "" {
		return false
	}
	sess, serr := s.authSessions.Get(sessionID)
	if serr != nil {
		DeleteCookie(w, "auth-session-id")
		return false
	}
	sess.Ctx.Conn.Password = cr.Auth.Password
	if sp := cr.Auth.SecondaryPassword; sp != "" {
		sess.Ctx.GetOTP().Code = sp
		sess.Ctx.GetRADIUS().ChallengeCode = sp
	}
	sess.Ctx.Conn.TLS = r.TLS
	sess.Ctx.Conn.RemoteAddr = r.RemoteAddr
	sess.UserActLog = sessionData.UserActLog
	s.resumeAuthSession(w, r, sess)
	return true
}

// 判断 samlSession 是否可被 SSO token 兑换：
// 仅 SSO 流程产生的会话（sso-done/sso-pending/webauth）或带 ForcePwd 标记的
// 强制改密 pipeline 会话允许；普通管道会话（OTP/RADIUS/SMS 等待）必须拒绝以防冒用
func isSSOTokenSessionAllowed(s *AuthSession) bool {
	switch s.Kind {
	case AuthKindSSODone, AuthKindSSOPending, AuthKindWebAuth:
		return true
	case AuthKindPipeline:
		// SSO 会话进入强制改密管道后被标记为 pipeline，原 SSO token 仍需可兑换续跑
		return s.ForcePwd
	default:
		return false
	}
}
