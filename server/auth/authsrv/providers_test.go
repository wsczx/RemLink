package authsrv

import (
	"testing"

	"github.com/wsczx/remlink/auth"
)

// 重复调用 RegisterProviders 不得 panic 或重复注册：
// 无论被调用一次还是多次（app.Start 显式装配或测试 setup），重复注册必须被跳过
// 不能触发 "auth: 重复注册" panic
func TestRegisterProvidersIdempotent(t *testing.T) {
	r := auth.NewProviderRegistry()
	RegisterProviders(r) // 空注册表，覆盖真实注册路径
	for _, p := range providers {
		if !r.IsRegistered(p.Name) {
			t.Fatalf("provider %q 未被注册", p.Name)
		}
	}
	RegisterProviders(r) // 重复调用不应 panic 或改变注册表
}
