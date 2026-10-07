package webvpn

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/dbdata"
)

func newTestRewriter() *PathRewriter {
	return NewPathRewriter("/web/http/oa", "vpn.example.com", true, "10.0.0.5:8080", "oa.internal")
}

func TestParseWebPath(t *testing.T) {
	cases := []struct {
		in               string
		proto, app, rest string
		ok               bool
	}{
		{"/web/http/oa/", "http", "oa", "/", true},
		{"/web/http/oa", "http", "oa", "", true},
		{"/web/ssh/test/", "ssh", "test", "/", true},
		{"/web/rdp/win10/webvpn/app-info", "rdp", "win10", "/webvpn/app-info", true},
		{"/web/http/oa/a/b?x", "http", "oa", "/a/b?x", true},
		{"/web/HTTP/oa/", "", "", "", false},
		{"/web/telnet/oa/", "", "", "", false},
		{"/web/http/OA/", "", "", "", false},
		{"/web/http//", "", "", "", false},
		{"/web/http/o.a/", "", "", "", false},
		{"/webvpn/app-info", "", "", "", false},
		{"/web/", "", "", "", false},
	}
	for _, c := range cases {
		proto, app, rest, ok := ParseWebPath(c.in)
		assert.Equal(t, c.ok, ok, c.in)
		if c.ok {
			assert.Equal(t, c.proto, proto, c.in)
			assert.Equal(t, c.app, app, c.in)
			assert.Equal(t, c.rest, rest, c.in)
		}
	}
	assert.True(t, WebPathHasDotSegment("/api/../admin"))
	assert.True(t, WebPathHasDotSegment("/./x"))
	assert.False(t, WebPathHasDotSegment("/a..b/c"))
	assert.Equal(t, "/web/ssh/test", WebPathPrefix("ssh", "test"))
}

func TestFromProxiedPage(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "https://vpn.example.com/api/v1/portal/logout", nil)
	assert.False(t, FromProxiedPage(r))
	r.Header.Set("Referer", "https://vpn.example.com/ui/")
	assert.False(t, FromProxiedPage(r))
	r.Header.Set("Referer", "https://vpn.example.com/web/http/oa/index.html")
	assert.True(t, FromProxiedPage(r))
	r.Header.Set("Referer", "https://vpn.example.com/web/ssh/test/")
	assert.False(t, FromProxiedPage(r))
	r.Header.Set("Referer", "https://other.example.com/web/http/oa/")
	assert.False(t, FromProxiedPage(r))
}

func TestPathRewriterURL(t *testing.T) {
	p := newTestRewriter()
	cases := map[string]string{
		"/":                              "/web/http/oa/",
		"/static/app.js?v=1#x":           "/web/http/oa/static/app.js?v=1#x",
		"/web/http/oa/already":           "/web/http/oa/already",
		"relative/x.png":                 "relative/x.png",
		"../up.png":                      "../up.png",
		"#anchor":                        "#anchor",
		"data:image/png;base64,AAA=":     "data:image/png;base64,AAA=",
		"javascript:void(0)":             "javascript:void(0)",
		"mailto:a@b.c":                   "mailto:a@b.c",
		"https://cdn.example.org/x.js":   "https://cdn.example.org/x.js",
		"//cdn.example.org/x.js":         "//cdn.example.org/x.js",
		"http://10.0.0.5:8080/login":     "/web/http/oa/login",
		"http://oa.internal/a?b=1":       "/web/http/oa/a?b=1",
		"//10.0.0.5:8080/x":              "/web/http/oa/x",
		"https://vpn.example.com/api/v1": "/web/http/oa/api/v1",
		"  /spaced ":                     "/web/http/oa/spaced",
	}
	for in, want := range cases {
		assert.Equal(t, want, p.URL(in), in)
	}
}

func TestPathRewriterLocation(t *testing.T) {
	p := newTestRewriter()
	cases := map[string]string{
		"/login":                        "/web/http/oa/login",
		"http://10.0.0.5:8080/home?a=1": "https://vpn.example.com/web/http/oa/home?a=1",
		"https://oa.internal/":          "https://vpn.example.com/web/http/oa/",
		"login.html":                    "login.html",
		"https://sso.example.org/auth":  "https://sso.example.org/auth",
		// 指向 VPN 主域名但在前缀之外：收敛到前缀内，防止借重定向跳出 CSP 路径限制
		"https://vpn.example.com/api/v1/portal/logout": "https://vpn.example.com/web/http/oa/api/v1/portal/logout",
		"//vpn.example.com/ui/":                        "https://vpn.example.com/web/http/oa/ui/",
		// 反斜杠 / 控制字符变体
		"/\\vpn.example.com/ui/":  "https://vpn.example.com/web/http/oa/ui/",
		"/\t/10.0.0.5:8080/x":     "https://vpn.example.com/web/http/oa/x",
		"/web/http/oa/keep":       "/web/http/oa/keep",
		"ws://10.0.0.5:8080/sock": "wss://vpn.example.com/web/http/oa/sock",
	}
	for in, want := range cases {
		assert.Equal(t, want, p.Location(in), in)
	}
	assert.Equal(t, "0; url=/web/http/oa/next", p.Refresh("0; url=/next"))
	assert.Equal(t, "5;URL='/web/http/oa/x'", p.Refresh("5;URL='/x'"))
	assert.Equal(t, "5", p.Refresh("5"))
}

