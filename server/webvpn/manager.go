package webvpn

import (
	"sync"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// WebVPN 子系统中心对象，聚合所有子组件。进程启动 Start、退出 Stop。
type manager struct {
	apps    *AppStore
	session *AuthSessionManager
	audit   *AuditBatcher
	revoker *Revoker
}

var (
	once sync.Once
	mgr  *manager
)

// 返回 WebVPN 子系统单例（懒初始化）
func GetManager() *manager {
	once.Do(func() {
		mgr = &manager{
			apps:    NewAppStore(),
			session: NewSessionManager(),
			audit:   NewAuditBatcher(),
			revoker: NewRevoker(),
		}
	})
	return mgr
}

// 启动子系统：加载整用户踢出阈值并启动审计批处理。
func (m *manager) Start() {
	dbdata.LoadWebVpnRevoke()
	m.audit.Start()
	base.Debug("WebVPN 子系统已启动")
}

// 停止子系统后台任务，等待在途审计落库。
func (m *manager) Stop() {
	m.audit.Stop()
	base.Debug("WebVPN 子系统已停止")
}

func (m *manager) Apps() *AppStore { return m.apps }

func (m *manager) Session() *AuthSessionManager { return m.session }

func (m *manager) Audit() *AuditBatcher { return m.audit }

func (m *manager) Revoker() *Revoker { return m.revoker }
