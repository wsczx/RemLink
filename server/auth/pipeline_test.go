// 管道执行测试：覆盖 GetPipeline+Run/Resume 的构建、执行、恢复、边界路径。

package auth

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockAuth 可配置返回值的 mock 认证器
type mockAuth struct {
	name      string
	result    StepResult
	err       error
	challenge *ChallengeInfo
}

func (m *mockAuth) Name() string                              { return m.name }
func (m *mockAuth) Authenticate(*Context) (StepResult, error) { return m.result, m.err }
func (m *mockAuth) Challenge() *ChallengeInfo                 { return m.challenge }

// 在测试中临时注册 mock 认证器
func mockRegister(name string, a Authenticator) {
	Registry.Register(name, func() Authenticator { return a })
}

// 从注册表中移除 mock
func mockUnregister(name string) {
	unregister(name)
}

// 构建管道并执行，返回 PipelineResult（模拟 Service.Authenticate 但不需要 DB loader）。
func runPipeline(ctx *Context, profile GroupAuthProfile) *PipelineResult {
	pipeline, err := GetPipeline(profile, nil)
	if err != nil {
		return &PipelineResult{Result: StepFail, Err: err}
	}
	result, err := pipeline.Run(ctx)
	return buildResult(ctx, result, err, pipeline, -1)
}

// 构建管道并从指定步号恢复，返回 PipelineResult。
func resumePipeline(ctx *Context, profile GroupAuthProfile, state PipelineState) *PipelineResult {
	pipeline, err := GetPipeline(profile, nil)
	if err != nil {
		return &PipelineResult{Result: StepFail, Err: err}
	}
	ctx.passedSteps = state.PassedSteps
	result, err := pipeline.Resume(ctx, state.StepIdx)
	return buildResult(ctx, result, err, pipeline, state.StepIdx)
}

func buildResult(ctx *Context, result StepResult, err error, pipeline *Pipeline, prevStepIdx int) *PipelineResult {
	pr := &PipelineResult{
		Result:      result,
		Err:         err,
		Username:    ctx.Conn.Username,
		GroupName:   ctx.Conn.GroupName,
		Info:        ctx.LogInfo(),
		PrevStepIdx: prevStepIdx,
	}
	if result == StepPending {
		if c := pipeline.GetChallenger(); c != nil {
			pr.Challenge = c.Challenge()
		}
		pr.State = PipelineState{
			StepIdx:     pipeline.PendingStep(),
			PassedSteps: ctx.passedSteps,
		}
	}
	return pr
}

// ========== Run 测试 ==========

func TestExecute_SingleStep_Pass(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_pass", &mockAuth{name: "mock_pass", result: StepPass})
	defer mockUnregister("mock_pass")

	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "mock_pass"}}}
	ctx := &Context{}
	result := runPipeline(ctx, profile)

	ast.NotNil(result)
	ast.Equal(StepPass, result.Result)
	ast.Nil(result.Err)
	ast.Nil(result.Challenge)
}

func TestExecute_MultiStep_Pass(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_p1", &mockAuth{name: "mock_p1", result: StepPass})
	mockRegister("mock_p2", &mockAuth{name: "mock_p2", result: StepPass})
	mockRegister("mock_p3", &mockAuth{name: "mock_p3", result: StepPass})
	defer func() { mockUnregister("mock_p1"); mockUnregister("mock_p2"); mockUnregister("mock_p3") }()

	profile := GroupAuthProfile{Step: []AuthMethodConfig{
		{Type: "mock_p1"}, {Type: "mock_p2"}, {Type: "mock_p3"},
	}}
	ctx := &Context{}
	result := runPipeline(ctx, profile)

	ast.NotNil(result)
	ast.Equal(StepPass, result.Result)
	ast.Nil(result.Err)
}

func TestExecute_StepFails_StopsPipeline(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_fail", &mockAuth{name: "mock_fail", result: StepFail, err: fmt.Errorf("认证失败")})
	mockRegister("mock_next", &mockAuth{name: "mock_next", result: StepPass})
	defer func() { mockUnregister("mock_fail"); mockUnregister("mock_next") }()

	profile := GroupAuthProfile{Step: []AuthMethodConfig{
		{Type: "mock_fail"}, {Type: "mock_next"},
	}}
	ctx := &Context{}
	result := runPipeline(ctx, profile)

	ast.NotNil(result)
	ast.Equal(StepFail, result.Result)
	ast.NotNil(result.Err)
	ast.Contains(result.Err.Error(), "mock_fail")
}

