package webvpn

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

//   - portal_session：精确 host（Domain=""，不跨子域）
//   - webvpn_session：通配 .base2（跨 WebVPN 子域共享）
//   - webvpn_grant  ：通配 .base2（一次性免登授权，子域可读取兑换）
// 改动这些规则会直接导致「门户需清 cookie 才能用 / 子域免登失效」等问题复发。

// 验证注册域最后两段的提取（IP / 单段 / 带端口 / 大小写）。
func TestBase2Domain(t *testing.T) {
	ast := assert.New(t)
	ast.Equal("example.com", base2Domain("example.com"))
	ast.Equal("example.com", base2Domain("wv.example.com"))
	ast.Equal("example.com", base2Domain("a.b.wv.example.com"))
	ast.Equal("example.com", base2Domain("Wv.Example.COM"))
	ast.Equal("example.com", base2Domain("wv.example.com:8443"))
	ast.Equal("example.co.uk", base2Domain("wv.example.co.uk"))
	// 单段主机（如 localhost）原样返回
	ast.Equal("localhost", base2Domain("localhost"))
	// 空输入
	ast.Equal("", base2Domain(""))
}

// 验证 WebVPN 会话 cookie 的域分派：
// 属于 WebVpnDomain 注册域 → 通配 .base2；IP / 未配置 / 不属于 → 精确 host（""）。
func TestCookieDomain(t *testing.T) {
	ast := assert.New(t)
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.WebVpnDomain = "wv.example.com"
	})

	// 子域、根域本身、带端口 → 通配 .example.com
	ast.Equal(".example.com", CookieDomain("app.wv.example.com"))
	ast.Equal(".example.com", CookieDomain("wv.example.com"))
	ast.Equal(".example.com", CookieDomain("APP.WV.EXAMPLE.COM:8443."))
	ast.Equal(".example.com", CookieDomain("app.wv.example.com:8443"))

	// 其他注册域 → 精确 host（""），避免跨域污染
	ast.Equal("", CookieDomain("portal.other.com"))
	ast.Equal("", CookieDomain("example.org"))

	// IP 访问 → 精确 host（""）
	ast.Equal("", CookieDomain("192.168.1.10"))
	ast.Equal("", CookieDomain("192.168.1.10:8443"))

	// 未配置 WebVpnDomain → 精确 host（""）
	base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = "" })
	ast.Equal("", CookieDomain("app.wv.example.com"))
}

// 验证一次性免登授权清除域：始终为 .base2（带前导点），未配置则为 ""。
// grant 写入（setGrantCookie）与清除（ClearGrantCookie）都依赖此函数，必须保持一致。
func TestWildcardDomain(t *testing.T) {
	ast := assert.New(t)
	base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = "wv.example.com" })
	ast.Equal(".example.com", wildcardDomain())

	base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = "" })
	ast.Equal("", wildcardDomain())
}

func TestPathAllowedBoundaries(t *testing.T) {
	ast := assert.New(t)
	ast.True(pathAllowed("/admin", "/admin"))
	ast.True(pathAllowed("/admin/users", "/admin/"))
	ast.False(pathAllowed("/administrator", "/admin"))
	ast.True(pathAllowed("/anything", "/"))
}

func TestParseWebVpnBackendURL(t *testing.T) {
	ast := assert.New(t)
	for _, raw := range []string{
		"http://10.0.0.8:8080/app",
		"https://[fd00::8]/",
	} {
		_, err := dbdata.ParseWebVpnBackendURL(raw)
		ast.NoError(err, raw)
	}
	for _, raw := range []string{
		"10.0.0.8:8080",
		"ftp://10.0.0.8/",
		"http://user:pass@10.0.0.8/",
		"http://10.0.0.8/?token=secret",
		"http://",
	} {
		_, err := dbdata.ParseWebVpnBackendURL(raw)
		ast.Error(err, raw)
	}
}
