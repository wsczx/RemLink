package dbdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
)

// 覆盖吊销阈值持久化后跨内存重置仍能使旧会话失效
func TestWebVpnCorsAllowedOrigins(t *testing.T) {
	app := &WebVpnApp{AllowCrossSite: true, CorsAllowedOrigins: []string{"https://app2.wv.example.com"}}
	cases := []struct {
		origin string
		want   bool
	}{
		{"https://app2.wv.example.com", true},
		{"https://APP2.wv.example.com", true},
		{"https://app2.wv.example.com/evil", false},
		{"https://app2.wv.example.com.attacker.com", false},
		{"http://app2.wv.example.com", false},
		{"https://other.wv.example.com", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, WebVpn.CorsOriginAllowed(app, tc.origin), "Origin=%s", tc.origin)
	}
}

func TestWebVpnRevokePersistAcrossMemoryReset(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	ast.Equal(int64(0), WebVpn.RevokeBeforeOf("alice"), "未吊销用户阈值应为 0")
	WebVpn.RevokeUser("alice")

	beforeReset := WebVpn.RevokeBeforeOf("alice")
	ast.Greater(beforeReset, int64(0), "吊销后阈值应大于 0")
	WebVpn.RevokeReset()

	afterReset := WebVpn.RevokeBeforeOf("alice")
	ast.Greater(afterReset, int64(0), "内存清空后应从 DB 读回吊销阈值")
	ast.Equal(beforeReset, afterReset, "DB 回查的阈值应与写入一致")

	ast.Equal(int64(0), WebVpn.RevokeBeforeOf("bob"), "未吊销用户阈值应保持 0")
}

// 覆盖数据库回查后的内存缓存行为
func TestWebVpnRevokeBeforeOfCacheFill(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()
	WebVpn.RevokeUser("carol")
	WebVpn.RevokeReset()

	first := WebVpn.RevokeBeforeOf("carol")

	second := WebVpn.RevokeBeforeOf("carol")
	ast.Equal(first, second, "回查后内存缓存应使后续读取返回一致值")
	ast.Greater(first, int64(0))
}

// 覆盖批量吊销及其持久化结果
func TestWebVpnRevokeUsersBatch(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()
	base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = "test" })
	defer base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = "" })
	WebVpn.RevokeUsers([]string{"u1", "u2", "u3"})
	WebVpn.RevokeReset()

	for _, u := range []string{"u1", "u2", "u3"} {
		ast.Greater(WebVpn.RevokeBeforeOf(u), int64(0), "批量吊销用户 %s 阈值应持久化", u)
	}
}

// 覆盖未吊销用户与其他用户记录的隔离
func TestWebVpnRevokeUserRemoveUnset(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()
	WebVpn.RevokeUser("active")
	WebVpn.RevokeReset()

	ast.Equal(int64(0), WebVpn.RevokeBeforeOf("never-kicked"))
	ast.Equal(int64(0), WebVpn.RevokeBeforeOf(""))
}

// 覆盖应用名的字符限制
func TestWebVpnAppNameValid(t *testing.T) {
	ast := assert.New(t)
	ast.True((&WebVpnApp{Name: "app1"}).validName())
	ast.True((&WebVpnApp{Name: "my-app"}).validName())
	ast.True((&WebVpnApp{Name: "oa-2024"}).validName())
	ast.False((&WebVpnApp{Name: ""}).validName())
	ast.False((&WebVpnApp{Name: "App1"}).validName(), "大写应拒绝")
	ast.False((&WebVpnApp{Name: "my_app"}).validName(), "下划线应拒绝")
	ast.False((&WebVpnApp{Name: "a.b"}).validName(), "点号应拒绝")
	ast.False((&WebVpnApp{Name: "a b"}).validName(), "空格应拒绝")
}
