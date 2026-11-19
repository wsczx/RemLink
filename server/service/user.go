package service

import (
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

// 补全默认口令并保存，返回明文口令与口令是否变更
// 新建未填口令时随机生成；修改未填时沿用原口令；显式填写视为变更
func SaveUser(data *dbdata.User) (plainPwd string, pwdChanged bool, err error) {
	if data.PinCode == "" {
		var exist dbdata.User
		if data.Id > 0 && dbdata.Users.Get(data.Id, &exist) == nil {
			data.PinCode = exist.PinCode
		} else {
			data.PinCode = utils.RandomRunes(8)
			pwdChanged = true
		}
	} else {
		pwdChanged = true
	}
	plainPwd = data.PinCode

	if err := dbdata.Users.SetUser(data); err != nil {
		return "", false, err
	}
	return plainPwd, pwdChanged, nil
}

// 应用用户变更：刷新在线缓存、停用断线、必要时吊销门户 JWT
func ApplyUserChange(data *dbdata.User, pwdChanged bool) {
	actions.UpdateUserLimitTime(data.Username, data.LimitTime)
	if data.Status != 1 {
		actions.CloseUserSessions(data.Username, dbdata.UserLogoutAdmin)
	}
	if pwdChanged || data.Status != 1 {
		if n := actions.RevokeJwt(data.Username); n > 0 {
			base.Info("用户口令/状态变更，已吊销", n, "个门户会话:", data.Username)
		}
	}
}

// 单个删除：WebVPN 吊销同步落库（失败记录不阻断），JWT 与在线会话同步联动
func DeleteUser(user *dbdata.User) {
	if err := actions.RevokeUserWebVpn(user.Username); err != nil {
		base.Error("用户删除成功但 WebVPN 会话吊销持久化失败:", user.Username, err)
	}
	if n := actions.RevokeJwt(user.Username); n > 0 {
		base.Info("用户已删除，已吊销", n, "个门户会话:", user.Username)
	}
	actions.CloseUserSessions(user.Username, dbdata.UserLogoutAdmin)
}

// 批量删除后的统一联动：WebVPN 吊销异步执行避免阻塞响应，JWT 与在线会话同步联动
func RevokeDeletedUsers(usernames []string) {
	if len(usernames) == 0 {
		return
	}
	actions.RevokeUsersWebVpn(usernames)
	for _, username := range usernames {
		if n := actions.RevokeJwt(username); n > 0 {
			base.Info("用户已删除，已吊销", n, "个门户会话:", username)
		}
		actions.CloseUserSessions(username, dbdata.UserLogoutAdmin)
	}
}
