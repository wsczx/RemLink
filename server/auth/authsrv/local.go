package authsrv

import (
	"fmt"
	"slices"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

func init() {
	auth.Registry.Register("local", func() auth.Authenticator {
		return &LocalAuth{}
	})
}

type LocalAuth struct {
	auth.NopChallenger
}

func (a *LocalAuth) Name() string { return "local" }

func (a *LocalAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	if ctx.Conn.Username == "" || len(ctx.Conn.Password) < 6 {
		return auth.StepFail, fmt.Errorf("用户名或密码格式错误")
	}

	v := &dbdata.User{}
	err := dbdata.One("Username", ctx.Conn.Username, v)
	if err != nil {
		// 防用户枚举：用户名不存在与密码错误返回一致的文案
		if dbdata.CheckErrNotFound(err) {
			base.Debug("local auth account invalid:", ctx.Conn.Username)
		} else {
			base.Warn("local auth query user failed:", ctx.Conn.Username, err)
		}
		return auth.StepFail, fmt.Errorf("用户名或密码错误")
	}
	if v.Status != 1 {
		return auth.StepFail, fmt.Errorf("用户名或密码错误")
	}

	// 将用户信息写入 ctx.UserInfo，供后续步骤（otp 等）共享
	ctx.SetUserInfo(v.ToAuthInfo())
	ctx.Conn.Nickname = v.Nickname

	// 先校验密码，再校验组/类型等配置项：避免在校验密码前按组归属泄露用户名是否存在
	if !dbdata.VerifyPassword(ctx.Conn.Password, v.PinCode) {
		return auth.StepFail, fmt.Errorf("用户名或密码错误")
	}

	// 实时检查过期时间
	if v.IsExpired() {
		return auth.StepFail, fmt.Errorf("用户已过期")
	}

	if v.Type == "ldap" {
		return auth.StepFail, fmt.Errorf("LDAP 用户不能使用本地认证")
	}

	if !slices.Contains(v.Groups, ctx.Conn.GroupName) {
		return auth.StepFail, fmt.Errorf("用户组错误")
	}

	return auth.StepPass, nil
}
