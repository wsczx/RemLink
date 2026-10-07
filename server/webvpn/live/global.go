package live

import (
	"sync"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

var global = NewRegistry()

// 全局注册表
func Default() *Registry { return global }

func init() {
	// 用户会话被吊销（踢下线、权限收窄、删除/禁用用户、改组）→ 立即断开该用户全部长连接
	dbdata.OnWebVpnUserRevoked(func(username string) {
		if n := global.KickUser(username, ReasonRevoked); n > 0 {
			base.Info("WebVPN 吊销用户，断开在线长连接:", username, n)
		}
	})
	// 应用变更/删除 → 复核该应用的长连接
	dbdata.OnWebVpnAppChanged(func(appName string) {
		if n := global.Revalidate(func(s *Session) bool { return s.AppName == appName }, ValidateWithDB); n > 0 {
			base.Info("WebVPN 应用变更，断开在线长连接:", appName, n)
		}
	})
}

// 以数据库当前状态复核会话：应用存在且启用、协议与目标未变、用户仍被授权、来源 IP 仍在白名单、未被整用户吊销
func ValidateWithDB(s *Session) (bool, string) {
	app, err := dbdata.GetWebVpnAppByName(s.AppName)
	if err != nil || app == nil || app.Status != 1 || app.Id != s.AppId {
		return false, ReasonAppChanged
	}
	if app.ProtocolOf() != s.Protocol || app.TargetAddr() != s.Target {
		return false, ReasonAppChanged
	}
	if !dbdata.WebVpnUserAllowed(app, &dbdata.User{Username: s.Username, Groups: s.Groups}) {
		return false, ReasonUnauthorized
	}
	if !IPAllowed(s.ClientIP, app.IpAllowList) {
		return false, ReasonUnauthorized
	}
	// 吊销阈值晚于会话建立 → 已被整用户踢出（钩子未覆盖到的兜底，如多实例写库）
	if before := dbdata.WebVpnRevokeBeforeOf(s.Username); before > 0 && before > s.StartedAt.Unix() {
		return false, ReasonRevoked
	}
	return true, ""
}

var sweepOnce sync.Once

// 启动周期复核（兜底），interval<=0 取 30s
func StartSweeper(interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	sweepOnce.Do(func() {
		go func() {
			t := time.NewTicker(interval)
			defer t.Stop()
			for range t.C {
				global.Revalidate(nil, ValidateWithDB)
			}
		}()
	})
}
