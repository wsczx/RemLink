package webvpn

import "github.com/wsczx/remlink/base"

// 聚合 WebVPN 子组件
type WebVPNManager struct {
	apps    *AppStore
	session *AuthSessions
	audit   *AuditBatcher
	revoker *Revoker
}

func NewManager() *WebVPNManager {
	m := &WebVPNManager{
		apps:    NewAppStore(),
		session: NewAuthSessions(),
		audit:   NewAuditBatcher(),
	}
	m.revoker = &Revoker{mgr: m}
	return m
}

// 启动审计批处理
func (m *WebVPNManager) Start() {
	m.audit.Start()
	base.Debug("WebVPN 子系统已启动")
}

// 等待在途审计落库
func (m *WebVPNManager) Stop() {
	m.audit.Stop()
	base.Debug("WebVPN 子系统已停止")
}

func (m *WebVPNManager) Apps() *AppStore { return m.apps }

func (m *WebVPNManager) Session() *AuthSessions { return m.session }

func (m *WebVPNManager) Audit() *AuditBatcher { return m.audit }

func (m *WebVPNManager) Revoker() *Revoker { return m.revoker }
