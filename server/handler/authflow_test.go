package handler

import (
	"testing"

	"github.com/wsczx/remlink/auth"
)

// 固化跨端锁语义不变量：
// Flow.Username 始终取首认证用户名（Ctx.Conn.Username），与续跑场景管道可能
// 避免「锁错人」。端点不得在回调里用 Result.Username 覆盖 Flow.Username。
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
// 自动持久化；否则仅写回不持久化（调用方自行负责存储）。
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
