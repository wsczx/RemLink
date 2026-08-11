// 强制改密共享逻辑
//
// WebAuth 与门户各自有独立的强制改密 handler（读请求体、校验会话/token 的方式不同），
// 但「校验密码策略 → 哈希 → 写库（pin_code + 清除 change_pwd）→ 以库为准重载用户信息」
// 改密码规则只维护一处
//
// 设计边界：
//   - 不负责「续跑管道 / 签发令牌」——各端续跑 API 不同（webAuthRunOrResume / authsrv.Resume），
//     且门户改密后还要签发门户登录响应，由调用方处理。
//   - 不校验用户状态 / 外部用户 / 是否已改过——这些是各端会话前置校验，调用方负责。
//   - 原生管道的强制改密走标准 login 写库流程（forcepwd 步骤仅判定是否需改），不在此函数范围。
package handler

import (
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

// 执行强制改密的核心原子操作：校验密码策略、哈希、写库、重载
// 返回 error 表示改密失败（策略不合规 / 哈希失败 / 数据库写入失败），调用方据此返回错误响应
// 成功后将 ctx 内的用户信息以数据库为准重载，供后续续跑管道读取到最新的 ForcePwd=false
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
	// 以数据库为准重载用户信息，使后续续跑管道读到 ForcePwd=false（直接通过 forcepwd 步）
	authsrv.ReloadUserInfo(ctx)
	return nil
}
