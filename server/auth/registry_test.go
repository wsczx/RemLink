package auth

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 临时注册 mock 认证器，返回清理函数
func testRegister(name string, a Authenticator) func() {
	Registry.Register(name, func() Authenticator { return a })
	return func() {
		unregister(name)
	}
}

// alwaysFailAuth 始终认证失败的 mock
type alwaysFailAuth struct{ NopChallenger }

func (a *alwaysFailAuth) Name() string { return "always_fail" }
func (a *alwaysFailAuth) Authenticate(*Context) (StepResult, error) {
	return StepFail, fmt.Errorf("always fail")
}

// providerAuth 需要 Provider 配置的 mock
type providerAuth struct {
	NopChallenger
	cfg map[string]any
}

func (a *providerAuth) Name() string { return "provider_auth" }

func (a *providerAuth) Authenticate(ctx *Context) (StepResult, error) {
	if a.cfg == nil {
		return StepFail, fmt.Errorf("未配置 Provider")
	}
	return StepPass, nil
}

func mockResolver(cfg map[string]any) ProviderResolverFunc {
	return func(name, typ string) (map[string]any, error) {
		if cfg == nil {
			return nil, fmt.Errorf("provider %q 不存在", name)
		}
		return cfg, nil
	}
}

func TestRegister_Duplicate_Panics(t *testing.T) {
	ast := assert.New(t)
	cleanup := testRegister("dup_test", &mockAuth{name: "dup_test", result: StepPass})
	defer cleanup()

	ast.Panics(func() {
		Registry.Register("dup_test", func() Authenticator {
			return &mockAuth{name: "dup_test", result: StepPass}
		})
	}, "重复注册应 panic")
}

func TestGetFactory_Exists(t *testing.T) {
	cleanup := testRegister("factory_test", &mockAuth{name: "factory_test", result: StepPass})
	defer cleanup()

	f, ok := Registry.GetFactory("factory_test")
	assert.True(t, ok)
	assert.NotNil(t, f)

	inst := f()
	assert.Equal(t, "factory_test", inst.Name())
}

func TestGetFactory_NotExists(t *testing.T) {
	f, ok := Registry.GetFactory("not_register_abc123")
	assert.False(t, ok)
	assert.Nil(t, f)
}

func TestIsRegistered(t *testing.T) {
	cleanup := testRegister("reg_check", &mockAuth{name: "reg_check", result: StepPass})
	defer cleanup()

	assert.True(t, Registry.IsRegistered("reg_check"))
	assert.False(t, Registry.IsRegistered("not_there"))
}

func TestRegisteredNames(t *testing.T) {
	cleanup1 := testRegister("names_a", &mockAuth{name: "names_a", result: StepPass})
	cleanup2 := testRegister("names_b", &mockAuth{name: "names_b", result: StepPass})
	defer func() { cleanup1(); cleanup2() }()

	names := Registry.RegisteredNames()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "names_a")
	assert.Contains(t, names, "names_b")
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			Registry.IsRegistered(fmt.Sprintf("con_%d", n))
			Registry.RegisteredNames()
			Registry.GetFactory(fmt.Sprintf("con_%d", n))
		}(i)
	}
	wg.Wait()
}

func TestGetPipeline_EmptySteps(t *testing.T) {
	pipeline, err := GetPipeline(GroupAuthProfile{}, nil)
	assert.Nil(t, pipeline)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "未包含任何步骤")
}

func TestGetPipeline_SingleStep(t *testing.T) {
	cleanup := testRegister("build_single", &mockAuth{name: "build_single", result: StepPass})
	defer cleanup()

	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "build_single"}}}
	pipeline, err := GetPipeline(profile, nil)

	assert.Nil(t, err)
	assert.NotNil(t, pipeline)
	assert.Len(t, pipeline.Steps, 1)
	assert.Equal(t, "build_single", pipeline.Steps[0].Name())
}

func TestGetPipeline_MultiStep(t *testing.T) {
	cleanup1 := testRegister("ms1", &mockAuth{name: "ms1", result: StepPass})
	cleanup2 := testRegister("ms2", &mockAuth{name: "ms2", result: StepPass})
	cleanup3 := testRegister("ms3", &mockAuth{name: "ms3", result: StepPass})
	defer func() { cleanup1(); cleanup2(); cleanup3() }()

	profile := GroupAuthProfile{Step: []AuthMethodConfig{
		{Type: "ms1"},
		{Type: "ms2"},
		{Type: "ms3"},
	}}
	pipeline, err := GetPipeline(profile, nil)

	assert.Nil(t, err)
	assert.NotNil(t, pipeline)
	assert.Len(t, pipeline.Steps, 3)
}

func TestGetPipeline_UnknownType(t *testing.T) {
	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "not_existing_xyz"}}}
	pipeline, err := GetPipeline(profile, nil)

	assert.Nil(t, pipeline)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "未知认证类型")
	assert.Contains(t, err.Error(), "not_existing_xyz")
}

func TestGetPipeline_WithProvider_NoResolver(t *testing.T) {
	cleanup := testRegister("need_provider", &providerAuth{})
	defer cleanup()

	profile := GroupAuthProfile{Step: []AuthMethodConfig{
		{Type: "need_provider", Provider: "my-ldap"},
	}}
	pipeline, err := GetPipeline(profile, nil)

	assert.Nil(t, pipeline)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "未提供解析器")
}

