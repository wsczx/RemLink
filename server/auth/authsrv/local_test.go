package authsrv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

func createTestUser(username, password string, disableOtp bool) *dbdata.User {
	hashedPwd, _ := utils.PasswordHash(password)
	u := &dbdata.User{
		Username:   username,
		PinCode:    hashedPwd,
		Groups:     []string{"default"},
		Status:     1,
		DisableOtp: disableOtp,
	}
	// OtpSecret 为 "" 会被 SetUser 自动填充，所以需要 DisableOtp=true 来跳过 OTP 检查
	err := dbdata.SetUser(u)
	if err != nil {
		panic("createTestUser failed: " + err.Error())
	}
	return u
}

func TestLocalAuth_Name(t *testing.T) {
	la := &LocalAuth{}
	assert.Equal(t, "local", la.Name())
}

func TestLocalAuth_EmptyUsername(t *testing.T) {
	preTestData(t)

	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "",
			Password:  "testpass123",
			GroupName: "default",
		},
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepFail, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "格式错误")
}

func TestLocalAuth_ShortPassword(t *testing.T) {
	preTestData(t)

	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testuser",
			Password:  "abc", // 少于 6 位
			GroupName: "default",
		},
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepFail, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "格式错误")
}

func TestLocalAuth_UserNotFound(t *testing.T) {
	preTestData(t)

	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "nonexistent_user_xyz",
			Password:  "testpass123",
			GroupName: "default",
		},
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepFail, result)
	assert.NotNil(t, err)
}

func TestLocalAuth_WrongPassword(t *testing.T) {
	preTestData(t)
	createTestUser("testlocal", "correctpass1", true)

	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testlocal",
			Password:  "wrongpass123",
			GroupName: "default",
		},
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepFail, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "密码错误")
}

func TestLocalAuth_Success(t *testing.T) {
	preTestData(t)
	u := createTestUser("testlocal2", "testpass123", true)

	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testlocal2",
			Password:  "testpass123",
			GroupName: "default",
		},
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepPass, result)
	assert.Nil(t, err)

	ub := ctx.UserInfoLoaded()
	assert.NotNil(t, ub)
	assert.Equal(t, u.OtpSecret, ub.OtpSecret)
	assert.Equal(t, u.DisableOtp, ub.DisableOtp)
}

func TestLocalAuth_WrongGroup(t *testing.T) {
	preTestData(t)
	createTestUser("testlocal3", "testpass123", true)

	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testlocal3",
			Password:  "testpass123",
			GroupName: "wronggroup",
		},
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepFail, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "用户组错误")
}

func TestLocalAuth_LdapUser(t *testing.T) {
	preTestData(t)
	hashedPwd, _ := utils.PasswordHash("testpass123")
	u := &dbdata.User{
		Username:   "ldapuser1",
		PinCode:    hashedPwd,
		Groups:     []string{"default"},
		Status:     1,
		DisableOtp: true,
		Type:       "ldap",
	}
	_ = dbdata.SetUser(u)

	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "ldapuser1",
			Password:  "testpass123",
			GroupName: "default",
		},
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepFail, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "LDAP 用户不能使用本地认证")
}

func TestLocalAuth_WithOtpDisabled(t *testing.T) {
	preTestData(t)
	pwd := "abcdef123456"
	hashedPwd, _ := utils.PasswordHash(pwd)
	u := &dbdata.User{
		Username:   "testnostrip",
		PinCode:    hashedPwd,
		Groups:     []string{"default"},
		Status:     1,
		DisableOtp: true,
	}
	_ = dbdata.SetUser(u)

	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testnostrip",
			Password:  pwd,
			GroupName: "default",
		},
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepPass, result)
	assert.Nil(t, err)
}

func TestLocalAuth_ExistingOtpCode(t *testing.T) {
	preTestData(t)
	hashedPwd, _ := utils.PasswordHash("testpass123456")
	u := &dbdata.User{
		Username:   "testnocode",
		PinCode:    hashedPwd,
		Groups:     []string{"default"},
		Status:     1,
		DisableOtp: false,
	}
	_ = dbdata.SetUser(u)

	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testnocode",
			Password:  "testpass123456",
			GroupName: "default",
		},
		OTP: &auth.OTPState{Code: "999999"}, // 已经提供了独立 OTP 码
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepPass, result)
	assert.Nil(t, err)
	// local 步骤不处理 OTP，应保留调用方注入的码
	assert.Equal(t, "999999", ctx.OTP.Code)
}

func TestLocalAuth_ExistingOtpCode_WrongPwd(t *testing.T) {
	preTestData(t)
	hashedPwd, _ := utils.PasswordHash("testpass123456")
	u := &dbdata.User{
		Username:   "testcodewp",
		PinCode:    hashedPwd,
		Groups:     []string{"default"},
		Status:     1,
		DisableOtp: false,
	}
	_ = dbdata.SetUser(u)

	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testcodewp",
			Password:  "wrongpwd", // 密码错误
			GroupName: "default",
		},
		OTP: &auth.OTPState{Code: "123456"},
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepFail, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "密码错误")
}

func TestLocalAuth_PasswordWithOtpSuffix_NotAccepted(t *testing.T) {
	preTestData(t)
	createTestUser("testsuffix", "testpass123", false)
	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testsuffix",
			Password:  "testpass123" + "123456",
			GroupName: "default",
		},
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepFail, result)
	assert.NotNil(t, err)
	assert.True(t, ctx.OTP == nil || ctx.OTP.Code == "",
		"local 步骤不得再向 OTP 状态注入动态码")

	ctxOK := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testsuffix",
			Password:  "testpass123",
			GroupName: "default",
		},
	}
	resOK, errOK := la.Authenticate(ctxOK)
	assert.Equal(t, auth.StepPass, resOK)
	assert.Nil(t, errOK)
}

func TestLocalAuth_NilExtra(t *testing.T) {
	preTestData(t)
	createTestUser("testnil", "testpass123", true)

	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testnil",
			Password:  "testpass123",
			GroupName: "default",
		},
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepPass, result)
	assert.Nil(t, err)
}
