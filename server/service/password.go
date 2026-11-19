package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/jwt"
	"github.com/wsczx/remlink/pkg/utils"
)

const (
	ResetTokenPurpose = "portal_reset_password"
	// 密码重置令牌有效期（秒）
	ResetTokenTTL = 900
)

var (
	ErrResetTokenExpired = errors.New("重置链接已过期，请重新申请")
	ErrResetTokenInvalid = errors.New("无效的重置链接")
	ErrUserUnavailable   = errors.New("用户不存在或已停用")
	ErrExternalUser      = errors.New("外部认证用户请到对应身份源修改密码")
	ErrOldPassword       = errors.New("旧密码错误")
)

// 密码重置令牌解析后的业务声明
type ResetClaims struct {
	JTI    string
	UserID int
}

// 签发带 jti 的密码重置令牌；一次性由调用方标记保证
func IssueResetToken(user *dbdata.User) (string, error) {
	jti := fmt.Sprintf("%d_%d", user.Id, time.Now().UnixNano())
	return jwt.SetJwtData(map[string]any{
		"purpose": ResetTokenPurpose,
		"user_id": user.Id,
		"jti":     jti,
	}, time.Now().Unix()+ResetTokenTTL)
}

// 解析并校验密码重置令牌的用途与声明
// 令牌本身无效/过期返回 ErrResetTokenExpired；用途不符或缺 jti 返回 ErrResetTokenInvalid
func ParseResetClaims(token string) (ResetClaims, error) {
	data, err := jwt.GetJwtData(token)
	if err != nil {
		return ResetClaims{}, ErrResetTokenExpired
	}
	if purpose, _ := data["purpose"].(string); purpose != ResetTokenPurpose {
		return ResetClaims{}, ErrResetTokenInvalid
	}
	jti, _ := data["jti"].(string)
	if jti == "" {
		return ResetClaims{}, ErrResetTokenInvalid
	}
	userID, ok := data["user_id"].(float64)
	if !ok || userID < 1 {
		return ResetClaims{}, ErrResetTokenInvalid
	}
	return ResetClaims{JTI: jti, UserID: int(userID)}, nil
}

// 载入可自助改密的本地用户（要求启用且非外部认证）
func LocalPasswordUser(userID int) (*dbdata.User, error) {
	user := &dbdata.User{}
	if err := dbdata.Users.Get(userID, user); err != nil || user.Status != 1 {
		return nil, ErrUserUnavailable
	}
	if user.Type != "" && user.Type != "local" {
		return nil, ErrExternalUser
	}
	return user, nil
}

// 写入已哈希的口令并吊销该用户全部门户 JWT，返回吊销数
// 拆出哈希步骤是为了让调用方能在写库前插入一次性令牌标记
func CommitPassword(username, hashed string) (int, error) {
	if err := dbdata.Users.UpdatePassword(username, hashed); err != nil {
		return 0, err
	}
	return actions.RevokeJwt(username), nil
}

// 校验本地口令
func VerifyLocalPassword(user *dbdata.User, password string) error {
	if password == "" || !utils.VerifyPassword(password, user.PinCode) {
		return fmt.Errorf("密码错误")
	}
	return nil
}

// 哈希新口令并提交，返回吊销数
func applyNewPassword(username, newPwd string) (int, error) {
	hashed, err := utils.PasswordHash(newPwd)
	if err != nil {
		return 0, err
	}
	return CommitPassword(username, hashed)
}

// 校验旧口令，写入新口令并吊销门户会话，返回吊销数
// 新口令的复杂度校验由调用方在解析请求时完成（需向用户回显具体策略要求）
func ChangeLocalPassword(user *dbdata.User, oldPwd, newPwd string) (int, error) {
	if oldPwd == "" || !utils.VerifyPassword(oldPwd, user.PinCode) {
		return 0, ErrOldPassword
	}
	return applyNewPassword(user.Username, newPwd)
}
