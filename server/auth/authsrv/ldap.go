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
		return auth.StepPending, nil
	}

	a.Defaults()

	password := ctx.Conn.Password

	conn, err := a.Connect()
	if err != nil {
		return auth.StepFail, fmt.Errorf("LDAP 连接失败: %w", err)
	}
	defer conn.Close()

	sr, err := a.SearchUsers(conn, ctx.Conn.Username, []string{"displayName", "cn", "name"})
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

	// 用户已启用 OTP 且非门户登录时，将密码尾 6 位作为动态码剥离后重试 bind
	ub := ctx.UserInfoLoaded()
	if ub == nil {
		LoadUserInfo(ctx)
		ub = ctx.UserInfoLoaded()
	}
	if ub != nil && ub.OtpSecret != "" && !ub.DisableOtp && !ctx.PortalLogin && len(password) > 6 {
		pl := len(password)
		strippedPwd := password[:pl-6]
		otpSuffix := password[pl-6:]
		if err := conn.Bind(userDN, strippedPwd); err == nil {
			otp := ctx.GetOTP()
			otp.Code = otpSuffix
			base.Debug("LDAP 认证（剥离OTP后缀兜底）: user=", ctx.Conn.Username)
			ctx.SetInfo("LDAP 认证通过")
			ctx.Conn.Nickname = ldapDisplayName(sr)
			return auth.StepPass, nil
		}
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
