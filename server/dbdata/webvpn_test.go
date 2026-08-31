package dbdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
)

// 覆盖吊销阈值持久化后跨内存重置仍能使旧会话失效。
func TestWebVpnCorsAllowedOrigins(t *testing.T) {
	app := &WebVpnApp{AllowCrossSite: true, CorsAllowedOrigins: []string{"https://erp-dev.wg.maizuo.com"}}
	cases := []struct {
		origin string
		want   bool
	}{
		{"https://erp-dev.wg.maizuo.com", true},
		{"https://ERP-DEV.wG.MAIZUO.COM", true},
		{"https://erp-dev.wg.maizuo.com/evil", false},
		{"https://erp-dev.wg.maizuo.com.attacker.com", false},
		{"http://erp-dev.wg.maizuo.com", false},
		{"https://other.wg.maizuo.com", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, WebVpnCorsOriginAllowed(app, tc.origin), "Origin=%s", tc.origin)
	}
}

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

	WebVpnRevokeReset()

	// 内存已空，但仍应从 DB 回查到阈值（证明持久化生效）
	afterReset := WebVpnRevokeBeforeOf("alice")
	ast.Greater(afterReset, int64(0), "内存清空后应从 DB 读回吊销阈值")
	ast.Equal(beforeReset, afterReset, "DB 回查的阈值应与写入一致")

	// 其他用户不应受影响
	ast.Equal(int64(0), WebVpnRevokeBeforeOf("bob"), "未吊销用户阈值应保持 0")
}

// 覆盖数据库回查后的内存缓存行为。
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

// 覆盖批量吊销及其持久化结果。
func TestWebVpnRevokeUsersBatch(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()
	base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = "test" })
	defer base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = "" })

	WebVpnRevokeUsers([]string{"u1", "u2", "u3"})

	WebVpnRevokeReset() // 模拟重启

	for _, u := range []string{"u1", "u2", "u3"} {
		ast.Greater(WebVpnRevokeBeforeOf(u), int64(0), "批量吊销用户 %s 阈值应持久化", u)
	}
}

// 覆盖未吊销用户与其他用户记录的隔离。
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

// 覆盖应用名的字符限制。
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
