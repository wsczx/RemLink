package sessdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
)

// TestGroupNatMasqueradeEnabled 锁住全局 NAT 开关对「组自定义网段」的约束力。
// 这是修复「关闭全局 NAT 后组自定义网段仍被下发 MASQUERADE」的核心契约：
// 关闭 GlobalNat -> v4 组网段不伪装；关闭 GlobalNat6 -> v6 组网段不伪装。
func TestGroupNatMasqueradeEnabled(t *testing.T) {
	ast := assert.New(t)

	set := func(v4, v6 bool) {
		base.UpdateCfg(func(c *base.ServerConfig) {
			c.GlobalNat = v4
			c.GlobalNat6 = v6
		})
	}

	set(true, true)
	ast.True(GroupNatMasqueradeEnabled(false), "GlobalNat 开时 v4 组应伪装")
	ast.True(GroupNatMasqueradeEnabled(true), "GlobalNat6 开时 v6 组应伪装")

	set(false, true)
	ast.False(GroupNatMasqueradeEnabled(false), "GlobalNat 关时 v4 组不应伪装（纯路由）")
	ast.True(GroupNatMasqueradeEnabled(true), "GlobalNat6 开时 v6 组仍应伪装")

	set(true, false)
	ast.True(GroupNatMasqueradeEnabled(false), "GlobalNat 开时 v4 组仍应伪装")
	ast.False(GroupNatMasqueradeEnabled(true), "GlobalNat6 关时 v6 组不应伪装（纯路由）")

	set(false, false)
	ast.False(GroupNatMasqueradeEnabled(false), "GlobalNat 关时 v4 组不应伪装")
	ast.False(GroupNatMasqueradeEnabled(true), "GlobalNat6 关时 v6 组不应伪装")
}
