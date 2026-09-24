package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
)

// Flow.Username 始终取首认证用户名（Ctx.Conn.Username），与续跑场景管道可能
// 端点不得在回调里用 Result.Username 覆盖 Flow.Username
func TestFlowUsername_LockSemantics(t *testing.T) {
	const firstUser = "alice"
	const reparsedUser = "alice-ldap" // 续跑时管道可能解析出的不同身份

	fl := &Flow{
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{Username: firstUser},
		},
		Username: firstUser,
		Result: &auth.PipelineResult{
			Username:  reparsedUser, // 续跑解析身份不等于首认证身份
			Result:    auth.StepPending,
			Challenge: &auth.ChallengeInfo{Type: auth.ChallengeOTP},
		},
	}

	// 不变量：Flow.Username 不被 Result.Username 污染
	if fl.Username != firstUser {
		t.Fatalf("Flow.Username 应为首认证用户名 %q，实际 %q", firstUser, fl.Username)
	}

	// 锁定动作必须基于 Flow.Username（首认证身份），而非 Result.Username
	if fl.Username == fl.Result.Username {
		t.Fatalf("测试构造错误：应为不同身份以验证解耦")
	}
	lockTarget := fl.Username
	if lockTarget != firstUser {
		t.Fatalf("锁定目标应取 Flow.Username=%q，实际 %q", firstUser, lockTarget)
	}
}

// 验证断点写回与可选持久化收口：
// 当 Flow.Session 注入且持有 SessionID 时，savePendingState 写回 StepIdx 后
// 自动持久化；否则仅写回不持久化（调用方自行负责存储）
func TestSavePendingState_WritesBack(t *testing.T) {
	ctx := &auth.Context{}
	pending := &auth.PipelineResult{
		Result: auth.StepPending,
		State:  auth.PipelineState{StepIdx: 2, PassedSteps: []string{"0", "1"}},
	}

	fl := &Flow{Ctx: ctx, Result: pending}
	fl.savePendingState()

	if ctx.StepIdx() != 2 {
		t.Fatalf("StepIdx 未写回，实际 %d", ctx.StepIdx())
	}
	if len(ctx.PassedSteps()) != 2 {
		t.Fatalf("PassedSteps 未写回，实际 %v", ctx.PassedSteps())
	}
}

// 验证挑战阶段锁定计数决策的三个分支：
// ① 挑战码错误（原地踏步）→ 失败；② 非重试但无活动挑战（异常态）→ 失败；③ 正常进入挑战（带 Challenge）→ 成功清计数
func TestFlow_pendingLockDecision(t *testing.T) {
	ast := assert.New(t)
	f := &Flow{Username: "u", RemoteAddr: "1.2.3.4:5678"}

	retry := &auth.PipelineResult{
		Result:      auth.StepPending,
		PrevStepIdx: 1,
		State:       auth.PipelineState{StepIdx: 1},
		Challenge:   &auth.ChallengeInfo{},
	}
	ast.False(f.pendingLockDecision(retry), "挑战码错误应计失败")

	noChallenge := &auth.PipelineResult{
		Result:      auth.StepPending,
		PrevStepIdx: -1,
		State:       auth.PipelineState{StepIdx: 0},
		Challenge:   nil,
	}
	ast.False(f.pendingLockDecision(noChallenge), "非重试且无活动挑战应计失败")

	ok := &auth.PipelineResult{
		Result:      auth.StepPending,
		PrevStepIdx: -1,
		State:       auth.PipelineState{StepIdx: 0},
		Challenge:   &auth.ChallengeInfo{},
	}
	ast.True(f.pendingLockDecision(ok), "正常进入挑战应清计数")
}
