package webvpn

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathAllowedRejectsDotSegment(t *testing.T) {
	ast := assert.New(t)

	// 穿越尝试：字符串前缀匹配可通过，但含 ".." / "." 段
	ast.False(pathAllowed("/api/../../admin", "/api"), "含 .. 的路径必须拒绝")
	ast.False(pathAllowed("/api/./admin", "/api"), "含 . 的路径必须拒绝")
	ast.False(pathAllowed("/../etc", "/"), "根前缀同样必须拒绝 ..")
	ast.False(pathAllowed("/..", "/api"), "越出根目录必须拒绝")

	// 正常路径不受影响（含尾部斜杠）
	ast.True(pathAllowed("/api", "/api"))
	ast.True(pathAllowed("/api/", "/api"), "尾部斜杠应放行")
	ast.True(pathAllowed("/api/users", "/api"))
	ast.True(pathAllowed("/anything/deep", "/"))

	// 未配置前缀 → 拒绝
	ast.False(pathAllowed("/api", ""))
}

func TestScrubBackendCORSHeaders(t *testing.T) {
	header := make(http.Header)
	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Access-Control-Allow-Credentials", "true")
	header.Set("Access-Control-Allow-Headers", "Authorization")
	header.Set("Access-Control-Allow-Methods", "GET")
	header.Set("Access-Control-Expose-Headers", "X-Internal")
	header.Set("Access-Control-Max-Age", "3600")
	header.Set("X-Backend", "kept")

	scrubBackendCORSHeaders(header)

	for _, name := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Credentials",
		"Access-Control-Allow-Headers",
		"Access-Control-Allow-Methods",
		"Access-Control-Expose-Headers",
		"Access-Control-Max-Age",
	} {
		assert.Empty(t, header.Values(name), name)
	}
	assert.Equal(t, "kept", header.Get("X-Backend"))
}

func TestStripRemLinkCookiesIsCaseInsensitive(t *testing.T) {
	cookies := []*http.Cookie{
		{Name: "WEBVPN_SESSION", Value: "spoofed"},
		{Name: "Portal_Session", Value: "spoofed"},
		{Name: "JSESSIONID", Value: "backend"},
	}

	assert.Equal(t, "JSESSIONID=backend", StripRemLinkCookies(cookies))
}

func TestScrubSetCookieDomain(t *testing.T) {
	resp := &http.Response{Header: make(http.Header)}
	resp.Header.Add("Set-Cookie", "webvpn_session=spoofed; Path=/")
	resp.Header.Add("Set-Cookie", "PORTAL_SESSION=spoofed; Path=/")
	resp.Header.Add("Set-Cookie", "backend=value; DOMAIN=.backend.example; Path=/; HttpOnly")
	resp.Header.Add("Set-Cookie", "other=value; Domain=public.example; Path=/")
	resp.Header.Add("Set-Cookie", "plain=value; Path=/")

	scrubSetCookieDomain(resp, "backend.example:443")

	assert.Equal(t, []string{
		"backend=value; Path=/; HttpOnly",
		"other=value; Domain=public.example; Path=/",
		"plain=value; Path=/",
	}, resp.Header.Values("Set-Cookie"))
}