func TestPathRewriterSetCookie(t *testing.T) {
	p := newTestRewriter()
	c, ok := p.SetCookie("sid=abc; Path=/; Domain=10.0.0.5; HttpOnly; SameSite=Lax")
	require.True(t, ok)
	assert.Equal(t, "sid=abc; Path=/web/http/oa/; HttpOnly; SameSite=Lax", c)

	c, ok = p.SetCookie("pref=1; path=/app; Secure")
	require.True(t, ok)
	assert.Equal(t, "pref=1; Path=/web/http/oa/app; Secure", c)

	c, ok = p.SetCookie("x=1")
	require.True(t, ok)
	assert.Equal(t, "x=1", c, "未指定 Path 时默认路径本就位于前缀之下")

	c, ok = p.SetCookie("y=1; Path=relative")
	require.True(t, ok)
	assert.Equal(t, "y=1; Path=/web/http/oa/", c)

	for _, name := range []string{"portal_session", "webvpn_session", "WEBVPN_GRANT", "auth-session-id", "acSamlv2Token"} {
		_, ok = p.SetCookie(name + "=evil; Path=/")
		assert.False(t, ok, "RemLink 自有 cookie 名 %s 必须被丢弃", name)
	}
	_, ok = p.SetCookie("=novalue")
	assert.False(t, ok)
}

func TestPathRewriterCSS(t *testing.T) {
	p := newTestRewriter()
	in := `body{background:url(/img/bg.png)} .a{background:url( "/img/a.png" )} .b{background:url('rel.png')}
@import "/css/base.css"; @import '/css/x.css'; .d{background:url(data:image/png;base64,AAA=)} .e{src:url(http://10.0.0.5:8080/f.woff)}`
	out := string(p.CSS([]byte(in)))
	assert.Contains(t, out, `url(/web/http/oa/img/bg.png)`)
	assert.Contains(t, out, `url("/web/http/oa/img/a.png")`)
	assert.Contains(t, out, `url('rel.png')`)
	assert.Contains(t, out, `@import "/web/http/oa/css/base.css"`)
	assert.Contains(t, out, `@import '/web/http/oa/css/x.css'`)
	assert.Contains(t, out, `url(data:image/png;base64,AAA=)`)
	assert.Contains(t, out, `url(/web/http/oa/f.woff)`)
}

func rewriteHTMLString(t *testing.T, p *PathRewriter, in string) string {
	var b bytes.Buffer
	require.NoError(t, p.RewriteHTML(&b, strings.NewReader(in)))
	return b.String()
}

func TestPathRewriterHTML(t *testing.T) {
	p := newTestRewriter()
	in := `<!DOCTYPE html>
<html><head>
<base href="/">
<meta http-equiv="refresh" content="30; url=/timeout">
<link rel="stylesheet" href="/css/app.css" integrity="sha384-xxx" crossorigin>
<style>.x{background:url(/i.png)}</style>
<script src="/js/app.js"></script>
<script>fetch('/api/data')</script>
</head><body class="Keep  Spacing">
<a href="/page?a=1&amp;b=2">p</a> <a href="rel">r</a> <a href="https://example.org/">ext</a>
<img src="/img/a.png" srcset="/img/a.png 1x, /img/a@2x.png 2x">
<form action="/submit" method="post"><button formaction="/alt">go</button></form>
<object data="/obj.swf"></object><video poster="/p.jpg"></video>
<div style="background:url('/bg.png')">t</div>
<svg><use xlink:href="/sprite.svg#i"/></svg>
<textarea><a href="/not-a-tag"></textarea>
</body></html>`
	out := rewriteHTMLString(t, p, in)
	for _, want := range []string{
		`<base href="/web/http/oa/">`,
		`content="30; url=/web/http/oa/timeout"`,
		`href="/web/http/oa/css/app.css"`,
		`<style>.x{background:url(/web/http/oa/i.png)}</style>`,
		`<script src="/web/http/oa/js/app.js"></script>`,
		`<script>fetch('/api/data')</script>`, // 脚本不改写
		`<body class="Keep  Spacing">`,        // 未改动的标签原样输出
		`href="/web/http/oa/page?a=1&amp;b=2"`,
		`<a href="rel">r</a>`,
		`<a href="https://example.org/">ext</a>`,
		`src="/web/http/oa/img/a.png"`,
		`srcset="/web/http/oa/img/a.png 1x, /web/http/oa/img/a@2x.png 2x"`,
		`action="/web/http/oa/submit"`,
		`formaction="/web/http/oa/alt"`,
		`data="/web/http/oa/obj.swf"`,
		`poster="/web/http/oa/p.jpg"`,
		`style="background:url(&#39;/web/http/oa/bg.png&#39;)"`,
		`xlink:href="/web/http/oa/sprite.svg#i"`,
		`<textarea><a href="/not-a-tag"></textarea>`,
	} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "integrity", "改写过的样式表须去掉 SRI")
	assert.Contains(t, out, "crossorigin")

	// 幂等：再改写一次不应重复加前缀
	assert.Equal(t, out, rewriteHTMLString(t, p, out))
	// 截断的尾部数据不丢失
	assert.Equal(t, `<p>x</p><a hre`, rewriteHTMLString(t, p, `<p>x</p><a hre`))
}

