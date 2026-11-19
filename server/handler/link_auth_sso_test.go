package handler

import "testing"

// SSO 强制改密会话必须允许继续认证，否则会死锁：
// SSO 会话进入强制改密管道后被标记为 pipeline，其原 SSO token 仍需可兑换续跑管道；
// 普通管道会话（OTP/RADIUS/SMS 等待）必须被拒绝以防冒用
func TestIsSSOTokenSessionAllowed(t *testing.T) {
	cases := []struct {
		name     string
		kind     string
		forcePwd bool
		want     bool
	}{
		{"sso-done", AuthKindSSODone, false, true},
		{"sso-pending", AuthKindSSOPending, false, true},
		{"webauth-done", AuthKindWebAuth, false, true},
		{"pipeline+forcepwd(SSO强制改密)", AuthKindPipeline, true, true},
		{"pipeline 普通管道", AuthKindPipeline, false, false},
		{"未知类型", "unknown", false, false},
	}
	for _, c := range cases {
		s := &AuthSession{Kind: c.kind, ForcePwd: c.forcePwd}
		if got := isSSOTokenSessionAllowed(s); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
