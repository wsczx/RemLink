package authsrv

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xlzd/gotp"
)

func TestCheckOtp_ValidCode(t *testing.T) {
	ast := assert.New(t)

	secret := gotp.RandomSecret(16)
	totp := gotp.NewDefaultTOTP(secret)
	code := totp.Now()

	result := CheckOtp("testuser", code, secret)
	ast.True(result, "正确 OTP 应通过验证")
}

func TestCheckOtp_InvalidCode(t *testing.T) {
	ast := assert.New(t)

	secret := "JBSWY3DPEHPK3PXP"
	result := CheckOtp("testuser", "000000", secret)
	ast.False(result, "错误 OTP 应拒绝")
}

func TestCheckOtp_ReplayAttack(t *testing.T) {
	ast := assert.New(t)

	secret := gotp.RandomSecret(16)
	totp := gotp.NewDefaultTOTP(secret)
	code := totp.Now()

	// 第一次通过
	result1 := CheckOtp("repeat_user", code, secret)
	ast.True(result1, "首次使用应通过")

	// 第二次重放 -> 拒绝
	result2 := CheckOtp("repeat_user", code, secret)
	ast.False(result2, "重放应被拒绝")
}

func TestCheckOtp_ExpiredCleanup(t *testing.T) {
	ast := assert.New(t)

	secret := gotp.RandomSecret(16)
	totp := gotp.NewDefaultTOTP(secret)
	code := totp.Now()

	// 首次使用写入缓存
	CheckOtp("cleanup_test", code, secret)

	userOtpMux.Lock()
	key := "cleanup_test:" + code
	_, exists := userOtp[key]
	ast.True(exists, "OTP 记录应存在")
	delete(userOtp, key)
	userOtpMux.Unlock()

	// 记录已清理，应允许再次使用
	result := CheckOtp("cleanup_test", code, secret)
	ast.True(result, "过期清理后应允许再次使用")
}

func TestCheckOtp_DifferentUsersSameCode(t *testing.T) {
	ast := assert.New(t)

	secret1 := gotp.RandomSecret(16)
	totp1 := gotp.NewDefaultTOTP(secret1)
	code1 := totp1.Now()

	secret2 := gotp.RandomSecret(16)
	// 不同用户用同一 code 但不同 secret 不冲突
	result := CheckOtp("user_a", code1, secret1)
	ast.True(result)
	result = CheckOtp("user_b", code1, secret2)
	// 不同 secret 下 code 大概率不同，这里主要测不会误报重放
	// 如果巧合相同 code，重放保护 key 是 "user:code"
}

func TestCheckOtp_EmptySecret(t *testing.T) {
	ast := assert.New(t)
	result := CheckOtp("nouser", "123456", "")
	ast.False(result, "空 secret 应拒绝")
}

func TestOTPAuth_Name(t *testing.T) {
	oa := &OTPAuth{}
	assert.Equal(t, "otp", oa.Name())
}

func TestOTPAuth_Challenge(t *testing.T) {
	ast := assert.New(t)
	oa := &OTPAuth{}
	ch := oa.Challenge()
	ast.NotNil(ch)
	ast.Equal(auth.ChallengeOTP, ch.Type)
	ast.Equal("otp", ch.Template)
}

func TestOTPAuth_Disabled(t *testing.T) {
	preTestData(t)
	hashedPwd, _ := utils.PasswordHash("testpass123")
	u := &dbdata.User{
		Username:   "otpdisabled",
		PinCode:    hashedPwd,
		Groups:     []string{"default"},
		Status:     1,
		DisableOtp: true,
	}
	_ = dbdata.SetUser(u)

	oa := &OTPAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username: "otpdisabled",
		},
		UserInfo: &auth.UserInfo{DisableOtp: true},
	}
	result, err := oa.Authenticate(ctx)

	assert.Equal(t, auth.StepPass, result)
	assert.Nil(t, err)
}

func TestOTPAuth_NoCode_StepPending(t *testing.T) {
	preTestData(t)
	hashedPwd, _ := utils.PasswordHash("testpass123")
	u := &dbdata.User{
		Username:   "otpwcode",
		PinCode:    hashedPwd,
		Groups:     []string{"default"},
		Status:     1,
		DisableOtp: false,
	}
	_ = dbdata.SetUser(u)

	oa := &OTPAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username: "otpwcode",
		},
		UserInfo: &auth.UserInfo{OtpSecret: u.OtpSecret, DisableOtp: false},
	}
	result, err := oa.Authenticate(ctx)

	assert.Equal(t, auth.StepPending, result)
	assert.Nil(t, err)
	// SendOtp=false 时不应触发发送
}