// 端到端：经路径模式反代后，路径前缀被去掉、gzip HTML 被解压改写、响应头被改写并加同源缓解头
func TestPathReverseProxyEndToEnd(t *testing.T) {
	var seenPath, seenRef, seenAE, seenPrefix, seenCookie string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath, seenRef, seenAE = r.URL.RequestURI(), r.Header.Get("Referer"), r.Header.Get("Accept-Encoding")
		seenPrefix, seenCookie = r.Header.Get("X-Forwarded-Prefix"), r.Header.Get("Cookie")
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "http://"+r.Host+"/home", http.StatusFound)
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Security-Policy", "default-src *")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Service-Worker-Allowed", "/")
			w.Header().Set("Clear-Site-Data", `"cookies"`)
			w.Header().Add("Set-Cookie", "sid=1; Path=/; Domain="+strings.Split(r.Host, ":")[0])
			w.Header().Add("Set-Cookie", "portal_session=evil; Path=/")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			var b bytes.Buffer
			gz := gzip.NewWriter(&b)
			gz.Write([]byte(`<a href="/x">x</a><img src="/i.png">`))
			gz.Close()
			w.Header().Set("Content-Length", "999") // 错误长度：改写后必须移除
			w.Write(b.Bytes())
		case "/bin":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("/raw/bytes"))
		default:
			w.Write([]byte("ok"))
		}
	}))
	defer backend.Close()

	app := &dbdata.WebVpnApp{Name: "oa", Backend: backend.URL, Status: 1}
	proxy, err := NewPathReverseProxy(app, "vpn.example.com", true)
	require.NoError(t, err)

	do := func(path string) *http.Response {
		r := httptest.NewRequest(http.MethodGet, "https://vpn.example.com"+path, nil)
		r.Header.Set("Accept-Encoding", "gzip, deflate, br")
		r.Header.Set("Referer", "https://vpn.example.com/web/http/oa/prev?q=1")
		r.AddCookie(&http.Cookie{Name: "portal_session", Value: "secret"})
		r.AddCookie(&http.Cookie{Name: "sid", Value: "1"})
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, r)
		return rec.Result()
	}

	resp := do("/web/http/oa/page?a=1")
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "/page?a=1", seenPath, "转发前应去掉 /web/http/<应用> 前缀")
	assert.Equal(t, "https://vpn.example.com/prev?q=1", seenRef)
	assert.Equal(t, "gzip", seenAE, "只协商可解码的 gzip")
	assert.Equal(t, "/web/http/oa", seenPrefix)
	assert.NotContains(t, seenCookie, "portal_session", "门户会话不得转发给后端")
	assert.Contains(t, seenCookie, "sid=1")
	assert.Equal(t, `<a href="/web/http/oa/x">x</a><img src="/web/http/oa/i.png">`, string(body))
	assert.Empty(t, resp.Header.Get("Content-Encoding"))
	assert.Empty(t, resp.Header.Get("Content-Length"))
	assert.Empty(t, resp.Header.Get("X-Frame-Options"))
	assert.Empty(t, resp.Header.Get("Service-Worker-Allowed"))
	assert.Empty(t, resp.Header.Get("Clear-Site-Data"))
	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
	csp := resp.Header.Get("Content-Security-Policy")
	assert.Contains(t, csp, "connect-src https://vpn.example.com/web/http/oa/ wss://vpn.example.com/web/http/oa/")
	assert.Contains(t, csp, "form-action https://vpn.example.com/web/http/oa/")
	assert.Contains(t, csp, "object-src 'none'")
	assert.NotContains(t, csp, "'self';") // 连接不允许整站同源
	assert.Equal(t, "same-origin-allow-popups", resp.Header.Get("Cross-Origin-Opener-Policy"))
	assert.Equal(t, []string{"sid=1; Path=/web/http/oa/"}, resp.Header.Values("Set-Cookie"))

	resp = do("/web/http/oa/redirect")
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "https://vpn.example.com/web/http/oa/home", resp.Header.Get("Location"))

	resp = do("/web/http/oa/bin")
	body, _ = io.ReadAll(resp.Body)
	assert.Equal(t, "/raw/bytes", string(body), "非 HTML/CSS 不改写")

	resp = do("/web/http/oa/")
	assert.Equal(t, "/", seenPath)
}
