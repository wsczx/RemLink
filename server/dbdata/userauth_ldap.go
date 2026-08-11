package dbdata

import (
	"fmt"

	"github.com/go-ldap/ldap"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xlzd/gotp"
)

// AuthLdap LDAP 认证配置（嵌入共享 LDAPConfig，Connect/SearchFilter/SearchUsers 等方法由嵌入体提供）
type AuthLdap struct {
	auth.LDAPConfig
}

// 从 Group 的 AuthProfile 中通过 Provider 名称解析 LDAP 配置。
func ResolveLdapConfig(g *Group) (*AuthLdap, error) {
	profile, err := auth.ParseAuthProfile(g.AuthProfile)
	if err != nil {
		return nil, err
	}

	var cfg map[string]any
	for _, step := range profile.Step {
		if step.Type == "ldap" {
			if step.Provider == "" {
				return nil, fmt.Errorf("LDAP 步骤未设置 Provider")
			}
			resolved, err := ResolveProviderConfig(step.Provider, "ldap")
			if err != nil {
				return nil, fmt.Errorf("解析 LDAP Provider %q: %w", step.Provider, err)
			}
			cfg = resolved
			break
		}
	}
	if cfg == nil {
		return nil, fmt.Errorf("认证配置中未找到 LDAP 步骤")
	}

	result := &AuthLdap{}
	if err := auth.GetProviderConfigFromMap(cfg, &result.LDAPConfig); err != nil {
		return nil, err
	}
	result.Defaults()
	return result, nil
}

// 从 LDAP 同步用户到本地数据库
func (a *AuthLdap) SaveUsers(g *Group) error {
	// 建立LDAP连接
	l, err := a.Connect()
	if err != nil {
		return err
	}
	defer l.Close()

	// 搜索所有用户
	sr, err := a.SearchUsers(l, "", []string{
		"displayName",
		"mail",
		"telephoneNumber",    // AD/OpenLDAP 手机号（标准 AD 用此属性）
		"mobile",             // 兼容装了 Exchange / 加了 schema 扩展的 AD、以及 OpenLDAP inetOrgPerson
		"userAccountControl", // AD用户状态
		"accountExpires",     // AD账号过期时间
		"shadowExpire",       // Linux LDAP用户状态
		a.SearchAttr,
	})
	if err != nil {
		return err
	}
	// 兼容不同目录 schema：优先 telephoneNumber，空则退 mobile
	ldapPhone := func(entry *ldap.Entry) string {
		if v := entry.GetAttributeValue("telephoneNumber"); v != "" {
			return v
		}
		return entry.GetAttributeValue("mobile")
	}
	// 创建LDAP用户映射
	ldapUserMap := make(map[string]bool)
	// 处理搜索结果
	for _, entry := range sr.Entries {
		if a.SyncUserStatus {
			// 检查用户状态，只同步正常用户
			if err := auth.CheckAccountStatus(&ldap.SearchResult{Entries: []*ldap.Entry{entry}}); err != nil {
				continue
			}
		}
		var groups []string
		needOTP := a.EnableOtp || HasAuthType(g.AuthProfile, "otp")
		ldapuser := &User{
			Type:       "ldap",
			Username:   entry.GetAttributeValue(a.SearchAttr),
			Nickname:   entry.GetAttributeValue("displayName"),
			Email:      entry.GetAttributeValue("mail"),
			Phone:      ldapPhone(entry),
			Groups:     append(groups, g.Name),
			DisableOtp: !needOTP,
			OtpSecret:  gotp.RandomSecret(32),
			SendEmail:  false,
			Status:     1,
		}
		ldapUserMap[ldapuser.Username] = true
		// 新增或更新ldap用户
		u := &User{}
		if err := One("username", ldapuser.Username, u); err != nil {
			if CheckErrNotFound(err) {
				if err := Add(ldapuser); err != nil {
					base.Error("新增ldap用户失败", ldapuser.Username, err)
					continue
				}
				continue
			}
			base.Error("查询用户失败", ldapuser.Username, err)
			continue
		}
		if u.Type != "ldap" {
			base.Warn("已存在本地同名用户:", ldapuser.Username)
			continue
		}
		// 现有LDAP用户，更新字段
		u.Nickname = entry.GetAttributeValue("displayName")
		u.DisableOtp = !needOTP
		if u.OtpSecret == "" {
			u.OtpSecret = gotp.RandomSecret(32)
		}
		if u.Email == "" {
			u.Email = entry.GetAttributeValue("mail")
		}
		if u.Phone == "" {
			u.Phone = ldapPhone(entry)
		}
		if !utils.InArrStr(u.Groups, g.Name) {
			u.Groups = append(u.Groups, g.Name)
		}

		if err := Set(u); err != nil {
			return fmt.Errorf("更新ldap用户%s失败:%v", u.Username, err.Error())
		}
	}
	if len(sr.Entries) == 0 {
		return fmt.Errorf("LDAP 搜索到的用户列表为空（可能连接失败、base DN 配置错误或搜索权限不足），组: %s", g.Name)
	}

	// 查询本地LDAP用户
	var localLdapUsers []User
	if err := FindWhere(&localLdapUsers, 0, 0, "type = 'ldap'"); err != nil {
		base.Error("查询本地LDAP用户失败:", err)
		return fmt.Errorf("查询本地LDAP用户失败: %w", err)
	}

	// 删除LDAP中不存在的本地用户
	for _, localUser := range localLdapUsers {
		if !utils.InArrStr(localUser.Groups, g.Name) {
			continue
		}
		if !ldapUserMap[localUser.Username] {
			localUser.Groups = utils.RemoveStrFromArr(localUser.Groups, g.Name)
			if len(localUser.Groups) == 0 {
				if err := Del(&localUser); err != nil {
					base.Error("删除本地LDAP用户失败:", localUser.Username, err)
				} else {
					base.Info("成功删除本地LDAP用户:", localUser.Username)
				}
			} else {
				if err := Set(&localUser); err != nil {
					base.Error("更新用户组失败:", localUser.Username, err)
				} else {
					base.Info("成功从组 '"+g.Name+"' 移除用户:", localUser.Username)
				}
			}
		}
	}
	return nil
}

// 同步所有 LDAP 认证源用户（按各 LDAP 认证源的 sync_users 独立控制）
func SyncLdapUsers() {
	groups, err := GetAllGroups()
	if err != nil {
		base.Error("获取所有组失败:", err)
		return
	}
	for _, g := range groups {
		if !HasAuthType(g.AuthProfile, "ldap") {
			continue
		}
		authLdap, err := ResolveLdapConfig(&g)
		if err != nil {
			base.Error("解析LDAP配置失败:", g.Name, err)
			continue
		}
		if !authLdap.SyncUsers {
			continue
		}
		go func(g Group, authldap *AuthLdap) {
			if err := authldap.SaveUsers(&g); err != nil {
				base.Error("LDAP 自动同步失败", g.Name, err)
			} else {
				base.Info("LDAP用户同步成功", g.Name)
			}
		}(g, authLdap)
	}
}
