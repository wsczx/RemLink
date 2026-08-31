package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStepResult_String(t *testing.T) {
	ast := assert.New(t)

	ast.Equal("Pass", StepPass.String())
	ast.Equal("Pending", StepPending.String())
	ast.Equal("Fail", StepFail.String())
	ast.Equal("Unknown", StepResult(99).String())
}

func TestContext_LogInfo_WithSetInfo(t *testing.T) {
	ast := assert.New(t)

	ctx := &Context{}
	ctx.SetInfo("飞书 SSO 认证")
	ast.Equal("飞书 SSO 认证", ctx.LogInfo())
}

func TestContext_LogInfo_FromPassedSteps(t *testing.T) {
	ast := assert.New(t)

	ctx := &Context{}
	ctx.AddPassedStep("ldap")
	ctx.AddPassedStep("otp")
	ast.Equal("LDAP+OTP认证通过", ctx.LogInfo())
}

func TestContext_LogInfo_SingleStep(t *testing.T) {
	ast := assert.New(t)

	ctx := &Context{}
	ctx.AddPassedStep("local")
	ast.Equal("本地密码认证通过", ctx.LogInfo())
}

func TestContext_LogInfo_NoSteps(t *testing.T) {
	ast := assert.New(t)

	ctx := &Context{}
	ast.Equal("认证成功", ctx.LogInfo())
}

func TestContext_LogInfo_SetInfoPriority(t *testing.T) {
	ast := assert.New(t)

	ctx := &Context{}
	ctx.AddPassedStep("cert")
	ctx.AddPassedStep("otp")
	ast.Equal("证书+OTP认证通过", ctx.LogInfo())

	// 多步组合认证：SetInfo 不应覆盖完整认证流程
	ctx.SetInfo("自定义认证描述")
	ast.Equal("证书+OTP认证通过", ctx.LogInfo())

	// 单步认证：SetInfo 优先
	ctx2 := &Context{}
	ctx2.AddPassedStep("wxwork")
	ctx2.SetInfo("用户通过企业微信认证登录")
	ast.Equal("用户通过企业微信认证登录", ctx2.LogInfo())
}

func TestContext_LogInfo_UnknownStepType(t *testing.T) {
	ast := assert.New(t)

	ctx := &Context{}
	ctx.AddPassedStep("unknown_type_abc")
	ast.Equal("unknown_type_abc认证通过", ctx.LogInfo())
}

func TestBuildInfoFromSteps(t *testing.T) {
	tests := []struct {
		name     string
		steps    []string
		expected string
	}{
		{"empty", []string{}, "认证成功"},
		{"single local", []string{"local"}, "本地密码认证通过"},
		{"single ldap", []string{"ldap"}, "LDAP认证通过"},
		{"single radius", []string{"radius"}, "RADIUS认证通过"},
		{"single cert", []string{"cert"}, "证书认证通过"},
		{"single otp", []string{"otp"}, "OTP认证通过"},
		{"single wxwork", []string{"wxwork"}, "企微认证通过"},
		{"single feishu", []string{"feishu"}, "飞书认证通过"},
		{"single admin", []string{"admin"}, "管理员认证认证通过"},
		{"local+otp", []string{"local", "otp"}, "本地密码+OTP认证通过"},
		{"cert+otp", []string{"cert", "otp"}, "证书+OTP认证通过"},
		{"ldap+otp", []string{"ldap", "otp"}, "LDAP+OTP认证通过"},
		{"cert+ldap+otp", []string{"cert", "ldap", "otp"}, "证书+LDAP+OTP认证通过"},
		{"unknown", []string{"xxx"}, "xxx认证通过"},
		{"mixed known+unknown", []string{"local", "yyy"}, "本地密码+yyy认证通过"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildInfoFromSteps(tt.steps)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContext_AddPassedStep_Order(t *testing.T) {
	ast := assert.New(t)

	ctx := &Context{}
	ctx.AddPassedStep("cert")
	ctx.AddPassedStep("ldap")
	ctx.AddPassedStep("otp")

	ast.Len(ctx.passedSteps, 3)
	ast.Equal([]string{"cert", "ldap", "otp"}, ctx.passedSteps)
}

func TestContext_LogInfo_NilExtra(t *testing.T) {
	ctx := &Context{}
	ast := assert.New(t)
	ast.Equal("认证成功", ctx.LogInfo())
}

func TestContext_AddPassedStep_NilExtra(t *testing.T) {
	ctx := &Context{}
	ctx.AddPassedStep("local")
	ast := assert.New(t)
	ast.Equal("本地密码认证通过", ctx.LogInfo())
}
