package webvpn

import (
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 封装整用户/批量踢出逻辑。通过抬高吊销阈值（O(1)）使此前签发的会话立即失效
type Revoker struct{}

func NewRevoker() *Revoker { return &Revoker{} }

// 吊销指定用户的全部 WebVPN 会话（整用户下线）
func (r *Revoker) RevokeUser(username string) {
	GetManager().Session().RevokeUser(username)
}

// 批量吊销一批用户的 WebVPN 会话（权限变更后让已签发会话立即失效）
func (r *Revoker) RevokeUsers(usernames []string) {
	for _, u := range usernames {
		r.RevokeUser(u)
	}
}

// 吊销指定用户组全部成员的 WebVPN 会话
func (r *Revoker) RevokeGroupMembers(groupNames []string) {
	if len(groupNames) == 0 {
		return
	}
	want := make(map[string]bool, len(groupNames))
	for _, g := range groupNames {
		want[g] = true
	}
	var users []dbdata.User
	for _, g := range groupNames {
		like := `%"` + dbdata.EscapeLike(g) + `"%`
		var part []dbdata.User
		if err := dbdata.FindWhere(&part, 0, 0, "groups LIKE ?", like); err != nil {
			base.Error("WebVPN 查询组成员失败:", err)
			continue
		}
		users = append(users, part...)
	}
	kicked := make(map[string]bool)
	for _, u := range users {
		for _, g := range u.Groups {
			if want[g] {
				if !kicked[u.Username] {
					r.RevokeUser(u.Username)
					kicked[u.Username] = true
				}
				break
			}
		}
	}
}

// 返回指定用户的吊销阈值（0 表示未吊销）
func (r *Revoker) BeforeOf(username string) int64 {
	return dbdata.WebVpnRevokeBeforeOf(username)
}
