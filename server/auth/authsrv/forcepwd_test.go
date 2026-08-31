package authsrv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

func createForcePwdUser(username, userType string, forcePwd bool) *dbdata.User {
	hashedPwd, _ := utils.PasswordHash("testpass123")
	u := &dbdata.User{
		Username:   username,
		PinCode:    hashedPwd,
		Groups:     []string{"default"},
		Status:     1,
		DisableOtp: true,
		Type:       userType,
		ForcePwd:   forcePwd,
	}
	if err := dbdata.SetUser(u); err != nil {
		panic("createForcePwdUser failed: " + err.Error())
	}
	return u
}

func TestForcePwd_Name(t *testing.T) {
	f := &ForcePwd{}
	assert.Equal(t, "forcepwd", f.Name())
}

func TestForcePwd_UserNotFound(t *testing.T) {
	preTestData(t)

	f := &ForcePwd{}
	ctx := &auth.Context{Conn: auth.ConnInfo{Username: "nonexistent_forcepwd_user"}}
	result, err := f.Authenticate(ctx)

	assert.Equal(t, auth.StepFail, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "查询用户失败")
}

// 本地用户（Type=""）ForcePwd=true → StepPending
func TestForcePwd_LocalEmptyType_Pending(t *testing.T) {
	preTestData(t)
	createForcePwdUser("fp_local_empty", "", true)

	f := &ForcePwd{}
	ctx := &auth.Context{Conn: auth.ConnInfo{Username: "fp_local_empty"}}
	result, err := f.Authenticate(ctx)

	assert.Equal(t, auth.StepPending, result)
	assert.Nil(t, err)
}

// 本地用户（Type="local"）ForcePwd=true → StepPending
func TestForcePwd_LocalType_Pending(t *testing.T) {
	preTestData(t)
	createForcePwdUser("fp_local_type", "local", true)

	f := &ForcePwd{}
	ctx := &auth.Context{Conn: auth.ConnInfo{Username: "fp_local_type"}}
	result, err := f.Authenticate(ctx)

	assert.Equal(t, auth.StepPending, result)
	assert.Nil(t, err)
}

// 本地用户 ForcePwd=false → StepPass
func TestForcePwd_LocalNoForce_Pass(t *testing.T) {
	preTestData(t)
	createForcePwdUser("fp_local_noforce", "", false)

	f := &ForcePwd{}
	ctx := &auth.Context{Conn: auth.ConnInfo{Username: "fp_local_noforce"}}
	result, err := f.Authenticate(ctx)

	assert.Equal(t, auth.StepPass, result)
	assert.Nil(t, err)
}

// 外部用户（Type=ldap）即便 ForcePwd=true 也 StepPass（密码在外部系统）
func TestForcePwd_ExternalUser_Pass(t *testing.T) {
	preTestData(t)
	createForcePwdUser("fp_ldap_user", "ldap", true)

	f := &ForcePwd{}
	ctx := &auth.Context{Conn: auth.ConnInfo{Username: "fp_ldap_user"}}
	result, err := f.Authenticate(ctx)

	assert.Equal(t, auth.StepPass, result)
	assert.Nil(t, err)
}

// resume 场景：ctx.UserInfo 为 nil，仍应按库中 ForcePwd 判定为 Pending（验证不依赖 UserInfo）
func TestForcePwd_ResumeNilUserInfo_Pending(t *testing.T) {
	preTestData(t)
	createForcePwdUser("fp_resume_nil", "", true)

	f := &ForcePwd{}
	ctx := &auth.Context{Conn: auth.ConnInfo{Username: "fp_resume_nil"}}
	assert.Nil(t, ctx.UserInfo)
	result, err := f.Authenticate(ctx)

	assert.Equal(t, auth.StepPending, result)
	assert.Nil(t, err)
}

// 信任管道内已加载的 ctx.UserInfo：即便库中 ForcePwd=true，
// 只要 ctx.UserInfo 已存在且 ForcePwd=false，即按 StepPass（不重复查库覆盖）。
func TestForcePwd_TrustInMemoryUserInfo(t *testing.T) {
	preTestData(t)
	createForcePwdUser("fp_trust", "", true) // 库中 ForcePwd=true

	f := &ForcePwd{}
	ctx := &auth.Context{
		Conn:     auth.ConnInfo{Username: "fp_trust"},
		UserInfo: &auth.UserInfo{Username: "fp_trust", Type: "local", ForcePwd: false},
	}
	result, err := f.Authenticate(ctx)

	assert.Equal(t, auth.StepPass, result)
	assert.Nil(t, err)
}

// ctx 为 nil → Challenge 返回 nil
func TestForcePwd_Challenge_NilCtx(t *testing.T) {
	f := &ForcePwd{}
	assert.Nil(t, f.Challenge())
}

// 正常返回 ChallengeForcePwd 且带 username
func TestForcePwd_Challenge_ReturnsInfo(t *testing.T) {
	preTestData(t)
	createForcePwdUser("fp_challenge", "", true)

	f := &ForcePwd{}
	ctx := &auth.Context{Conn: auth.ConnInfo{Username: "fp_challenge"}}
	_, _ = f.Authenticate(ctx)

	ci := f.Challenge()
	assert.NotNil(t, ci)
	assert.Equal(t, auth.ChallengeForcePwd, ci.Type)
	assert.Equal(t, "fp_challenge", ci.Data["username"])
}

// 管道含 local → 在 local 之后插入 forcepwd 步骤
func TestInsertForcePwd_AfterLocal(t *testing.T) {
	preTestData(t)

	p := &auth.Pipeline{Steps: []auth.Authenticator{
		&testStubAuth{name: "local"},
		&testStubAuth{name: "otp"},
	}}
	insertForcePwd(p)

	names := make([]string, len(p.Steps))
	for i, s := range p.Steps {
		names[i] = s.Name()
	}
	assert.Equal(t, []string{"local", "forcepwd", "otp"}, names)
}

// 管道无 local → 不插入
func TestInsertForcePwd_NoLocal(t *testing.T) {
	preTestData(t)

	p := &auth.Pipeline{Steps: []auth.Authenticator{
		&testStubAuth{name: "ldap"},
		&testStubAuth{name: "otp"},
	}}
	insertForcePwd(p)

	names := make([]string, len(p.Steps))
	for i, s := range p.Steps {
		names[i] = s.Name()
	}
	assert.Equal(t, []string{"ldap", "otp"}, names)
}

// 仅在首个 local 之后插入一次
func TestInsertForcePwd_OnlyFirstLocal(t *testing.T) {
	preTestData(t)

	p := &auth.Pipeline{Steps: []auth.Authenticator{
		&testStubAuth{name: "local"},
		&testStubAuth{name: "local"},
	}}
	insertForcePwd(p)

	names := make([]string, len(p.Steps))
	for i, s := range p.Steps {
		names[i] = s.Name()
	}
	assert.Equal(t, []string{"local", "forcepwd", "local"}, names)
}
