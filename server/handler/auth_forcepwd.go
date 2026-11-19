// 强制改密共享逻辑：WebAuth 与门户共用核心操作（校验策略→哈希→写库→重载）
// 各端自行处理会话前置校验与续跑（两端续跑 API 与签发响应不同）
// 原生管道的强制改密走标准 login 写库流程，不在此范围
package handler

import (
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/jwt"
	"github.com/wsczx/remlink/pkg/utils"
)

// 校验密码策略、哈希并写库，吊销该用户全部 JWT，返回吊销的会话数
// 普通改密与重置密码共用；强制改密在调用后额外重载用户信息以续跑管道
func setPassword(username, newPassword string) (int, error) {
	if err := utils.CheckPasswordPolicy(newPassword); err != nil {
		return 0, err
	}
	hashed, err := utils.PasswordHash(newPassword)
	if err != nil {
		return 0, err
	}
	if err := dbdata.Users.UpdatePassword(username, hashed); err != nil {
		return 0, err
	}
	return jwt.RevokeSubjectJwt(username), nil
}

// 执行强制改密核心并续跑管道：写库后重载用户使 ForcePwd=false
func RunForcePasswordChange(ctx *auth.Context, username, newPassword string) error {
	n, err := setPassword(username, newPassword)
	if err != nil {
		return err
	}
	if n > 0 {
		base.Info("强制改密，已吊销", n, "个有效会话:", username)
	}
	authsrv.ReloadUserInfo(ctx)
	return nil
}
