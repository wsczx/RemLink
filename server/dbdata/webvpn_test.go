package dbdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
)

// TestWebVpnRevokePersistAcrossMemoryReset 验证 P1-8 修复：
// 踢出阈值持久化到 DB，即使进程内存被清空（模拟重启），仍能从 DB 读回吊销阈值，
// 使被踢用户的旧会话继续失效（原纯内存方案重启后即失效）。
func TestWebVpnRevokePersistAcrossMemoryReset(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	// 初始阈值应为 0（未吊销）
	ast.Equal(int64(0), WebVpnRevokeBeforeOf("alice"), "未吊销用户阈值应为 0")

	// 吊销 alice（写入 DB + 内存）
	WebVpnRevokeUser("alice")

	// 内存与 DB 都应能读到阈值
	beforeReset := WebVpnRevokeBeforeOf("alice")
	ast.Greater(beforeReset, int64(0), "吊销后阈值应大于 0")

	// 模拟重启：清空内存缓存
	WebVpnRevokeReset()

	// 内存已空，但仍应从 DB 回查到阈值（证明持久化生效）
	afterReset := WebVpnRevokeBeforeOf("alice")
	ast.Greater(afterReset, int64(0), "内存清空后应从 DB 读回吊销阈值")
	ast.Equal(beforeReset, afterReset, "DB 回查的阈值应与写入一致")

	// 其他用户不应受影响
	ast.Equal(int64(0), WebVpnRevokeBeforeOf("bob"), "未吊销用户阈值应保持 0")
}

// TestWebVpnRevokeBeforeOfCacheFill 验证回查 DB 命中后会回填内存缓存，
// 后续调用不再触碰 DB（通过连续调用返回一致值间接验证）。
func TestWebVpnRevokeBeforeOfCacheFill(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	WebVpnRevokeUser("carol")
	WebVpnRevokeReset() // 清内存，强制首次回查 DB

	first := WebVpnRevokeBeforeOf("carol")
	// 第二次命中内存缓存，返回值应与首次一致
	second := WebVpnRevokeBeforeOf("carol")
	ast.Equal(first, second, "回查后内存缓存应使后续读取返回一致值")
	ast.Greater(first, int64(0))
}

// TestWebVpnRevokeUsersBatch 验证批量吊销：
// 批量吊销后每个用户的阈值均被持久化，内存清空后仍可从 DB 读回。
func TestWebVpnRevokeUsersBatch(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()
	// 模拟 WebVPN 已启用（批量吊销在 WebVPN 未启用时直接跳过，不写库）
	base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = "test" })
	defer base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = "" })

	WebVpnRevokeUsers([]string{"u1", "u2", "u3"})

	WebVpnRevokeReset() // 模拟重启

	for _, u := range []string{"u1", "u2", "u3"} {
		ast.Greater(WebVpnRevokeBeforeOf(u), int64(0), "批量吊销用户 %s 阈值应持久化", u)
	}
}

// TestWebVpnRevokeUserRemoveUnset 验证未吊销用户始终返回 0，
// 即使 DB 中存在其他用户的吊销记录，也不应误读取。
func TestWebVpnRevokeUserRemoveUnset(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	WebVpnRevokeUser("active")
	WebVpnRevokeReset()

	// 从未被吊销的用户，内存与 DB 均无记录
	ast.Equal(int64(0), WebVpnRevokeBeforeOf("never-kicked"))
	ast.Equal(int64(0), WebVpnRevokeBeforeOf(""))
}

// TestWebVpnAppNameValid 验证应用名仅允许小写字母/数字/中划线，
// 防止作为子域前缀拼接出非预期主机或注入。
func TestWebVpnAppNameValid(t *testing.T) {
	ast := assert.New(t)
	ast.True(webVpnAppNameValid("app1"))
	ast.True(webVpnAppNameValid("my-app"))
	ast.True(webVpnAppNameValid("oa-2024"))
	ast.False(webVpnAppNameValid(""))
	ast.False(webVpnAppNameValid("App1"), "大写应拒绝")
	ast.False(webVpnAppNameValid("my_app"), "下划线应拒绝")
	ast.False(webVpnAppNameValid("a.b"), "点号应拒绝")
	ast.False(webVpnAppNameValid("a b"), "空格应拒绝")
}