func TestGetPipeline_WithProvider_ResolverFails(t *testing.T) {
	cleanup := testRegister("prov_fail", &providerAuth{})
	defer cleanup()

	resolver := func(name, typ string) (map[string]any, error) {
		return nil, fmt.Errorf("连接 %s 失败", name)
	}
	profile := GroupAuthProfile{Step: []AuthMethodConfig{
		{Type: "prov_fail", Provider: "bad-ldap"},
	}}
	pipeline, err := GetPipeline(profile, resolver)

	assert.Nil(t, pipeline)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "解析 Provider")
	assert.Contains(t, err.Error(), "连接 bad-ldap 失败")
}

func TestGetPipeline_WithProvider_Success(t *testing.T) {
	cleanup := testRegister("prov_ok", &providerAuth{})
	defer cleanup()

	resolver := mockResolver(map[string]any{"addr": "127.0.0.1:389"})
	profile := GroupAuthProfile{Step: []AuthMethodConfig{
		{Type: "prov_ok", Provider: "my-ldap"},
	}}
	pipeline, err := GetPipeline(profile, resolver)

	assert.Nil(t, err)
	assert.NotNil(t, pipeline)
	assert.Len(t, pipeline.Steps, 1)
}

func TestGetPipeline_UnknownType_StepIndexInError(t *testing.T) {
	cleanup1 := testRegister("valid_step", &mockAuth{name: "valid_step", result: StepPass})
	defer cleanup1()

	profile := GroupAuthProfile{Step: []AuthMethodConfig{
		{Type: "valid_step"},
		{Type: "nobody"},
		{Type: "valid_step"},
	}}
	pipeline, err := GetPipeline(profile, nil)

	assert.Nil(t, pipeline)
	assert.NotNil(t, err)
	// 错误应指明步骤 1（从0开始计数）
	assert.Contains(t, err.Error(), "步骤 1")
}

func TestGetPipeline_EmptyProvider_NoOp(t *testing.T) {
	cleanup := testRegister("no_prov", &mockAuth{name: "no_prov", result: StepPass})
	defer cleanup()

	// Provider 为空时不应调用 resolver，直接返回管道
	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "no_prov"}}}
	pipeline, err := GetPipeline(profile, nil)

	assert.Nil(t, err)
	assert.NotNil(t, pipeline)
}

func TestGetPipeline_EmptyConfigMap_NoOp(t *testing.T) {
	cleanup := testRegister("empty_cfg", &providerAuth{})
	defer cleanup()

	// resolver 返回空 map 时不应报错
	resolver := mockResolver(map[string]any{})
	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "empty_cfg", Provider: "empty-prov"}}}
	pipeline, err := GetPipeline(profile, resolver)

	assert.Nil(t, err)
	assert.NotNil(t, pipeline)
}

func TestParseAuthProfile_Valid(t *testing.T) {
	raw := json.RawMessage(`{"step":[{"type":"local"},{"type":"otp"}]}`)
	profile, err := ParseAuthProfile(raw)

	assert.Nil(t, err)
	assert.NotNil(t, profile)
	assert.Len(t, profile.Step, 2)
	assert.Equal(t, "local", profile.Step[0].Type)
	assert.Equal(t, "otp", profile.Step[1].Type)
}

func TestParseAuthProfile_WithProvider(t *testing.T) {
	raw := json.RawMessage(`{"step":[{"type":"ldap","provider":"北京LDAP"},{"type":"otp"}]}`)
	profile, err := ParseAuthProfile(raw)

	assert.Nil(t, err)
	assert.Len(t, profile.Step, 2)
	assert.Equal(t, "ldap", profile.Step[0].Type)
	assert.Equal(t, "北京LDAP", profile.Step[0].Provider)
}

func TestParseAuthProfile_EmptyRaw(t *testing.T) {
	profile, err := ParseAuthProfile(json.RawMessage{})
	assert.Nil(t, profile)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "认证配置为空")
}

func TestParseAuthProfile_NilRaw(t *testing.T) {
	profile, err := ParseAuthProfile(nil)
	assert.Nil(t, profile)
	assert.NotNil(t, err)
}

func TestParseAuthProfile_EmptySteps(t *testing.T) {
	raw := json.RawMessage(`{"step":[]}`)
	profile, err := ParseAuthProfile(raw)
	assert.Nil(t, profile)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "认证步骤为空")
}

func TestParseAuthProfile_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{not json}`)
	profile, err := ParseAuthProfile(raw)
	assert.Nil(t, profile)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "解析认证配置失败")
}

func TestParseAuthProfile_OTPFirst_Invalid(t *testing.T) {
	raw := json.RawMessage(`{"step":[{"type":"otp"},{"type":"local"}]}`)
	profile, err := ParseAuthProfile(raw)
	assert.Nil(t, profile)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "第一步不能是 OTP")
}

func TestParseAuthProfile_OTPOnly_Invalid(t *testing.T) {
	raw := json.RawMessage(`{"step":[{"type":"otp"}]}`)
	profile, err := ParseAuthProfile(raw)
	assert.Nil(t, profile)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "第一步不能是 OTP")
}

func TestGetProviderConfigFromMap_Valid(t *testing.T) {
	type ldapConfig struct {
		Addr   string `json:"addr"`
		BaseDN string `json:"base_dn"`
	}
	var target ldapConfig
	cfg := map[string]any{"addr": "10.0.0.1:389", "base_dn": "dc=example,dc=com"}
	err := GetProviderConfigFromMap(cfg, &target)

	assert.Nil(t, err)
	assert.Equal(t, "10.0.0.1:389", target.Addr)
	assert.Equal(t, "dc=example,dc=com", target.BaseDN)
}

func TestGetProviderConfigFromMap_EmptyMap(t *testing.T) {
	var target map[string]any
	err := GetProviderConfigFromMap(map[string]any{}, &target)
	assert.Nil(t, err)
	assert.Empty(t, target)
}
