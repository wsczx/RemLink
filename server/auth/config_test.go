package auth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockSSOAuth struct{}

func (mockSSOAuth) Name() string                              { return "mock_sso" }
func (mockSSOAuth) Authenticate(*Context) (StepResult, error) { return StepPass, nil }
func (mockSSOAuth) Challenge() *ChallengeInfo                 { return &ChallengeInfo{Type: ChallengeSSO} }

func TestHasSSOAndCredential(t *testing.T) {
	ast := assert.New(t)
	cleanup := testRegister("mock_sso", mockSSOAuth{})
	defer cleanup()

	// SSO + 凭据 应被判定为冲突
	ast.True(hasSSOAndCredential([]AuthMethodConfig{{Type: "mock_sso"}, {Type: "local"}}))
	ast.True(hasSSOAndCredential([]AuthMethodConfig{{Type: "mock_sso"}, {Type: "ldap"}}))
	ast.True(hasSSOAndCredential([]AuthMethodConfig{{Type: "mock_sso"}, {Type: "radius"}}))
	// SSO + otp / 纯 SSO / 纯凭据 不应冲突
	ast.False(hasSSOAndCredential([]AuthMethodConfig{{Type: "mock_sso"}, {Type: "otp"}}))
	ast.False(hasSSOAndCredential([]AuthMethodConfig{{Type: "mock_sso"}}))
	ast.False(hasSSOAndCredential([]AuthMethodConfig{{Type: "local"}, {Type: "otp"}}))
}

func TestParseAuthProfile_SSORejected(t *testing.T) {
	ast := assert.New(t)
	cleanup := testRegister("mock_sso", mockSSOAuth{})
	defer cleanup()

	_, err := ParseAuthProfile(json.RawMessage(`{"step":[{"type":"mock_sso"},{"type":"local"}]}`))
	ast.Error(err)
	ast.Contains(err.Error(), "SSO")

	// 合法组合不被误伤：SSO + otp 可用（组保存时自动同步用户生成 OTP 密钥）
	_, err = ParseAuthProfile(json.RawMessage(`{"step":[{"type":"mock_sso"},{"type":"otp"}]}`))
	ast.NoError(err)
}
