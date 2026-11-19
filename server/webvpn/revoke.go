package webvpn

import (
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 抬高吊销阈值使既有会话失效
type Revoker struct {
	mgr *WebVPNManager
}

// 吊销指定用户的全部 WebVPN 会话
func (r *Revoker) User(username string) {
	r.mgr.Session().RevokeUser(username)
}

// 批量吊销一批用户的 WebVPN 会话
func (r *Revoker) Users(usernames []string) {
	for _, u := range usernames {
		r.User(u)
	}
}

// 吊销指定用户组全部成员的 WebVPN 会话
func (r *Revoker) GroupMembers(groupNames []string) {
	if len(groupNames) == 0 {
		return
	}
	users, err := dbdata.Users.InGroups(groupNames)
	if err != nil {
		base.Error("WebVPN 查询组成员失败:", err)
		return
	}
	kicked := make(map[string]bool)
	for _, u := range users {
		if kicked[u.Username] {
			continue
		}
		r.User(u.Username)
		kicked[u.Username] = true
	}
}
