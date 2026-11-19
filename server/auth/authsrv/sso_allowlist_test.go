package authsrv

import (
	"slices"
	"testing"

	"github.com/wsczx/remlink/auth"
)

// ssoStep 白名单/黑名单逻辑用例
func TestSsoStepUserAllowList(t *testing.T) {
	pass := func(userID string, allowedUsers, depts, blocked []string) (auth.StepResult, error) {
		ctx := &auth.Context{SSO: &auth.SSOState{Code: "code-1"}}
		return ssoStep(ctx, "测试",
			func(string) (string, string, error) { return userID, "测试用户", nil },
			func(_ string, depts []string) (bool, error) {
				// 模拟：u1 属于 d1 部门，其余用户不属于任何允许部门
				return userID == "u1" && slices.Contains(depts, "d1"), nil
			},
			func(_ string, blocked []string) error {
				if slices.Contains(blocked, userID) {
					return errBlockedTest
				}
				return nil
			},
			func() []string { return depts },
			func() []string { return allowedUsers },
			func() []string { return blocked },
		)
	}

	cases := []struct {
		name         string
		user         string
		allowedUsers []string
		depts        []string
		blocked      []string
		wantPass     bool
	}{
		{"用户白名单命中直接放行", "u1", []string{"u1", "u2"}, nil, nil, true},
		{"用户白名单命中跳过部门检查", "u1", []string{"u1"}, []string{"d9"}, nil, true},
		{"部门白名单命中", "u1", nil, []string{"d1"}, nil, true},
		{"名单与部门均未命中", "u9", []string{"u1"}, []string{"d1"}, nil, false},
		{"仅配置用户名单未命中", "u9", []string{"u1"}, nil, nil, false},
		{"两白名单均空不限制", "u9", nil, nil, nil, true},
		{"黑名单优先于用户白名单", "u1", []string{"u1"}, nil, []string{"u1"}, false},
		{"黑名单优先于部门白名单", "u1", nil, []string{"d1"}, []string{"u1"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := pass(c.user, c.allowedUsers, c.depts, c.blocked)
			if c.wantPass && err != nil {
				t.Fatalf("期望通过，实际失败: %v", err)
			}
			if !c.wantPass && err == nil {
				t.Fatalf("期望拒绝，实际通过: %v", result)
			}
		})
	}
}

var errBlockedTest = &authFailError{}

type authFailError struct{}

func (*authFailError) Error() string { return "在拒绝名单中" }
