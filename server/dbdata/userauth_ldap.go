package dbdata

import (
	"fmt"

	"github.com/go-ldap/ldap"
	"github.com/wsczx/remlink/auth"
)

// LDAP 认证配置（嵌入共享 LDAPConfig，Connect/SearchFilter/SearchUsers 等方法由嵌入体提供）
type AuthLdap struct {
	auth.LDAPConfig
}

// 从 Group 的 AuthProfile 中通过 Provider 名称解析 LDAP 配置
func ResolveLdapConfig(g *Group) (*AuthLdap, error) {
	cfg, err := resolveProviderConfigMap(g, "ldap", "LDAP")
	if err != nil {
		return nil, err
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
		"shadowExpire",       // LDAP用户状态
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

	needOTP := a.EnableOtp || HasAuthType(g.AuthProfile, "otp")
	remote := make(map[string]RemoteUser)
	for _, entry := range sr.Entries {
		if a.SyncUserStatus {
			// 检查用户状态，只同步正常用户
			if err := auth.CheckAccountStatus(&ldap.SearchResult{Entries: []*ldap.Entry{entry}}); err != nil {
				continue
			}
		}
		id := entry.GetAttributeValue(a.SearchAttr)
		remote[id] = RemoteUser{
			ID:    id,
			Name:  entry.GetAttributeValue("displayName"),
			Phone: ldapPhone(entry),
			Email: entry.GetAttributeValue("mail"),
		}
	}
	// 判据是过滤前的原始搜索结果（与旧行为一致：全部被状态过滤时仍执行清理）
	if len(sr.Entries) == 0 {
		return fmt.Errorf("LDAP 搜索到的用户列表为空（可能连接失败、base DN 配置错误或搜索权限不足），组: %s", g.Name)
	}

	// LDAP 语义：本地已填写的字段不覆盖（保留本地编辑）；无拒绝名单
	return syncExternalUsers(g, "ldap", "LDAP", remote, needOTP, nil, true)
}

// 同步所有 LDAP 认证源用户（按各 LDAP 认证源的 sync_users 独立控制）
func SyncLdapUsers() {
	syncExternalUsersBySource(extSyncSource{
		authType: "ldap",
		name:     "LDAP",
		resolve: func(g *Group) (bool, func(*Group) error, error) {
			cfg, err := ResolveLdapConfig(g)
			if err != nil {
				return false, nil, err
			}
			return cfg.SyncUsers, cfg.SaveUsers, nil
		},
	})
}
