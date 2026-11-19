// Package service 承载业务编排：跨聚合的流程、配置变更与会话/缓存联动
package service

import (
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/jwt"
	"github.com/wsczx/remlink/sessdata"
)

// 保存操作触发的外部联动
type Actions struct {
	RemoveGroupNAT      func(v4, v6, dev string)            // 删除组 NAT 规则
	InvalidateCertCache func()                              // 失效组证书缓存
	SyncExternalUsers   func(g *dbdata.Group)               // 同步组 OTP 外部用户
	RevokeGroupWebVpn   func(names []string)                // 撤回组 WebVPN 授权
	RevokeUserWebVpn    func(username string) error         // 撤回用户 WebVPN 授权
	RevokeUsersWebVpn   func(usernames []string)            // 批量撤回用户 WebVPN 授权
	RevokeJwt           func(username string) int           // 撤回用户 JWT 令牌
	UpdateUserLimitTime func(username string, t *time.Time) // 更新用户限制时间
	CloseUserSessions   func(username string, code uint8)   // 关闭用户会话
}

var actions Actions

func SetActions(a Actions) { actions = a }

func ProductionActions(sessions *sessdata.SessionManager) Actions {
	return Actions{
		RemoveGroupNAT: func(v4, v6, dev string) { sessions.RemoveGroupNAT(v4, v6, dev) },

		InvalidateCertCache: func() { dbdata.Groups.InvalidateCertAuthCache() },

		SyncExternalUsers: func(g *dbdata.Group) { dbdata.Groups.SyncExternalUsersForOTP(g) },

		RevokeGroupWebVpn: func(names []string) {
			base.SafeGo(func() { dbdata.WebVpn.RevokeGroupMembers(names) })
		},

		RevokeUserWebVpn: func(username string) error { return dbdata.WebVpn.RevokeUser(username) },

		RevokeUsersWebVpn: func(usernames []string) {
			base.SafeGo(func() { dbdata.WebVpn.RevokeUsers(usernames) })
		},

		RevokeJwt: func(username string) int { return jwt.RevokeSubjectJwt(username) },

		UpdateUserLimitTime: func(username string, t *time.Time) { sessions.UpdateUserLimitTime(username, t) },

		CloseUserSessions: func(username string, code uint8) {
			sessions.CloseUserSessions(username, code)
		},
	}
}