func TestExecute_UnknownType(t *testing.T) {
	ast := assert.New(t)

	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "notexist_at_all"}}}
	ctx := &Context{}
	result := runPipeline(ctx, profile)

	ast.NotNil(result)
	ast.Equal(StepFail, result.Result)
	ast.NotNil(result.Err)
	ast.Contains(result.Err.Error(), "未知认证类型")
}

func TestExecute_EmptySteps(t *testing.T) {
	ast := assert.New(t)

	profile := GroupAuthProfile{Step: []AuthMethodConfig{}}
	ctx := &Context{}
	result := runPipeline(ctx, profile)

	ast.NotNil(result)
	ast.Equal(StepFail, result.Result)
	ast.NotNil(result.Err)
	ast.Contains(result.Err.Error(), "未包含任何步骤")
}

// ========== Resume 测试 ==========

func TestResume_ContinueAfterPending(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_start", &mockAuth{name: "mock_start", result: StepPass})
	mockRegister("mock_pending", &mockAuth{
		name:      "mock_pending",
		result:    StepPending,
		challenge: &ChallengeInfo{Type: ChallengeOTP, Template: "otp"},
	})
	defer func() { mockUnregister("mock_start"); mockUnregister("mock_pending") }()

	profile := GroupAuthProfile{Step: []AuthMethodConfig{
		{Type: "mock_start"}, {Type: "mock_pending"},
	}}

	// 首次执行：StepPass → StepPending
	ctx1 := &Context{Conn: ConnInfo{Username: "testuser", GroupName: "testgroup"}}
	result1 := runPipeline(ctx1, profile)

	ast.NotNil(result1)
	ast.Equal(StepPending, result1.Result)
	ast.NotNil(result1.Challenge)
	ast.Equal(1, result1.State.StepIdx)
	ast.Equal("testuser", result1.Username)
	ast.Equal("testgroup", result1.GroupName)

	// Resume 恢复：从 StepIdx=1 继续（mock_pending 仍返回 StepPending）
	ctx2 := &Context{Conn: ConnInfo{Username: "testuser", GroupName: "testgroup"}}
	result2 := resumePipeline(ctx2, profile, result1.State)

	ast.NotNil(result2)
	ast.Equal(StepPending, result2.Result)
}

func TestResume_AllStepsPassAfterPending(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_p2a", &mockAuth{name: "mock_p2a", result: StepPass})
	defer mockUnregister("mock_p2a")

	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "mock_p2a"}, {Type: "mock_p2a"}}}
	state := PipelineState{StepIdx: 1}
	ctx := &Context{}
	result := resumePipeline(ctx, profile, state)

	ast.NotNil(result)
	ast.Equal(StepPass, result.Result)
	ast.Nil(result.Err)
}

func TestResume_InvalidStepIdx(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_resume", &mockAuth{name: "mock_resume", result: StepPass})
	defer mockUnregister("mock_resume")

	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "mock_resume"}}}
	state := PipelineState{StepIdx: 5}
	ctx := &Context{}
	result := resumePipeline(ctx, profile, state)

	ast.NotNil(result)
	ast.Equal(StepFail, result.Result)
	ast.NotNil(result.Err)
	ast.Contains(result.Err.Error(), "无效的恢复步骤")
}

func TestResume_NegativeStepIdx(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_neg", &mockAuth{name: "mock_neg", result: StepPass})
	defer mockUnregister("mock_neg")

	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "mock_neg"}}}
	state := PipelineState{StepIdx: -1}
	ctx := &Context{}
	result := resumePipeline(ctx, profile, state)

	ast.NotNil(result)
	ast.Equal(StepFail, result.Result)
	ast.NotNil(result.Err)
	ast.Contains(result.Err.Error(), "无效的恢复步骤")
}

