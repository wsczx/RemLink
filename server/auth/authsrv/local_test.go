package authsrv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

// ========== 辅助函数 ==========

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

// ========== Name ==========

func TestLocalAuth_Name(t *testing.T) {
	la := &LocalAuth{}
	assert.Equal(t, "local", la.Name())
}

// ========== Authenticate 测试 ==========

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

// ========== ldap 类型用户不能本地认证 ==========

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

// ========== DisableOtp 用户正常登录 ==========

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

// ========== 已提供独立 otp_code 时 local 不修改它 ==========

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

// ========== 密码+OTP 合并输入（兜底剥离） ==========

func TestLocalAuth_PasswordWithOTPEmbedded(t *testing.T) {
	preTestData(t)
	createTestUser("testembed", "testpas0", false)
	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testembed",
			Password:  "testpas0" + "123456",
			GroupName: "default",
		},
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepPass, result)
	assert.Nil(t, err)
	// 兜底剥离后，后 6 位应作为动态码注入 OTP 状态
	otpCode := ""
	if ctx.OTP != nil {
		otpCode = ctx.OTP.Code
	}
	assert.Equal(t, "123456", otpCode)
}

func TestLocalAuth_PasswordWithOTPEmbedded_WrongPwd(t *testing.T) {
	preTestData(t)
	createTestUser("testembed2", "testpass123", false)
	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testembed2",
			Password:  "wrongpw_123456", // 密码部分错误
			GroupName: "default",
		},
	}
	result, err := la.Authenticate(ctx)

	// 剥离后密码仍不匹配 → 完整密码也不匹配 → 失败
	assert.Equal(t, auth.StepFail, result)
	assert.NotNil(t, err)
}

func TestLocalAuth_PortalLogin_NoStrip(t *testing.T) {
	preTestData(t)
	// 门户登录不剥离：合并输入应整体作为密码验证，因不匹配而失败
	createTestUser("testportal", "testpass123", false)
	la := &LocalAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testportal",
			Password:  "testpass123" + "123456",
			GroupName: "default",
		},
		PortalLogin: true,
	}
	result, err := la.Authenticate(ctx)

	assert.Equal(t, auth.StepFail, result)
	assert.NotNil(t, err)
}

// ========== nil Extra ==========

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
