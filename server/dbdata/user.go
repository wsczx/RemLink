package dbdata

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xlzd/gotp"
)

// 用户与设备映射的存储入口
type UserStore struct{ *table[User] }

// 用户与设备映射的存储入口实例，由 SetDB 注入持久化实现
var Users = &UserStore{}

func (u *User) IsExpired() bool {
	return utils.IsExpired(u.LimitTime)
}

func sameGroups(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]struct{}, len(a))
	for _, g := range a {
		m[g] = struct{}{}
	}
	for _, g := range b {
		if _, ok := m[g]; !ok {
			return false
		}
	}
	return true
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

func (st *UserStore) SetUser(v *User) error {
	var err error
	if v.Username == "" || len(v.Groups) == 0 {
		return errors.New("用户名或组错误")
	}

	planPass := v.PinCode

	if planPass == "" {
		planPass = utils.RandomRunes(8)
	}
	v.PinCode = planPass

	if v.Id == 0 && v.OtpSecret == "" {
		v.OtpSecret = gotp.RandomSecret(32)
	}

	ng := []string{}
	groups := Groups.GetGroupNames()
	for _, g := range v.Groups {
		if slices.Contains(groups, g) {
			ng = append(ng, g)
		}
	}
	if len(ng) == 0 {
		return errors.New("用户名或组错误")
	}
	v.Groups = ng

	if v.PolicyId > 0 {
		var policy Policy
		if err := GetDB().Get("Id", v.PolicyId, &policy); err != nil {
			return errors.New("引用的策略不存在")
		}
		if policy.Status != 1 {
			return errors.New("引用的策略已停用，请选择启用的策略")
		}
	}

	// 检查用户名是否重复
	var exist User
	err = GetDB().Get("Username", v.Username, &exist)
	if err == nil {
		if v.Id == 0 || exist.Id != v.Id {
			return errors.New("用户名已存在")
		}
	}

	var old User
	isUpdate := v.Id > 0 && GetDB().Get("Id", v.Id, &old) == nil
	pwdChanged := !isUpdate || v.PinCode != old.PinCode
	groupsChanged := isUpdate && !sameGroups(v.Groups, old.Groups)

	if isUpdate {
		v.TrafficUsed = old.TrafficUsed
		v.TrafficResetAt = old.TrafficResetAt
		v.CreatedAt = old.CreatedAt
		if v.OtpSecret == "" {
			v.OtpSecret = old.OtpSecret
		}
	}

	v.UpdatedAt = time.Now()
	if v.Id > 0 {
		err = GetDB().Save(v)
	} else {
		err = GetDB().Insert(v)
	}
	if err == nil {

		if pwdChanged || v.Status != 1 || groupsChanged {
			if revokeErr := WebVpn.RevokeUser(v.Username); revokeErr != nil {
				base.Error("用户更新成功但 WebVPN 会话吊销持久化失败:", v.Username, revokeErr)
			}
		}
	}

	return err
}

// 从所有用户的组列表中移除已删除的组
func (st *UserStore) RemoveGroupFromUsers(groupName string) error {
	var users []User
	if err := GetDB().Find(&users, 0, 0); err != nil {
		return err
	}
	for i := range users {
		if !slices.Contains(users[i].Groups, groupName) {
			continue
		}
		if idx := slices.Index(users[i].Groups, groupName); idx >= 0 {
			users[i].Groups = slices.Delete(users[i].Groups, idx, idx+1)
		}
		if err := GetDB().Save(&users[i]); err != nil {
			return err
		}
	}
	return nil
}

