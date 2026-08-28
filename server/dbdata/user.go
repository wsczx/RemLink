package dbdata

import (
	"crypto/subtle"
	"errors"
	"time"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xlzd/gotp"
)

func IsUserExpired(user *User) bool {
	return user != nil && user.LimitTime != nil && !time.Now().Before(*user.LimitTime)
}

func SetUser(v *User) error {
	var err error
	if v.Username == "" || len(v.Groups) == 0 {
		return errors.New("用户名或组错误")
	}

	planPass := v.PinCode
	// 密码为空时由系统随机生成
	if planPass == "" {
		planPass = utils.RandomRunes(8)
	}
	v.PinCode = planPass

	if v.OtpSecret == "" {
		v.OtpSecret = gotp.RandomSecret(32)
	}

	// 判断组是否有效
	ng := []string{}
	groups := GetGroupNames()
	for _, g := range v.Groups {
		if utils.InArrStr(groups, g) {
			ng = append(ng, g)
		}
	}
	if len(ng) == 0 {
		return errors.New("用户名或组错误")
	}
	v.Groups = ng

	// 校验策略引用（PolicyId=0 表示跟随组策略）
	if v.PolicyId > 0 {
		var policy Policy
		if err := One("Id", v.PolicyId, &policy); err != nil {
			return errors.New("引用的策略不存在")
		}
		if policy.Status != 1 {
			return errors.New("引用的策略已停用，请选择启用的策略")
		}
	}

	// 检查用户名是否重复
	var exist User
	err = One("Username", v.Username, &exist)
	if err == nil {
		if v.Id == 0 || exist.Id != v.Id {
			return errors.New("用户名已存在")
		}
	}

	v.UpdatedAt = time.Now()
	if v.Id > 0 {
		err = Set(v)
	} else {
		err = Add(v)
	}
	if err == nil {
		// 用户组或状态变更后，使已签发的 WebVPN 会话失效
		if revokeErr := WebVpnRevokeUser(v.Username); revokeErr != nil {
			base.Error("用户更新成功但 WebVPN 会话吊销持久化失败:", v.Username, revokeErr)
		}
	}

	return err
}

// 插入数据库前加密密码
func (u *User) BeforeInsert() {
	if base.GetCfg().EncryptionPassword && !utils.IsBcryptHash(u.PinCode) {
		hashedPassword, err := utils.PasswordHash(u.PinCode)
		if err != nil {
			base.Error(err)
		}
		u.PinCode = hashedPassword
	}
}

// 更新数据库前加密密码
func (u *User) BeforeUpdate() {
	if !utils.IsBcryptHash(u.PinCode) && base.GetCfg().EncryptionPassword {
		hashedPassword, err := utils.PasswordHash(u.PinCode)
		if err != nil {
			base.Error(err)
		}
		u.PinCode = hashedPassword
	}
}

// 从所有用户的组列表中移除已删除的组
func RemoveGroupFromUsers(groupName string) error {
	var users []User
	if err := Find(&users, 0, 0); err != nil {
		return err
	}
	for i := range users {
		if !utils.InArrStr(users[i].Groups, groupName) {
			continue
		}
		users[i].Groups = utils.RemoveStrFromArr(users[i].Groups, groupName)
		if err := Set(&users[i]); err != nil {
			return err
		}
	}
	return nil
}

// 返回所有"Groups 字段包含 groups 中任一组名"的用户
func UsersInGroups(groups []string) ([]User, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	want := make(map[string]bool, len(groups))
	for _, g := range groups {
		want[g] = true
	}
	var allUsers []User
	if err := Find(&allUsers, 0, 0); err != nil {
		return nil, err
	}
	var matched []User
	for _, u := range allUsers {
		for _, g := range u.Groups {
			if want[g] {
				matched = append(matched, u)
				break
			}
		}
	}
	return matched, nil
}

// 校验密码
func VerifyPassword(password, pinCode string) bool {
	if utils.IsBcryptHash(pinCode) {
		return utils.PasswordVerify(password, pinCode)
	}
	return subtle.ConstantTimeCompare([]byte(pinCode), []byte(password)) == 1
}

// 认证模块使用的用户信息
func (u *User) ToAuthInfo() *auth.UserInfo {
	return &auth.UserInfo{
		Username:   u.Username,
		Nickname:   u.Nickname,
		Type:       u.Type,
		Groups:     u.Groups,
		OtpSecret:  u.OtpSecret,
		DisableOtp: u.DisableOtp,
		LimitTime:  u.LimitTime,
		Phone:      u.Phone,
		Email:      u.Email,
		ForcePwd:   u.ForcePwd,
	}
}
