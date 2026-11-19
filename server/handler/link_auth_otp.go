// 验证入口，通过 Cookie 恢复认证管道并执行 OTP 挑战

package handler

import (
	"encoding/xml"
	"io"
	"net/http"

	"github.com/wsczx/remlink/base"
)

// OTP 验证入口（独立窗口模式），通过 Cookie 恢复 Pipeline
func (s *LinkAuthService) LinkAuthOTP(w http.ResponseWriter, r *http.Request) {
	sessionID, err := GetCookie(r, "auth-session-id")
	if err != nil {
		http.Error(w, "会话已过期，请重新登录", http.StatusUnauthorized)
		return
	}

	sessionData, err := s.authSessions.Get(sessionID)
	if err != nil {
		http.Error(w, "会话已过期，请重新登录", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		base.Error(err)
		s.authSessions.Delete(sessionID)
		DeleteCookie(w, "auth-session-id")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	cr := &ClientRequest{}
	err = xml.Unmarshal(body, cr)
	if err != nil {
		base.Error(err)
		s.authSessions.Delete(sessionID)
		DeleteCookie(w, "auth-session-id")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	username := sessionData.Ctx.Conn.Username

	// 锁定状态判断
	if !s.locks.Check(username, r.RemoteAddr) {
		w.WriteHeader(http.StatusTooManyRequests)
		s.authSessions.Delete(sessionID)
		DeleteCookie(w, "auth-session-id")
		return
	}

	// 将 OTP 码注入 Context（挑战响应）
	if sp := cr.Auth.SecondaryPassword; sp != "" {
		sessionData.Ctx.GetOTP().Code = sp
		sessionData.Ctx.GetRADIUS().ChallengeCode = sp
	}
	sessionData.Ctx.Conn.TLS = r.TLS
	sessionData.Ctx.Conn.RemoteAddr = r.RemoteAddr

	s.resumeAuthSession(w, r, sessionData)
}
