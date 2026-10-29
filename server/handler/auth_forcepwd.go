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

// 执行强制改密核心操作；成功后将 ctx 内用户信息以库为准重载
// 使续跑管道读到 ForcePwd=false
func RunForcePwdChange(ctx *auth.Context, username, newPassword string) error {
	if err := utils.CheckPasswordPolicy(newPassword); err != nil {
		return err
	}
	hashed, err := utils.PasswordHash(newPassword)
	if err != nil {
		return err
	}
	// 按用户名直接更新，避免先查全量用户仅取 Id 的重复查库
	if _, err := dbdata.GetXdb().Where("username = ?", username).Cols("pin_code", "change_pwd").
		Update(&dbdata.User{PinCode: hashed, ForcePwd: false}); err != nil {
		return err
	}
	// 强制改密意味着原凭据已泄露：吊销全部旧 JWT（续跑签发的新令牌不受影响）
	if n := jwt.RevokeSubjectJwt(username); n > 0 {
		base.Info("强制改密，已吊销", n, "个有效会话:", username)
	}
	// 重载使用户信息读到 ForcePwd=false，续跑管道直接通过该步
	authsrv.ReloadUserInfo(ctx)
	return nil
}