func TestResume_StepIdxZero_StartFromBeginning(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_r0", &mockAuth{name: "mock_r0", result: StepPass})
	defer mockUnregister("mock_r0")

	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "mock_r0"}, {Type: "mock_r0"}}}
	state := PipelineState{StepIdx: 0}
	ctx := &Context{}
	result := resumePipeline(ctx, profile, state)

	ast.NotNil(result)
	ast.Equal(StepPass, result.Result)
}

// ========== PipelineResult 结构 ==========

func TestPipelineResult_UsernamePropagation(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_up", &mockAuth{name: "mock_up", result: StepPass})
	defer mockUnregister("mock_up")

	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "mock_up"}}}
	ctx := &Context{Conn: ConnInfo{Username: "zhangsan", GroupName: "技术部"}}
	ctx.SetInfo("认证日志信息")
	result := runPipeline(ctx, profile)

	ast.NotNil(result)
	ast.Equal(StepPass, result.Result)
	ast.Equal("zhangsan", result.Username)
	ast.Equal("技术部", result.GroupName)
	ast.Equal("认证日志信息", result.Info)
}

// ========== 结果对象独立性 ==========

func TestExecute_ResultImmutable(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_imm", &mockAuth{name: "mock_imm", result: StepPass})
	defer mockUnregister("mock_imm")

	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "mock_imm"}}}
	ctx := &Context{}
	r1 := runPipeline(ctx, profile)
	r2 := runPipeline(ctx, profile)

	ast.NotSame(r1, r2, "两次执行应返回独立的结果对象")
}

// ========== StepFail 无 error 场景 ==========

func TestExecute_StepFailWithoutError(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_fail_noerr", &mockAuth{name: "mock_fail_noerr", result: StepFail, err: nil})
	defer mockUnregister("mock_fail_noerr")

	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "mock_fail_noerr"}}}
	ctx := &Context{}
	result := runPipeline(ctx, profile)

	ast.NotNil(result)
	ast.Equal(StepFail, result.Result)
	ast.Nil(result.Err, "StepFail 无 error 时 Err 应为 nil")
}

// 证书身份一致性检查

type certIdentityMock struct {
	name     string
	identity string
}

func (m *certIdentityMock) Name() string { return m.name }
func (m *certIdentityMock) Authenticate(ctx *Context) (StepResult, error) {
	ctx.Identity = m.identity
	return StepPass, nil
}

func TestExecute_IdentityConsistencyMatch(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_cert_ok", &certIdentityMock{name: "mock_cert_ok", identity: "alice"})
	defer mockUnregister("mock_cert_ok")

	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "mock_cert_ok"}}}
	ctx := &Context{Conn: ConnInfo{Username: "alice"}}
	result := runPipeline(ctx, profile)

	ast.Equal(StepPass, result.Result, "证书 CN 与登录用户名一致应通过")
	ast.Nil(result.Err)
}

func TestExecute_IdentityConsistencyMismatch(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_cert_bad", &certIdentityMock{name: "mock_cert_bad", identity: "alice"})
	defer mockUnregister("mock_cert_bad")

	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "mock_cert_bad"}}}
	ctx := &Context{Conn: ConnInfo{Username: "bob"}}
	result := runPipeline(ctx, profile)

	ast.Equal(StepFail, result.Result, "证书 CN 与登录用户名不一致应失败")
	ast.NotNil(result.Err)
	ast.Contains(result.Err.Error(), "证书身份")
	ast.Contains(result.Err.Error(), "不一致")
}

func TestExecute_IdentityConsistencyNoIdentitySkipped(t *testing.T) {
	ast := assert.New(t)
	mockRegister("mock_noid", &mockAuth{name: "mock_noid", result: StepPass})
	defer mockUnregister("mock_noid")

	// 无证书步骤（ctx.Identity 为空）→ 一致性检查应跳过，正常通过
	profile := GroupAuthProfile{Step: []AuthMethodConfig{{Type: "mock_noid"}}}
	ctx := &Context{Conn: ConnInfo{Username: "bob"}}
	result := runPipeline(ctx, profile)

	ast.Equal(StepPass, result.Result)
	ast.Nil(result.Err)
}
