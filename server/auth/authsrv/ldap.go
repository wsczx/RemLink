package authsrv

import (
	"fmt"

	"github.com/go-ldap/ldap"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
)

func init() {
	auth.Registry.Register("ldap", func() auth.Authenticator {
		return &LDAPAuth{}
	})
}

type LDAPAuth struct {
	auth.NopChallenger
	auth.LDAPConfig
}

func (a *LDAPAuth) Name() string { return "ldap" }

func (a *LDAPAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	if ctx.Conn.Username == "" || len(ctx.Conn.Password) < 1 {
		return auth.StepFail, fmt.Errorf("LDAP 用户名或密码为空")
	}

	a.Defaults()

	password := ctx.Conn.Password

	conn, err := a.Connect()
	if err != nil {
		return auth.StepFail, fmt.Errorf("LDAP 连接失败: %w", err)
	}
	defer conn.Close()

	sr, err := a.SearchUsers(conn, ctx.Conn.Username, []string{"displayName", "cn", "name", "userAccountControl", "accountExpires", "shadowExpire"})
	if err != nil {
		return auth.StepFail, fmt.Errorf("LDAP 查询失败: %w", err)
	}

	if len(sr.Entries) != 1 {
		if len(sr.Entries) == 0 {
			return auth.StepFail, fmt.Errorf("LDAP 找不到用户 %s", ctx.Conn.Username)
		}
		return auth.StepFail, fmt.Errorf("LDAP 发现多个 %s 账号", ctx.Conn.Username)
	}

	if err := auth.CheckAccountStatus(sr); err != nil {
		return auth.StepFail, fmt.Errorf("LDAP 用户 %s %w", ctx.Conn.Username, err)
	}

	userDN := sr.Entries[0].DN

	// 完整密码 bind
	if err := conn.Bind(userDN, password); err == nil {
		ctx.SetInfo("LDAP 认证通过")
		ctx.Conn.Nickname = ldapDisplayName(sr)
		return auth.StepPass, nil
	}

	base.Warn("LDAP 认证失败: user=", ctx.Conn.Username)
	return auth.StepFail, fmt.Errorf("LDAP 用户名或密码错误")
}

// 从 LDAP 搜索结果条目中取用户真实姓名
// 优先 displayName，回退 cn / name
func ldapDisplayName(sr *ldap.SearchResult) string {
	if sr == nil || len(sr.Entries) == 0 {
		return ""
	}
	entry := sr.Entries[0]
	for _, name := range []string{"displayName", "cn", "name"} {
		for _, attr := range entry.Attributes {
			if attr.Name == name && len(attr.Values) > 0 && attr.Values[0] != "" {
				return attr.Values[0]
			}
		}
	}
	return ""
}