// 返回所有"Groups 字段包含 groups 中任一组名"的用户
func (st *UserStore) InGroups(groups []string) ([]User, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	want := make(map[string]bool, len(groups))
	for _, g := range groups {
		want[g] = true
	}
	var allUsers []User
	if err := GetDB().Find(&allUsers, 0, 0); err != nil {
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

// 用户维度聚合统计
type UserStats struct {
	Total    int
	Local    int
	External int
	Active   int
	Disable  int
}

// 在不分页、不全量加载对象的前提下，对当前筛选条件做聚合统计
// 用 SQL 的 COUNT + CASE WHEN 在数据库侧完成统计，避免把全表用户载入内存
// 外部用户 = 所有非本地用户类型（ldap/radius/wxwork/dingtalk/feishu/external 均算外部）
func (st *UserStore) StatsWhere(where string, args ...any) (UserStats, error) {
	var s UserStats
	t := QuoteIdent("type")
	u := QuoteIdent("user")
	sql := fmt.Sprintf(`SELECT
		COUNT(*) AS total,
		SUM(CASE WHEN %s = 'local' THEN 1 ELSE 0 END) AS local,
		SUM(CASE WHEN %s = 'ldap' THEN 1 ELSE 0 END) AS ldap,
		SUM(CASE WHEN %s != 'local' THEN 1 ELSE 0 END) AS external,
		SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END) AS active,
		SUM(CASE WHEN status != 1 THEN 1 ELSE 0 END) AS disable
		FROM %s`, t, t, t, u)
	if where != "" {
		sql += " WHERE " + where
	}
	if _, err := GetDB().Engine().SQL(sql, args...).Get(&s); err != nil {
		return s, err
	}
	return s, nil
}

// 返回引用了指定策略的用户数量
func (st *UserStore) CountByPolicy(policyId int) int {
	return GetDB().CountWhere(&User{}, "policy_id=?", policyId)
}

// 按 ID 加载用户
func (st *UserStore) Get(id int, dest *User) error {
	return GetDB().Get("Id", id, dest)
}

// 按 ID 加载用户，返回指针
func (st *UserStore) GetByID(id int) (*User, error) {
	var u User
	if err := st.Get(id, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// 按用户名加载用户
func (st *UserStore) GetByUsername(name string, dest *User) error {
	return GetDB().Get("Username", name, dest)
}

// 按手机号加载用户
func (st *UserStore) GetByPhone(phone string, dest *User) error {
	return GetDB().Get("Phone", phone, dest)
}

// 返回全量用户列表
func (st *UserStore) FindAll(dest *[]User) error {
	return GetDB().Find(dest, 0, 0)
}

// 返回用户总行数
func (st *UserStore) Count() int {
	return GetDB().Count(&User{})
}

// 批量把用户改挂到指定策略
func (st *UserStore) SetPolicy(ids []int, policyID int) (int, error) {
	count := 0
	for _, id := range ids {
		u, err := st.GetByID(id)
		if err != nil {
			continue
		}
		u.PolicyId = policyID
		if err := st.Save(u); err != nil {
			return count, fmt.Errorf("更新用户 %s 失败: %w", u.Username, err)
		}
		count++
	}
	return count, nil
}

// 批量把当前引用指定策略的用户移出该策略
func (st *UserStore) ClearPolicy(ids []int, policyID int) (int, error) {
	count := 0
	for _, id := range ids {
		u, err := st.GetByID(id)
		if err != nil || u.PolicyId != policyID {
			continue
		}
		u.PolicyId = 0
		if err := st.Save(u); err != nil {
			return count, fmt.Errorf("移出用户 %s 失败: %w", u.Username, err)
		}
		count++
	}
	return count, nil
}

func (st *UserStore) ResetUserTraffic(username string) error {
	_, err := st.engine().Where("username=?", username).
		Cols("traffic_used", "traffic_reset_at").
		Update(&User{})
	return err
}
func (st *UserStore) ByIDs(ids []int) ([]User, error) {
	var users []User
	if err := st.engine().In("id", ids).Find(&users); err != nil {
		return nil, err
	}
	return users, nil
}
func (st *UserStore) DeleteUsersByIDs(ids []int) (int64, error) {
	return st.engine().In("id", ids).Delete(&User{})
}

// 更新密码并清除强制改密标记
func (st *UserStore) UpdatePassword(username, hashed string) error {
	_, err := st.engine().Where("username = ?", username).
		Cols("pin_code", "change_pwd").
		Update(&User{PinCode: hashed, ForcePwd: false})
	return err
}
func (st *UserStore) SetForcePwd(username string, force bool) error {
	_, err := st.engine().Where("username = ?", username).
		Cols("change_pwd").
		Update(&User{ForcePwd: force})
	return err
}
