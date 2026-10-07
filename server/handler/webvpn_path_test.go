package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/admin"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn/gateway"
)

// 路径模式测试应用：在 setupWebVpnTest 的基础上追加
func setupWebPathTest(t *testing.T) (*httptest.Server, func()) {
	backend, teardown := setupWebVpnTest(t)
	base.UpdateCfg(func(c *base.ServerConfig) { c.EnableUserPortal = true })
	be := backend.URL
	require.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "pathapp", Backend: be, Status: 1, Users: []string{"alice"}, AccessMode: dbdata.WebVpnAccessPath,
	}))
	require.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "bothapp", Backend: be, Status: 1, Users: []string{"alice"}, AccessMode: dbdata.WebVpnAccessBoth,
	}))
	require.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "pathapi", Backend: be, Status: 1, Users: []string{"alice"}, AccessMode: dbdata.WebVpnAccessPath,
		AllowPath: []string{"/api/"},
	}))
	require.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "sshp", Protocol: dbdata.WebVpnProtoSSH, TargetHost: "10.0.0.9", Status: 1, Users: []string{"alice"},
		AccessMode: dbdata.WebVpnAccessPath,
	}))
	return backend, teardown
}

func portalToken(t *testing.T, user string) string {
	tok, err := admin.SetJwtData(map[string]any{"portal_user": user, "portal_type": "local"}, time.Now().Add(time.Hour).Unix())
	require.NoError(t, err)
	return tok
}

// 构造主域名上的请求；user 为空表示未登录
func newWebPathReq(t *testing.T, method, target, user string) (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, "https://vpn.example.com"+target, nil)
	req.Host = "vpn.example.com"
	req.RemoteAddr = "203.0.113.9:54321"
	req.Header.Set("X-Forwarded-Proto", "https")
	if user != "" {
		req.AddCookie(&http.Cookie{Name: portalCookieName, Value: portalToken(t, user)})
	}
	return req, httptest.NewRecorder()
}

func TestWebPathUnauthenticatedRedirect(t *testing.T) {
	_, teardown := setupWebPathTest(t)
	defer teardown()

	req, rec := newWebPathReq(t, http.MethodGet, "/web/http/pathapp/a/b?x=1", "")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/portal?redirect="+url.QueryEscape("/web/http/pathapp/a/b?x=1"), rec.Header().Get("Location"))

	req, rec = newWebPathReq(t, http.MethodPost, "/web/http/pathapp/a", "")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "非导航请求未登录应返回 401")

	// 远程协议同样跳门户登录
	req, rec = newWebPathReq(t, http.MethodGet, "/web/ssh/sshp/", "")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, "/portal?redirect="+url.QueryEscape("/web/ssh/sshp/"), rec.Header().Get("Location"))
}

func TestWebPathProxyUsesPortalSession(t *testing.T) {
	_, teardown := setupWebPathTest(t)
	defer teardown()

	for _, app := range []string{"pathapp", "bothapp"} {
		req, rec := newWebPathReq(t, http.MethodGet, "/web/http/"+app+"/inner/page?q=1", "alice")
		require.True(t, WebPathHandler(rec, req))
		assert.Equal(t, http.StatusOK, rec.Code, app)
		assert.Equal(t, "OK backend", rec.Body.String())
		backendSeen.Lock()
		got := backendSeen.last
		backendSeen.Unlock()
		require.NotNil(t, got)
		assert.Equal(t, "/inner/page", got.URL.Path, "转发时去掉路径前缀")
		assert.Equal(t, "q=1", got.URL.RawQuery)
		assert.NotContains(t, got.Header.Get("Cookie"), "portal_session", "门户会话不得转发给后端")
		assert.Equal(t, "/web/http/"+app, got.Header.Get("X-Forwarded-Prefix"))
		assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "connect-src https://vpn.example.com/web/http/"+app+"/")
	}

	// 后端 302 到自身绝对地址：改写到前缀下
	req, rec := newWebPathReq(t, http.MethodGet, "/web/http/pathapp/redirect", "alice")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://vpn.example.com/web/http/pathapp/internal", rec.Header().Get("Location"))
}