func TestOTPAuth_InvalidCode_Retry(t *testing.T) {
	preTestData(t)
	hashedPwd, _ := utils.PasswordHash("testpass123")
	u := &dbdata.User{
		Username:   "otpbadcode",
		PinCode:    hashedPwd,
		Groups:     []string{"default"},
		Status:     1,
		DisableOtp: false,
	}
	_ = dbdata.SetUser(u)

	oa := &OTPAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username: "otpbadcode",
		},
		UserInfo: &auth.UserInfo{OtpSecret: u.OtpSecret},
		OTP:      &auth.OTPState{Code: "000000"},
	}
	result, err := oa.Authenticate(ctx)

	// OTP 错误返回 StepPending 允许重试（锁定由上层 LockManager 处理）
	assert.Equal(t, auth.StepPending, result)
	assert.Nil(t, err)
	code := ""
	if ctx.OTP != nil {
		code = ctx.OTP.Code
	}
	assert.Equal(t, "", code)
}

func TestOTPAuth_LoadFromDB(t *testing.T) {
	preTestData(t)
	hashedPwd, _ := utils.PasswordHash("testpass123")
	u := &dbdata.User{
		Username:   "otploaddb",
		PinCode:    hashedPwd,
		Groups:     []string{"default"},
		Status:     1,
		DisableOtp: false,
	}
	_ = dbdata.SetUser(u)

	oa := &OTPAuth{}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username: "otploaddb", // 无 otp_secret，触发 DB 加载
		},
	}
	// 因为没有 otp_code，会先返回 StepPending
	result, err := oa.Authenticate(ctx)

	// OTP 信息应从 DB 加载到 ctx.UserInfo（单一来源）
	ub := ctx.UserInfoLoaded()
	assert.NotNil(t, ub)
	assert.Equal(t, u.OtpSecret, ub.OtpSecret)
	assert.Equal(t, false, ub.DisableOtp)

	assert.Equal(t, auth.StepPending, result)
	assert.Nil(t, err)
}

func TestOTPAuth_CheckOtp_Concurrent(t *testing.T) {
	secret := gotp.RandomSecret(16)
	totp := gotp.NewDefaultTOTP(secret)
	code := totp.Now()

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// 不同用户名，同一 code+secret（各自独立）
			username := fmt.Sprintf("cuser%d", idx)
			CheckOtp(username, code, secret)
		}(i)
	}
	wg.Wait()
}

func TestOTPAuth_CheckOtp_Concurrent_SameUser(t *testing.T) {
	secret := gotp.RandomSecret(16)
	totp := gotp.NewDefaultTOTP(secret)
	code := totp.Now()

	var wg sync.WaitGroup
	results := make([]bool, 10)
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = CheckOtp("same_user", code, secret)
		}(i)
	}
	wg.Wait()

	// 同一用户+code 只能通过一次，其余因为重放保护全部返回 false
	passCount := 0
	for _, r := range results {
		if r {
			passCount++
		}
	}
	assert.Equal(t, 1, passCount, "同一用户+code 并发只有一次通过")
}

func TestSendOtpToUser_PhoneCallsFunc(t *testing.T) {
	ast := assert.New(t)
	preTestData(t)

	oldType := base.GetCfg().SendOtpType
	base.UpdateCfg(func(c *base.ServerConfig) { c.SendOtpType = "phone" })
	defer base.UpdateCfg(func(c *base.ServerConfig) { c.SendOtpType = oldType })

	oldFunc := SendOtpFunc
	var called bool
	SendOtpFunc = func(info *auth.UserInfo) error {
		called = true
		ast.Equal("u", info.Username)
		return nil
	}
	defer func() { SendOtpFunc = oldFunc }()

	sendOtpToUser(&auth.UserInfo{Username: "u"})
	ast.True(called, "phone 类型应调用 SendOtpFunc 发送验证码（修复前 default 分支直接 return）")
}

func TestSendOtpToUser_MailCallsFunc(t *testing.T) {
	ast := assert.New(t)
	preTestData(t)

	oldType := base.GetCfg().SendOtpType
	base.UpdateCfg(func(c *base.ServerConfig) { c.SendOtpType = "mail" })
	defer base.UpdateCfg(func(c *base.ServerConfig) { c.SendOtpType = oldType })

	oldFunc := SendOtpFunc
	var called bool
	SendOtpFunc = func(info *auth.UserInfo) error {
		called = true
		return nil
	}
	defer func() { SendOtpFunc = oldFunc }()

	sendOtpToUser(&auth.UserInfo{Username: "u"})
	ast.True(called, "mail 类型应调用 SendOtpFunc")
}

func TestSendOtpToUser_NilFuncNoPanic(t *testing.T) {
	ast := assert.New(t)
	preTestData(t)

	oldType := base.GetCfg().SendOtpType
	base.UpdateCfg(func(c *base.ServerConfig) { c.SendOtpType = "mail" })
	defer base.UpdateCfg(func(c *base.ServerConfig) { c.SendOtpType = oldType })

	oldFunc := SendOtpFunc
	SendOtpFunc = nil
	defer func() { SendOtpFunc = oldFunc }()

	ast.NotPanics(func() { sendOtpToUser(&auth.UserInfo{Username: "u"}) }, "SendOtpFunc 未注入时不应 panic")
}
