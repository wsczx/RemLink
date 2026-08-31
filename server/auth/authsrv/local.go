package authsrv

import (
	"fmt"
	"time"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
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

	// 实时检查过期时间
	if v.LimitTime != nil && time.Now().After(*v.LimitTime) {
		return auth.StepFail, fmt.Errorf("用户已过期")
	}

	// 先校验密码，再校验组/类型等配置项：避免在校验密码前按组归属泄露用户名是否存在
	if !verifyLocalPassword(ctx, v) {
		return auth.StepFail, fmt.Errorf("用户名或密码错误")
	}

	if v.Type == "ldap" {
		return auth.StepFail, fmt.Errorf("LDAP 用户不能使用本地认证")
	}

	if !utils.InArrStr(v.Groups, ctx.Conn.GroupName) {
		return auth.StepFail, fmt.Errorf("用户组错误")
	}

	return auth.StepPass, nil
}

// 验证本地密码：完整密码优先，失败则兜底尝试"密码+OTP后缀"剥离
func verifyLocalPassword(ctx *auth.Context, v *dbdata.User) bool {
	tryPassword := func(pwd string) bool {
		return dbdata.VerifyPassword(pwd, v.PinCode)
	}

	if tryPassword(ctx.Conn.Password) {
		return true
	}

	// 兜底：用户已启用 OTP 且非门户登录时，将密码尾 6 位作为动态码剥离后重试
	if v.OtpSecret != "" && !v.DisableOtp && !ctx.PortalLogin && len(ctx.Conn.Password) > 6 {
		pl := len(ctx.Conn.Password)
		strippedPwd := ctx.Conn.Password[:pl-6]
		if tryPassword(strippedPwd) {
			otp := ctx.GetOTP()
			otp.Code = ctx.Conn.Password[pl-6:]
			base.Debug("本地认证（剥离OTP后缀兜底）: user=", ctx.Conn.Username)
			return true
		}
	}
	return false
}