func TestWebPathAccessModeEnforced(t *testing.T) {
	_, teardown := setupWebPathTest(t)
	defer teardown()

	// 仅子域名的应用不能走路径
	req, rec := newWebPathReq(t, http.MethodGet, "/web/http/app1/", "alice")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// 协议段与应用协议不一致
	req, rec = newWebPathReq(t, http.MethodGet, "/web/ssh/pathapp/", "alice")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// 仅路径的应用不能走子域名
	sreq, srec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "pathapp"})
	webVpnProxy(srec, sreq, "pathapp")
	assert.Equal(t, http.StatusNotFound, srec.Code)
	// both 两者皆可
	sreq, srec, _ = newWebVpnReqEx(t, webVpnReqOpts{host: "bothapp"})
	webVpnProxy(srec, sreq, "bothapp")
	assert.Equal(t, http.StatusOK, srec.Code)

	// 不存在的应用 / 未授权用户
	req, rec = newWebPathReq(t, http.MethodGet, "/web/http/nope/", "alice")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	req, rec = newWebPathReq(t, http.MethodGet, "/web/http/pathapp/", "bob")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestWebPathAllowPathOnSubpath(t *testing.T) {
	_, teardown := setupWebPathTest(t)
	defer teardown()

	req, rec := newWebPathReq(t, http.MethodGet, "/web/http/pathapi/api/list", "alice")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusOK, rec.Code, "路径白名单作用于前缀之后的子路径")

	req, rec = newWebPathReq(t, http.MethodGet, "/web/http/pathapi/admin", "alice")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// 点段绕过白名单
	req, rec = newWebPathReq(t, http.MethodGet, "/web/http/pathapi/api/../admin", "alice")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestWebPathRemoteEntryAndRouting(t *testing.T) {
	_, teardown := setupWebPathTest(t)
	defer teardown()

	req, rec := newWebPathReq(t, http.MethodGet, "/web/ssh/sshp", "alice")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "/web/ssh/sshp/", rec.Header().Get("Location"))

	req, rec = newWebPathReq(t, http.MethodGet, "/web/ssh/sshp/", "alice")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/ui/#/webvpn/remote?app=sshp&mode=path&protocol=ssh", rec.Header().Get("Location"))

	req, rec = newWebPathReq(t, http.MethodGet, "/web/ssh/sshp/other", "alice")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// 远程协议接口交给网关路由
	req, rec = newWebPathReq(t, http.MethodGet, "/web/ssh/sshp/webvpn/app-info", "alice")
	assert.False(t, WebPathHandler(rec, req))

	// 子域名上的 /web/* 属于被代理应用自身路径
	req, rec = newWebPathReq(t, http.MethodGet, "/web/http/pathapp/", "alice")
	req.Host = "app1.wv.example.com"
	assert.False(t, WebPathHandler(rec, req))
	// 非 /web/ 路径不处理
	req, rec = newWebPathReq(t, http.MethodGet, "/webvpn/me", "alice")
	assert.False(t, WebPathHandler(rec, req))

	// 门户登录回跳带上的 webvpn_grant 被清理
	req, rec = newWebPathReq(t, http.MethodGet, "/web/http/pathapp/?webvpn_grant=abc&k=v", "alice")
	require.True(t, WebPathHandler(rec, req))
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/web/http/pathapp/?k=v", rec.Header().Get("Location"))
}

// 远程协议路径版接口：用户来自门户会话，应用来自路径，ws_path 带路径前缀
func TestWebPathGatewayAPI(t *testing.T) {
	_, teardown := setupWebPathTest(t)
	defer teardown()

	r := mux.NewRouter()
	gateway.SetPathUserResolver(portalCurrentUser)
	gateway.RegisterRoutes(r)

	call := func(target, user string, mod func(*http.Request)) (*httptest.ResponseRecorder, map[string]any) {
		req, rec := newWebPathReq(t, http.MethodGet, target, user)
		if mod != nil {
			mod(req)
		}
		r.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec, out
	}

	rec, out := call("/web/ssh/sshp/webvpn/app-info", "alice", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	data := out["data"].(map[string]any)
	assert.Equal(t, "sshp", data["name"])
	assert.Equal(t, "/web/ssh/sshp/webvpn/ws/term", data["ws_path"])

	rec, _ = call("/web/ssh/sshp/webvpn/app-info", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	rec, _ = call("/web/ssh/sshp/webvpn/app-info", "bob", nil)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	rec, _ = call("/web/sftp/sshp/webvpn/app-info", "alice", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code, "协议段不一致")
	rec, _ = call("/web/ssh/sshp/webvpn/app-info", "alice", func(r *http.Request) {
		r.Header.Set("Referer", "https://vpn.example.com/web/http/pathapp/x")
	})
	assert.Equal(t, http.StatusForbidden, rec.Code, "被代理 HTTP 页面不得调用远程协议接口")
	rec, _ = call("/web/ssh/sshp/webvpn/app-info", "alice", func(r *http.Request) {
		r.Header.Set("Sec-Fetch-Site", "cross-site")
	})
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// 写接口要求严格同源
	req, wrec := newWebPathReq(t, http.MethodPost, "/web/ssh/sshp/webvpn/sessions", "alice")
	req.Body = http.NoBody
	r.ServeHTTP(wrec, req)
	assert.Equal(t, http.StatusForbidden, wrec.Code, "缺少 Origin 的写请求应拒绝")

	// 仅路径的应用不能经子域名接口访问
	sreq, srec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "sshp", path: "/webvpn/app-info"})
	r.ServeHTTP(srec, sreq)
	assert.Equal(t, http.StatusNotFound, srec.Code)
}

