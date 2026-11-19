package handler

import (
	"sync"
	"time"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/sessdata"
	"github.com/wsczx/remlink/webvpn"
)

type PortalService struct {
	forgotLimiter *portalForgotLimiter
	resetTokens   *portalResetTokenStore
	sessions      *sessdata.SessionManager
	authSessions  *AuthSessionManager
	locks         *auth.LockManager
	webvpn        *webvpn.WebVPNManager
	sso           *SSOHandler
}

type portalForgotLimiter struct {
	mu   sync.Mutex
	next map[string]time.Time
}

type portalResetTokenStore struct {
	mu     sync.Mutex
	used   map[string]int64 // jti -> used_at unix
	inited bool
}

func NewPortalService(sessions *sessdata.SessionManager, authSessions *AuthSessionManager, locks *auth.LockManager, webvpnMgr *webvpn.WebVPNManager, sso *SSOHandler) *PortalService {
	return &PortalService{
		forgotLimiter: &portalForgotLimiter{next: make(map[string]time.Time)},
		resetTokens:   &portalResetTokenStore{used: make(map[string]int64)},
		sessions:      sessions,
		authSessions:  authSessions,
		locks:         locks,
		webvpn:        webvpnMgr,
		sso:           sso,
	}
}

func (p *PortalService) initResetTokens() {
	p.resetTokens.mu.Lock()
	if p.resetTokens.inited {
		p.resetTokens.mu.Unlock()
		return
	}
	p.resetTokens.inited = true
	p.resetTokens.mu.Unlock()

	data := dbdata.SettingPortalResetTokens{}
	if err := dbdata.Settings.Get(&data); err == nil && data.Tokens != nil {
		p.resetTokens.mu.Lock()
		now := time.Now().Unix()
		for jti, ts := range data.Tokens {
			if now-ts < 900 {
				p.resetTokens.used[jti] = ts
			}
		}
		p.resetTokens.mu.Unlock()
	}
}

func (p *PortalService) isTokenUsed(jti string) bool {
	p.initResetTokens()
	p.resetTokens.mu.Lock()
	defer p.resetTokens.mu.Unlock()
	_, ok := p.resetTokens.used[jti]
	return ok
}

// 返回 true 表示 token 已经被消费过
func (p *PortalService) checkAndMarkToken(jti string) bool {
	p.initResetTokens()
	p.resetTokens.mu.Lock()
	defer p.resetTokens.mu.Unlock()

	if _, ok := p.resetTokens.used[jti]; ok {
		return true
	}

	now := time.Now().Unix()
	p.resetTokens.used[jti] = now

	cutoff := now - 900
	clean := make(map[string]int64, len(p.resetTokens.used))
	for j, ts := range p.resetTokens.used {
		if ts >= cutoff {
			clean[j] = ts
		}
	}
	p.resetTokens.used = clean
	if err := dbdata.Settings.Save(&dbdata.SettingPortalResetTokens{Tokens: clean}); err != nil {
		base.Error("用户门户保存重置token状态失败:", err)
	}
	return false
}
