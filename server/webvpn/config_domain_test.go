package webvpn

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

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

// grant 写入与清除共用此域规则
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

func TestParseBackendURL(t *testing.T) {
	ast := assert.New(t)
	for _, raw := range []string{
		"http://10.0.0.8:8080/app",
		"https://[fd00::8]/",
	} {
		_, err := dbdata.ParseBackendURL(raw)
		ast.NoError(err, raw)
	}
	for _, raw := range []string{
		"10.0.0.8:8080",
		"ftp://10.0.0.8/",
		"http://user:pass@10.0.0.8/",
		"http://10.0.0.8/?token=secret",
		"http://",
	} {
		_, err := dbdata.ParseBackendURL(raw)
		ast.Error(err, raw)
	}
}