func TestPortalCSRFMiddleware(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := portalCSRFMiddleware(ok)
	mk := func(method, path string, hdr map[string]string) int {
		req := httptest.NewRequest(method, "https://vpn.example.com"+path, nil)
		req.Host = "vpn.example.com"
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	p := "/api/v1/portal/change_password"
	assert.Equal(t, http.StatusNoContent, mk(http.MethodGet, "/api/v1/portal/me", nil), "GET 不校验")
	assert.Equal(t, http.StatusNoContent, mk(http.MethodPost, "/webvpn/logout", nil), "非门户接口不校验")
	assert.Equal(t, http.StatusForbidden, mk(http.MethodPost, p, nil), "缺少自定义头")
	assert.Equal(t, http.StatusNoContent, mk(http.MethodPost, p, map[string]string{portalCSRFHeader: "1"}))
	assert.Equal(t, http.StatusNoContent, mk(http.MethodPost, p, map[string]string{
		portalCSRFHeader: "1", "Origin": "https://vpn.example.com", "Sec-Fetch-Site": "same-origin",
		"Referer": "https://vpn.example.com/ui/",
	}))
	assert.Equal(t, http.StatusForbidden, mk(http.MethodPost, p, map[string]string{portalCSRFHeader: "1", "Origin": "https://evil.example.org"}))
	assert.Equal(t, http.StatusForbidden, mk(http.MethodPost, p, map[string]string{portalCSRFHeader: "1", "Sec-Fetch-Site": "same-site"}))
	assert.Equal(t, http.StatusForbidden, mk(http.MethodPost, p, map[string]string{
		portalCSRFHeader: "1", "Referer": "https://vpn.example.com/web/http/oa/index.html",
	}), "路径模式被代理页面不得调用门户写接口")
}

func TestWebVpnMyAppsAccessURLs(t *testing.T) {
	_, teardown := setupWebPathTest(t)
	defer teardown()

	list := func() map[string]map[string]any {
		req, rec := newWebPathReq(t, http.MethodGet, "/webvpn/my-apps", "alice")
		webVpnMyApps(rec, req)
		var out struct {
			Code int              `json:"code"`
			Data []map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		m := map[string]map[string]any{}
		for _, a := range out.Data {
			m[a["name"].(string)] = a
		}
		return m
	}
	apps := list()
	assert.Equal(t, "https://app1.wv.example.com", apps["app1"]["subdomain_url"])
	assert.Equal(t, "", apps["app1"]["path_url"])
	assert.Equal(t, "https://app1.wv.example.com", apps["app1"]["url"])
	assert.Equal(t, "", apps["pathapp"]["subdomain_url"])
	assert.Equal(t, "/web/http/pathapp/", apps["pathapp"]["path_url"])
	assert.Equal(t, "/web/http/pathapp/", apps["pathapp"]["url"])
	assert.Equal(t, "https://bothapp.wv.example.com", apps["bothapp"]["url"], "两者皆可时优先子域名")
	assert.Equal(t, "/web/http/bothapp/", apps["bothapp"]["path_url"])
	assert.Equal(t, "/web/ssh/sshp/", apps["sshp"]["url"])

	// 未配置域名：子域名 HTTP 应用不可用（不出现在列表），远程协议回落到路径
	base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = "" })
	defer base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = "wv.example.com" })
	apps = list()
	_, has := apps["app1"]
	assert.False(t, has)
	assert.Equal(t, "/web/http/bothapp/", apps["bothapp"]["url"])
	assert.True(t, strings.HasPrefix(apps["sshp"]["url"].(string), "/web/ssh/"))
}

func TestWebPathSafeRedirect(t *testing.T) {
	assert.True(t, webPathSafeRedirect("https://vpn.example.com/web/ssh/test/", "vpn.example.com"))
	assert.True(t, webPathSafeRedirect("https://VPN.example.com/web/http/oa/x?y=1", "vpn.example.com"))
	assert.False(t, webPathSafeRedirect("http://vpn.example.com/web/ssh/test/", "vpn.example.com"))
	assert.False(t, webPathSafeRedirect("https://evil.example.org/web/ssh/test/", "vpn.example.com"))
	assert.False(t, webPathSafeRedirect("https://vpn.example.com/ui/", "vpn.example.com"))
	assert.False(t, webPathSafeRedirect("https://u@vpn.example.com/web/ssh/test/", "vpn.example.com"))
	assert.False(t, webPathSafeRedirect("https://vpn.example.com/web/http/oa/../../x", "vpn.example.com"))
}
