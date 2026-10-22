package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/dbtest"
	"github.com/wsczx/remlink/pkg/jwt"
	"github.com/wsczx/remlink/webvpn"
)

func TestWebVpnSameOriginRejectsUnsafeOrigins(t *testing.T) {
	cases := []struct {
		name   string
		origin string
		secure bool
		want   bool
	}{
		{name: "path", origin: "https://app.wv.example.com/evil", secure: true},
		{name: "user-info", origin: "https://attacker@app.wv.example.com", secure: true},
		{name: "cross-protocol", origin: "http://app.wv.example.com", secure: true},
		{name: "cross-host", origin: "https://other.wv.example.com", secure: true},
		{name: "same-origin", origin: "https://app.wv.example.com", secure: true, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "https://app.wv.example.com/", nil)
			r.Host = "app.wv.example.com"
			r.Header.Set("Origin", tc.origin)
			if !tc.secure {
				r.Header.Set("X-Forwarded-Proto", "http")
			} else {
				r.Header.Set("X-Forwarded-Proto", "https")
			}
			assert.Equal(t, tc.want, webVpnSameOrigin(r))
		})
	}
}

func TestWebVpnPreflightRejectsUnsafeOrigins(t *testing.T) {
	for _, origin := range []string{"https://app.wv.example.com/evil", "http://app.wv.example.com", "https://other.wv.example.com"} {
		r := httptest.NewRequest(http.MethodOptions, "https://app.wv.example.com/", nil)
		r.Host = "app.wv.example.com"
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		webVpnHandlePreflight(w, r, &dbdata.WebVpnApp{})
		assert.Equal(t, http.StatusForbidden, w.Code, "Origin=%s", origin)
	}
}

func TestWebVpnPreflightAllowsConfiguredOrigin(t *testing.T) {
	r := httptest.NewRequest(http.MethodOptions, "https://dev-erp-backend.wg.maizuo.com/activity/api", nil)
	r.Host = "dev-erp-backend.wg.maizuo.com"
	r.Header.Set("Origin", "https://erp-dev.wg.maizuo.com")
	r.Header.Set("Access-Control-Request-Method", http.MethodGet)
	r.Header.Set("Access-Control-Request-Headers", "sess-auth-token, content-type")
	w := httptest.NewRecorder()
	webVpnHandlePreflight(w, r, &dbdata.WebVpnApp{AllowCrossSite: true, CorsAllowedOrigins: []string{"https://erp-dev.wg.maizuo.com"}})
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "https://erp-dev.wg.maizuo.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "sess-auth-token, content-type", w.Header().Get("Access-Control-Allow-Headers"))
}

func TestWebVpnWithCORSRejectsUnsafeOrigins(t *testing.T) {
	for _, origin := range []string{"https://app.wv.example.com/evil", "http://app.wv.example.com", "https://other.wv.example.com"} {
		r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
		r.Host = "app.wv.example.com"
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Forwarded-Proto", "https")
		resp := &http.Response{Header: make(http.Header)}
		callback := withCORS(nil, r, &dbdata.WebVpnApp{})
		if callback != nil {
			require.NoError(t, callback(resp))
		}
		assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"), "Origin=%s", origin)
	}
}

var backendSeen struct {
	sync.Mutex
	last *http.Request
}

func setupWebVpnTest(t *testing.T) (backend *httptest.Server, teardown func()) {
	dbtest.SetupDB(t)

	backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendSeen.Lock()
		backendSeen.last = r
		backendSeen.Unlock()

		switch {
		case r.URL.Path == "/redirect":
			http.Redirect(w, r, "http://"+r.Host+"/internal", http.StatusFound)
		case r.URL.Path == "/csp":
			w.Header().Set("Content-Security-Policy", "default-src 'self'")
			w.Header().Set("X-Backend", "hit")
			w.Write([]byte("csp-page"))
		case strings.HasPrefix(r.URL.Path, "/slow"):
			w.WriteHeader(200)
			w.Write([]byte("chunk"))
			time.Sleep(50 * time.Millisecond)
		default:
			w.Header().Set("X-Backend", "hit")
			w.Write([]byte("OK backend"))
		}
	}))

	beURL, _ := url.Parse(backend.URL)

	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "app1", Backend: beURL.String(), Status: 1, Users: []string{"alice"},
	}))
	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "app2", Backend: beURL.String(), Status: 1, Users: []string{"bob"},
	}))
	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "appg", Backend: beURL.String(), Status: 1, Users: []string{"alice"}, Groups: []string{"ops"},
	}))
	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "appip", Backend: beURL.String(), Status: 1, Users: []string{"alice"},
		IpAllowList: []string{"10.0.0.0/8"},
	}))
	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "apppath", Backend: beURL.String(), Status: 1, Users: []string{"alice"},
		AllowPath: []string{"/api/"},
	}))
	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "apphost", Backend: beURL.String(), Status: 1, Users: []string{"alice"},
		HostRewrite: "backend.internal",
	}))
	appx := &dbdata.WebVpnApp{Name: "appx", Backend: beURL.String(), Users: []string{"alice"}}
	assert.NoError(t, dbdata.SetWebVpnApp(appx))
	got, err := dbdata.GetWebVpnAppByName("appx")
	assert.NoError(t, err)
	got.Status = 0
	assert.NoError(t, dbdata.SetWebVpnApp(got))

	alice := &dbdata.User{Username: "alice", Type: "local", Status: 1}
	assert.NoError(t, dbdata.Add(alice))
	bob := &dbdata.User{Username: "bob", Type: "local", Status: 1}
	assert.NoError(t, dbdata.Add(bob))

	dbdata.WebVpnRevokeReset()
	webvpn.GetManager().Session().ResetCache()

	webvpn.GetManager().Audit().Start()
	teardown = func() {
		webvpn.GetManager().Audit().Stop()
		// 失效 AppStore 缓存：否则后续用例反代连到已关闭端口（502）
		webvpn.GetManager().Apps().Invalidate()
		backend.Close()
	}
	return backend, teardown
}

func newWebVpnReq(t *testing.T, _ *httptest.Server, user, path string) (*http.Request, *httptest.ResponseRecorder) {
	token, err := jwt.SetJwtData(map[string]any{
		"webvpn_user":   user,
		"webvpn_type":   "local",
		"webvpn_groups": []string{},
	}, time.Now().Add(3*time.Hour).Unix())
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://app1.wv.example.com"+path, nil)
	req.Host = "app1.wv.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "1.2.3.4") // 伪造，应被清洗
	req.Header.Set("Cookie", "portal_session=fake; theme=dark")
	req.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: token})
	req.RemoteAddr = "203.0.113.9:54321"

	rec := httptest.NewRecorder()
	return req, rec
}

type webVpnReqOpts struct {
	host      string   // 子域名，默认 app1
	user      string   // webvpn_user，默认 alice
	groups    []string // webvpn_groups，默认空
	path      string   // 请求路径，默认 /
	clientIP  string   // 的 IP 部分，默认 203.0.113.9
	portal    string   // 附加的 portal_session cookie 值（用于兑换测试）
	iatOffset int64    // 签发 iat 相对现在的偏移（秒），默认 0
	expOffset int64    // 过期时间相对现在偏移（秒），默认 +3h
	noSession bool     // 不带 webvpn_session cookie
}

func newWebVpnReqEx(t *testing.T, opts webVpnReqOpts) (*http.Request, *httptest.ResponseRecorder, string) {
	if opts.host == "" {
		opts.host = "app1"
	}
	if opts.user == "" {
		opts.user = "alice"
	}
	if opts.path == "" {
		opts.path = "/"
	}
	if opts.clientIP == "" {
		opts.clientIP = "203.0.113.9"
	}
	if opts.expOffset == 0 {
		opts.expOffset = int64((3 * time.Hour).Seconds())
	}

	var rec *httptest.ResponseRecorder
	var req *http.Request
	token := ""
	if !opts.noSession {
		iat := time.Now().Unix() + opts.iatOffset
		exp := time.Now().Unix() + opts.expOffset
		token = issueWebVpnTokenForTest(t, opts.user, opts.groups, iat, exp)
		req = httptest.NewRequest(http.MethodGet, "https://"+opts.host+".wv.example.com"+opts.path, nil)
		req.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: token})
	} else {
		req = httptest.NewRequest(http.MethodGet, "https://"+opts.host+".wv.example.com"+opts.path, nil)
	}
	req.Host = opts.host + ".wv.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "1.2.3.4") // 伪造，应被清洗
	req.Header.Set("X-Real-IP", "9.9.9.9")       // 伪造，应被清洗
	req.AddCookie(&http.Cookie{Name: "theme", Value: "dark"})
	if opts.portal != "" {
		req.AddCookie(&http.Cookie{Name: "portal_session", Value: opts.portal})
	} else {
		req.AddCookie(&http.Cookie{Name: "portal_session", Value: "fake"})
	}
	req.RemoteAddr = opts.clientIP + ":54321"
	rec = httptest.NewRecorder()
	return req, rec, token
}

func issueWebVpnTokenForTest(t *testing.T, user string, groups []string, iat, exp int64) string {
	return issueWebVpnTokenWithIssued(t, user, groups, iat, exp, 0)
}

// 在 issueWebVpnTokenForTest 基础上可额外控制 webvpn_issued 锚点（0=不写，旧 token 行为）
func issueWebVpnTokenWithIssued(t *testing.T, user string, groups []string, iat, exp, issued int64) string {
	data := map[string]any{
		"webvpn_user":   user,
		"webvpn_type":   "local",
		"webvpn_groups": groups,
		"iat":           iat,
		"exp":           exp,
	}
	if issued > 0 {
		data["webvpn_issued"] = issued
	}
	tok, err := jwt.SetJwtData(data, exp)
	assert.NoError(t, err)
	return tok
}

func TestWebVpnProxyRewrite(t *testing.T) {
	backend, teardown := setupWebVpnTest(t)
	defer teardown()
	beHost := strings.TrimPrefix(backend.URL, "http://")

	req, rec := newWebVpnReq(t, backend, "alice", "/")
	webVpnProxy(rec, req, "app1")

	backendSeen.Lock()
	got := backendSeen.last
	backendSeen.Unlock()
	assert.NotNil(t, got, "后端应收到请求")
	assert.Equal(t, beHost, got.Host, "Host 应改写为后端地址")
	assert.Equal(t, "203.0.113.9", got.Header.Get("X-Forwarded-For"), "XFF 应重写为真实客户端 IP")
	assert.Equal(t, "app1.wv.example.com", got.Header.Get("X-Forwarded-Host"))
	// 自有 portal_session 不泄漏给后端，应用自身 cookie 必须原样转发
	assert.NotContains(t, got.Header.Get("Cookie"), "portal_session", "portal_session 不应泄漏给后端")
	assert.Contains(t, got.Header.Get("Cookie"), "theme=dark", "应用自身 cookie 应转发给后端")
	assert.Equal(t, "1", got.Header.Get("X-RemLink-WebVpn"))
}

func TestWebVpnProxyLocationRewrite(t *testing.T) {
	backend, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec := newWebVpnReq(t, backend, "alice", "/redirect")
	webVpnProxy(rec, req, "app1")

	loc := rec.Result().Header.Get("Location")
	assert.True(t, strings.Contains(loc, "app1.wv.example.com"),
		"302 Location 应改写回子域名，实际: %s", loc)
	assert.False(t, strings.Contains(loc, "127.0.0.1"),
		"Location 不应残留后端地址，实际: %s", loc)
}

func TestWebVpnHostMatchesBackend(t *testing.T) {
	ast := assert.New(t)
	ast.True(webvpn.HostMatchesBackend("10.0.0.5:8080", "10.0.0.5:8080"))
	ast.True(webvpn.HostMatchesBackend("backend.internal", "backend.internal"))
	ast.True(webvpn.HostMatchesBackend("app.backend.internal", "backend.internal"))
	ast.True(webvpn.HostMatchesBackend("api.10.0.0.5", "10.0.0.5"))
	ast.False(webvpn.HostMatchesBackend("badexample.com.evil.org", "example.com"))
	ast.False(webvpn.HostMatchesBackend("notbackend.internal", "backend.internal"))
	ast.False(webvpn.HostMatchesBackend("10.0.0.50", "10.0.0.5"))
	ast.False(webvpn.HostMatchesBackend("", "10.0.0.5"))
	ast.False(webvpn.HostMatchesBackend("10.0.0.5", ""))
}

func TestWebVpnProxyStripSecurityHeaders(t *testing.T) {
	backend, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec := newWebVpnReq(t, backend, "alice", "/csp")
	webVpnProxy(rec, req, "app1")

	assert.Empty(t, rec.Result().Header.Get("Content-Security-Policy"),
		"被代理响应不应继承全局 CSP")
}

func TestWebVpnProxyUnauthorized(t *testing.T) {
	backend, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec := newWebVpnReq(t, backend, "alice", "/")
	req.Host = "app2.wv.example.com"
	webVpnProxy(rec, req, "app2")
	assert.Equal(t, http.StatusForbidden, rec.Code, "未授权用户应 403")
}

func TestWebVpnProxyDisabledApp(t *testing.T) {
	backend, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec := newWebVpnReq(t, backend, "alice", "/")
	req.Host = "appx.wv.example.com"
	webVpnProxy(rec, req, "appx")
	assert.Equal(t, http.StatusForbidden, rec.Code, "禁用应用应拒绝")
}

func TestWebVpnProxyUnauthenticated(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	req := httptest.NewRequest(http.MethodGet, "https://app1.wv.example.com/", nil)
	req.Host = "app1.wv.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	webVpnProxy(rec, req, "app1")

	assert.Equal(t, http.StatusFound, rec.Code, "未登录应 302 到登录")
	loc := rec.Result().Header.Get("Location")
	assert.True(t, strings.Contains(loc, "/portal"), "应跳转门户登录，实际: %s", loc)
}

func TestWebVpnProxyBackendDown(t *testing.T) {
	backend, teardown := setupWebVpnTest(t)
	backend.Close() // 后端立即不可用

	req, rec := newWebVpnReq(t, backend, "alice", "/")
	webVpnProxy(rec, req, "app1")
	assert.Equal(t, http.StatusBadGateway, rec.Code, "后端不可达应 502")
	assert.Contains(t, rec.Body.String(), "后端", "应返回友好错误页")
	teardown()
}

func TestWebVpnProxySkipVerify(t *testing.T) {
	backend, teardown := setupWebVpnTest(t)
	defer teardown()

	tlsBackend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("tls-ok"))
	}))
	defer tlsBackend.Close()

	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "tlsnoskip", Backend: tlsBackend.URL, Status: 1, Users: []string{"alice"},
	}))
	req, rec := newWebVpnReq(t, backend, "alice", "/")
	req.Host = "tlsnoskip.wv.example.com"
	webVpnProxy(rec, req, "tlsnoskip")
	assert.Equal(t, http.StatusBadGateway, rec.Code, "自签证书且未开启跳过校验应 502")

	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "tlsskip", Backend: tlsBackend.URL, Status: 1, Users: []string{"alice"}, SkipVerify: true,
	}))
	req2, rec2 := newWebVpnReq(t, backend, "alice", "/")
	req2.Host = "tlsskip.wv.example.com"
	webVpnProxy(rec2, req2, "tlsskip")
	assert.Equal(t, http.StatusOK, rec2.Code, "开启跳过校验后自签后端应可访问")
	assert.Equal(t, "tls-ok", rec2.Body.String())
}

func TestWebVpnRevokeAllForUser(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	token, err := jwt.SetJwtData(map[string]any{
		"webvpn_user": "alice", "webvpn_type": "local", "webvpn_groups": []string{},
	}, time.Now().Add(3*time.Hour).Unix())
	assert.NoError(t, err)
	u, ok := webvpn.GetManager().Session().UserFromToken(token)
	assert.True(t, ok)
	assert.Equal(t, "alice", u.Username)

	webvpn.GetManager().Revoker().RevokeUser("alice")
	_, ok = webvpn.GetManager().Session().UserFromToken(token)
	assert.False(t, ok, "踢出后旧会话应立即失效")
}

// 客户端伪造的 X-Forwarded-For / X-Real-IP 必须被丢弃，后端只收到真实 RemoteAddr
func TestWebVpnProxyStripClientIPSpoofing(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{})
	webVpnProxy(rec, req, "app1")

	backendSeen.Lock()
	got := backendSeen.last
	backendSeen.Unlock()
	if !assert.NotNil(t, got, "应成功反代到后端") {
		return
	}
	assert.Equal(t, "203.0.113.9", got.Header.Get("X-Forwarded-For"), "XFF 应重写为真实客户端 IP")
	assert.Empty(t, got.Header.Get("X-Real-IP"), "伪造的 X-Real-IP 必须被剥离")
	assert.NotContains(t, got.Header.Get("X-Forwarded-For"), "1.2.3.4", "伪造 XFF 不应泄漏")
}

func TestWebVpnProxyStripAllSecurityHeaders(t *testing.T) {
	backend, teardown := setupWebVpnTest(t)
	defer teardown()

	backend.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendSeen.Lock()
		backendSeen.last = r
		backendSeen.Unlock()
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Write([]byte("secure-page"))
	})

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", path: "/"})
	webVpnProxy(rec, req, "app1")

	h := rec.Result().Header
	assert.Empty(t, h.Get("Content-Security-Policy"), "CSP 应被剥离")
	assert.Empty(t, h.Get("Cross-Origin-Embedder-Policy"), "COEP 应被剥离")
	assert.Empty(t, h.Get("Cross-Origin-Resource-Policy"), "CORP 应被剥离")
	assert.Empty(t, h.Get("X-Frame-Options"), "X-Frame-Options 应被剥离")
}

func TestWebVpnProxyGroupAuthorization(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "appg", groups: []string{"dev"}})
	webVpnProxy(rec, req, "appg")
	assert.Equal(t, http.StatusForbidden, rec.Code, "无授权组的用户应 403")

	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "appg", groups: []string{"ops"}})
	webVpnProxy(rec2, req2, "appg")
	assert.Equal(t, http.StatusOK, rec2.Code, "命中授权组的用户应放行")
}

func TestWebVpnProxyIpAllowList(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "appip", clientIP: "203.0.113.9"})
	webVpnProxy(rec, req, "appip")
	assert.Equal(t, http.StatusForbidden, rec.Code, "不在 IP 白名单应 403")

	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "appip", clientIP: "10.1.2.3"})
	webVpnProxy(rec2, req2, "appip")
	assert.Equal(t, http.StatusOK, rec2.Code, "命中 IP 白名单应放行")
}

func TestWebVpnProxyPathAllowList(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "apppath", path: "/admin"})
	webVpnProxy(rec, req, "apppath")
	assert.Equal(t, http.StatusForbidden, rec.Code, "越权路径应 403")

	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "apppath", path: "/api/users"})
	webVpnProxy(rec2, req2, "apppath")
	assert.Equal(t, http.StatusOK, rec2.Code, "命中路径白名单应放行")
}

// 签名过早仅验证放行，续期由 WebVpnHandler 外层完成
func TestWebVpnProxySlidingRenewal(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{iatOffset: -2 * 3600})
	webVpnProxy(rec, req, "app1")
	assert.Equal(t, http.StatusOK, rec.Code, "滑动续期：旧但有效的会话应放行")
}

func TestWebVpnProxyHostRewrite(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "apphost"})
	webVpnProxy(rec, req, "apphost")

	backendSeen.Lock()
	got := backendSeen.last
	backendSeen.Unlock()
	if !assert.NotNil(t, got, "应成功反代到后端") {
		return
	}
	assert.Equal(t, "backend.internal", got.Host, "Host 应改写为 HostRewrite 配置值")
}

func TestWebVpnProxyAppNotFound(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "ghost"})
	webVpnProxy(rec, req, "ghost")
	assert.Equal(t, http.StatusNotFound, rec.Code, "未配置的应用应 404")
}

// 门户登录下发的 webvpn_grant 可在子域兑换正式会话
func TestWebVpnExchangeFromPortal(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	portalTok, err := jwt.SetJwtData(map[string]any{
		"portal_user": "alice",
	}, time.Now().Add(time.Hour).Unix())
	assert.NoError(t, err)
	portalJTI, err := jwt.JtiOf(portalTok)
	assert.NoError(t, err)
	grantTok, err := webvpn.GetManager().Session().IssueGrant(
		nil, nil, &dbdata.User{Username: "alice", Type: "local", Status: 1}, portalJTI)
	assert.NoError(t, err)

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true, portal: portalTok})
	req.AddCookie(&http.Cookie{Name: "webvpn_grant", Value: grantTok})

	consumed := WebVpnHandler(rec, req)
	assert.True(t, consumed, "WebVPN 子域请求应被本处理器消费")
	assert.Equal(t, http.StatusOK, rec.Code, "持 webvpn_grant 应自动兑换并放行")
	var gotWebVpn bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == webVpnSessionCookie {
			gotWebVpn = true
			assert.Equal(t, "example.com", c.Domain, "会话 cookie 应与门户共用父域 Domain（前导点被标准库归一化）")
		}
	}
	assert.True(t, gotWebVpn, "应种回 webvpn_session cookie")
}

func TestWebVpnExchangeFromQueryRedirectsToCleanURL(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	grantTok, err := webvpn.GetManager().Session().IssueGrant(
		nil, nil, &dbdata.User{Username: "alice", Type: "local", Status: 1}, "portal-jti")
	assert.NoError(t, err)

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req.URL.RawQuery = url.Values{webvpnGrantQuery: []string{grantTok}}.Encode()
	assert.True(t, WebVpnHandler(rec, req))
	assert.Equal(t, http.StatusFound, rec.Code, "URL grant 兑换后应清理地址栏参数")
	location := rec.Header().Get("Location")
	assert.NotContains(t, location, webvpnGrantQuery, "清理后的地址不应继续携带 grant")

	cleanReq, cleanRec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == webVpnSessionCookie {
			cleanReq.AddCookie(cookie)
		}
	}
	assert.True(t, WebVpnHandler(cleanRec, cleanReq))
	assert.Equal(t, http.StatusOK, cleanRec.Code, "清理 URL 后应凭正式会话直接放行")
}

func TestWebVpnInvalidQueryGrantRedirectsToCleanURL(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	portalTok, err := jwt.SetJwtData(map[string]any{"portal_user": "alice"}, time.Now().Add(time.Hour).Unix())
	assert.NoError(t, err)

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true, portal: portalTok})
	req.URL.RawQuery = url.Values{webvpnGrantQuery: []string{"invalid-grant"}, "keep": []string{"1"}}.Encode()
	assert.True(t, WebVpnHandler(rec, req))
	assert.Equal(t, http.StatusFound, rec.Code, "无效 URL grant 也应先清理地址栏")
	location := rec.Header().Get("Location")
	assert.NotContains(t, location, webvpnGrantQuery, "无效 grant 不应继续出现在跳转地址")
	assert.Contains(t, location, "keep=1", "清理 grant 时应保留其它查询参数")
}

func TestWebVpnGrantIsReusable(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	portalTok, err := jwt.SetJwtData(map[string]any{"portal_user": "alice"}, time.Now().Add(time.Hour).Unix())
	assert.NoError(t, err)
	portalJTI, err := jwt.JtiOf(portalTok)
	assert.NoError(t, err)
	grantTok, err := webvpn.GetManager().Session().IssueGrant(
		nil, nil, &dbdata.User{Username: "alice", Type: "local", Status: 1}, portalJTI)
	assert.NoError(t, err)

	req1, rec1, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true, portal: portalTok})
	req1.AddCookie(&http.Cookie{Name: "webvpn_grant", Value: grantTok})
	assert.True(t, WebVpnHandler(rec1, req1), "首次兑换应消费请求")
	assert.Equal(t, http.StatusOK, rec1.Code, "首次兑换应放行")

	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req2.AddCookie(&http.Cookie{Name: "webvpn_grant", Value: grantTok})
	assert.True(t, WebVpnHandler(rec2, req2), "二次兑换请求仍应被本处理器消费")
	assert.NotEqual(t, http.StatusOK, rec2.Code, "同一 grant 二次兑换不得再次放行")

	_, err = jwt.GetJwtData(portalTok)
	assert.NoError(t, err, "兑换 grant 不应吊销门户会话 jti，门户登录态仍有效")
}

func TestWebVpnGrantExpires(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	portalJTI := "portal-jti-expired"
	expiredTok, err := jwt.SetJwtData(map[string]any{
		"webvpn_grant_user": "alice",
		"webvpn_grant_type": "local",
		"webvpn_grant_jti":  portalJTI,
	}, time.Now().Add(-time.Hour).Unix())
	assert.NoError(t, err)

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req.AddCookie(&http.Cookie{Name: "webvpn_grant", Value: expiredTok})
	WebVpnHandler(rec, req)
	assert.NotEqual(t, http.StatusOK, rec.Code, "过期 grant 兑换必须失败")
}

func TestWebVpnExchangeKeepsPortalSession(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	portalTok, err := jwt.SetJwtData(map[string]any{"username": "bob"}, time.Now().Add(time.Hour).Unix())
	assert.NoError(t, err)
	portalJTI, err := jwt.JtiOf(portalTok)
	assert.NoError(t, err)

	grantTok, err := webvpn.GetManager().Session().IssueGrant(
		nil, nil, &dbdata.User{Username: "alice", Type: "local", Status: 1}, portalJTI)
	assert.NoError(t, err)

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true, portal: portalTok})
	req.AddCookie(&http.Cookie{Name: "webvpn_grant", Value: grantTok})
	WebVpnHandler(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	_, err = jwt.GetJwtData(portalTok)
	assert.NoError(t, err, "门户会话 jti 不应被 WebVPN 兑换流程吊销")
}

func TestWebVpnLogoutRevokesSession(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec, token := newWebVpnReqEx(t, webVpnReqOpts{host: "app1"})
	webVpnProxy(rec, req, "app1")
	assert.Equal(t, http.StatusOK, rec.Code)

	logoutReq := httptest.NewRequest(http.MethodPost, "/webvpn/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: token})
	logoutRec := httptest.NewRecorder()
	webVpnLogout(logoutRec, logoutReq)

	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req2.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: token})
	webVpnProxy(rec2, req2, "app1")
	assert.NotEqual(t, http.StatusOK, rec2.Code, "登出后原会话应立即失效")
}

// 门户登出联动吊销该用户 WebVPN 会话，避免子域残留旧会话
func TestPortalLogoutRevokesWebVpnSession(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
	})

	_, _, token := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", user: "alice"})
	assert.NotEmpty(t, token)

	portalTok, err := jwt.SetJwtData(map[string]any{
		"portal_user": "alice",
		"portal_type": "local",
	}, time.Now().Add(time.Hour).Unix())
	assert.NoError(t, err)
	logoutReq := httptest.NewRequest(http.MethodPost, "/portal/api/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: portalCookieName, Value: portalTok})
	logoutReq.Header.Set("X-Forwarded-Proto", "https")
	logoutRec := httptest.NewRecorder()
	PortalLogout(logoutRec, logoutReq)
	assert.Equal(t, http.StatusOK, logoutRec.Code, "门户登出应成功")

	mgr := webvpn.GetManager()
	_, ok := mgr.Session().UserFromToken(token)
	assert.False(t, ok, "门户登出后该用户的 WebVPN 会话应被联动吊销")

	bobTok, err := jwt.SetJwtData(map[string]any{
		"webvpn_user":   "bob",
		"webvpn_type":   "local",
		"webvpn_groups": []string{},
		"iat":           time.Now().Unix(),
		"exp":           time.Now().Add(time.Hour).Unix(),
	}, time.Now().Add(time.Hour).Unix())
	assert.NoError(t, err)
	_, bobOk := mgr.Session().UserFromToken(bobTok)
	assert.True(t, bobOk, "其他用户的 WebVPN 会话不应被误伤")
}

// 兑换失败必须直接渲染 403，不得 302 回登录页（否则前端回跳+后端再跳=刷新死循环）
func TestWebVpnPortalLoggedInAutoExchange(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	portalTok, err := jwt.SetJwtData(map[string]any{
		"portal_user": "alice",
		"portal_type": "local",
	}, time.Now().Add(time.Hour).Unix())
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://app1.wv.example.com/", nil)
	req.Host = "app1.wv.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: portalCookieName, Value: portalTok})
	rec := httptest.NewRecorder()

	handled := WebVpnHandler(rec, req)
	assert.True(t, handled, "WebVpnHandler 应处理该请求")
	assert.Equal(t, http.StatusOK, rec.Code, "门户已登录用户应通过免登兑换直接进入，而非跳登录页")
	assert.Empty(t, rec.Header().Get("Location"), "不应重定向到登录页")
	cookies := rec.Result().Cookies()
	foundSession := false
	for _, c := range cookies {
		if c.Name == webVpnSessionCookie {
			foundSession = true
			break
		}
	}
	assert.True(t, foundSession, "免登兑换成功后应下发 WebVPN 会话 cookie")
}

func TestWebVpnProxyAuditLogged(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", clientIP: "203.0.113.9"})
	webVpnProxy(rec, req, "app1")
	assert.Equal(t, http.StatusOK, rec.Code)

	assert.Eventually(t, func() bool {
		list, _, err := dbdata.WebVpnAuditList(10, 1, dbdata.WebVpnAuditSearch{Username: "alice", AppName: "app1"})
		if err != nil {
			return false
		}
		for _, a := range list {
			if a.ClientIP == "203.0.113.9" && a.StatusCode == 200 {
				return true
			}
		}
		return false
	}, 3*time.Second, 100*time.Millisecond, "审计记录应落库且含真实客户端 IP")
}

// P1-8：整用户踢出阈值持久化到 DB，重启后旧 token 仍失效
func TestWebVpnRevokedPersistAcrossRestart(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	tok := issueWebVpnTokenForTest(t, "alice", []string{"alice-group"},
		time.Now().Add(-time.Minute).Unix(), time.Now().Add(2*time.Hour).Unix())

	u, ok := webvpn.GetManager().Session().UserFromToken(tok)
	ast.True(ok, "未吊销前会话应可用")
	ast.Equal("alice", u.Username)

	webvpn.GetManager().Revoker().RevokeUser("alice")

	_, ok = webvpn.GetManager().Session().UserFromToken(tok)
	ast.False(ok, "踢出后同进程内旧会话应立即失效")

	dbdata.WebVpnRevokeReset()

	_, ok = webvpn.GetManager().Session().UserFromToken(tok)
	ast.False(ok, "重启（内存清空）后旧会话应仍按 DB 阈值失效")
}

// 踢出仅使「踢出前签发」的会话失效，「踢出后新签发」不受影响
func TestWebVpnRevokedBeforeThreshold(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	oldTok := issueWebVpnTokenForTest(t, "bob", []string{"g"},
		time.Now().Add(-10*time.Minute).Unix(), time.Now().Add(2*time.Hour).Unix())
	ast.True(webVpnUserFromTokenOK(oldTok), "踢出前旧会话应可用")

	webvpn.GetManager().Revoker().RevokeUser("bob")

	ast.False(webVpnUserFromTokenOK(oldTok), "踢出后旧会话应失效")

	// 等待秒级时间戳跨秒，确保新会话的 iat 严格晚于阈值 T（否则会误判为踢出前签发）
	sec := time.Now().Unix()
	for time.Now().Unix() == sec {
		time.Sleep(10 * time.Millisecond)
	}

	newTok := issueWebVpnTokenForTest(t, "bob", []string{"g"},
		time.Now().Unix(), time.Now().Add(2*time.Hour).Unix())
	ast.True(webVpnUserFromTokenOK(newTok), "踢出后新会话应可用")
}

// 会话绝对寿命自 webvpn_issued 起算，超 WebVpnSessionMaxLifetime 强制失效
func TestWebVpnSessionAbsoluteMaxLifetime(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	now := time.Now().Unix()
	exp := now + int64((2 * time.Hour).Seconds())

	within := issueWebVpnTokenWithIssued(t, "alice", []string{"alice-group"},
		now, exp, now-int64(time.Hour.Seconds()))
	ast.True(webVpnUserFromTokenOK(within), "会话寿命内（issued 1h 前）应可用")

	expired := issueWebVpnTokenWithIssued(t, "alice", []string{"alice-group"},
		now, exp, now-int64((9*time.Hour).Seconds()))
	ast.False(webVpnUserFromTokenOK(expired), "会话超绝对寿命（issued 9h 前）应强制失效")

	legacy := issueWebVpnTokenForTest(t, "alice", []string{"alice-group"}, now, exp)
	ast.True(webVpnUserFromTokenOK(legacy), "无锚点旧 token 以 iat 兜底、寿命内应可用")
}

func webVpnUserFromTokenOK(token string) bool {
	_, ok := webvpn.GetManager().Session().UserFromToken(token)
	return ok
}

// P1-6：子域请求门户写接口必须 403，不得 delegate（否则跨子域门户越权）
func TestWebVpnHandlerSubdomainPortalApiForbidden(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	for _, p := range []string{
		"/portal/api/logout",
		"/portal/api/change_password",
		"/portal/api/devices/offline",
	} {
		req := httptest.NewRequest(http.MethodPost, p, nil)
		req.Host = "evil.wv.example.com" // 任意 WebVPN 子域
		rec := httptest.NewRecorder()
		handled := WebVpnHandler(rec, req)
		ast.True(handled, "子域 /portal/api/* 应由 WebVpnHandler 拦截（不应 delegate），path=%s", p)
		ast.Equal(http.StatusForbidden, rec.Code, "子域 /portal/api/* 应返回 403，path=%s", p)
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(m, "/portal", nil)
		req.Host = "evil.wv.example.com"
		rec := httptest.NewRecorder()
		handled := WebVpnHandler(rec, req)
		ast.True(handled, "子域非 GET /portal 应由 WebVpnHandler 拦截，method=%s", m)
		ast.Equal(http.StatusForbidden, rec.Code, "子域非 GET /portal 应返回 403，method=%s", m)
	}
}

// 子域登录流程必需的门户接口放行 delegate，否则登录被 403 拦死
func TestWebVpnHandlerSubdomainPortalLoginAllowed(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/portal/api/login"},
		{http.MethodPost, "/portal/api/verify"},
		{http.MethodPost, "/portal/api/sms/send"},
		{http.MethodPost, "/portal/api/sms/verify"},
		{http.MethodGet, "/portal/api/login-config"},
		{http.MethodGet, "/portal/api/me"},
		{http.MethodGet, "/portal/api/otp/status"},
		{http.MethodGet, "/portal/api/sso"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		req.Host = "app.wv.example.com" // 任意 WebVPN 子域
		rec := httptest.NewRecorder()
		handled := WebVpnHandler(rec, req)
		ast.False(handled, "子域登录前置接口应 delegate 回主路由，method=%s path=%s", c.method, c.path)
		ast.NotEqual(http.StatusForbidden, rec.Code, "子域登录前置接口不应 403，method=%s path=%s", c.method, c.path)
	}
}

// 子域下可 delegate 的路径为 /webvpn/*、/ui/*、/portal GET
func TestWebVpnHandlerSubdomainWhitelistAllowed(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/webvpn/me"},
		{http.MethodPost, "/webvpn/logout"},
		{http.MethodGet, "/ui/static/js/app.js"},
		{http.MethodGet, "/portal"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		req.Host = "app.wv.example.com"
		rec := httptest.NewRecorder()
		handled := WebVpnHandler(rec, req)
		ast.False(handled, "子域白名单路径应 delegate 回主路由，method=%s path=%s", c.method, c.path)
	}
}

// 子域非白名单路径转交代理，避免落入门户处理器
func TestWebVpnHandlerSubdomainNonWhitelistProxy(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	req := httptest.NewRequest(http.MethodGet, "/some/random/path", nil)
	req.Host = "app.wv.example.com"
	rec := httptest.NewRecorder()
	handled := WebVpnHandler(rec, req)
	ast.True(handled, "子域非白名单路径应由 WebVpnHandler 转交代理")
}

// 主域名的 /portal/api/* 不被拦截，delegate 回主路由
func TestWebVpnHandlerNonSubdomainIgnored(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	req := httptest.NewRequest(http.MethodPost, "/portal/api/logout", nil)
	req.Host = "www.example.com" // 非 WebVPN 子域
	rec := httptest.NewRecorder()
	handled := WebVpnHandler(rec, req)
	ast.False(handled, "非 WebVPN 子域请求不应由 WebVpnHandler 处理")
}

// 注销端点 CSRF 同源校验：同域放行，跨站拒绝
func TestWebVpnSameOrigin(t *testing.T) {
	ast := assert.New(t)

	r1 := httptest.NewRequest(http.MethodPost, "/webvpn/logout", nil)
	r1.Host = "app.wv.example.com"
	r1.Header.Set("Origin", "http://app.wv.example.com")
	ast.True(webVpnSameOrigin(r1), "同域 Origin 应放行")

	r2 := httptest.NewRequest(http.MethodPost, "/webvpn/logout", nil)
	r2.Host = "app.wv.example.com"
	r2.Header.Set("Origin", "https://evil.com")
	ast.False(webVpnSameOrigin(r2), "跨站 Origin 必须拒绝")

	r3 := httptest.NewRequest(http.MethodPost, "/webvpn/logout", nil)
	r3.Host = "app.wv.example.com"
	r3.Header.Set("Referer", "https://app.wv.example.com/some/page")
	ast.True(webVpnSameOrigin(r3), "同域 Referer 应放行")

	r4 := httptest.NewRequest(http.MethodPost, "/webvpn/logout", nil)
	r4.Host = "app.wv.example.com"
	ast.True(webVpnSameOrigin(r4), "无来源头应放行（同域原生导航）")
}

// 第三方登录回跳仅放行 https 且属于 .WebVpnDomain 后缀
func TestWebVpnSafeRedirect(t *testing.T) {
	_, teardown := setupWebVpnTest(t) // = "wv.example.com"
	defer teardown()
	ast := assert.New(t)

	ast.True(webVpnSafeRedirect("https://app.wv.example.com/foo"), "子域 https 应放行")
	ast.True(webVpnSafeRedirect("https://app.wv.example.com:4343/foo"), "子域带端口 https 应放行")
	ast.True(webVpnSafeRedirect("https://wv.example.com/foo"), "主域 https 应放行")
	ast.False(webVpnSafeRedirect("https://evil.com/foo"), "外部域名应拒绝")
	ast.False(webVpnSafeRedirect("https://wv.example.com.evil.org/foo"), "子域混淆应拒绝")
	ast.False(webVpnSafeRedirect("http://app.wv.example.com/foo"), "非 https 应拒绝")
	ast.False(webVpnSafeRedirect(""), "空地址应拒绝")
	ast.False(webVpnSafeRedirect("/foo"), "相对地址应拒绝")
}

// 主域地址取 webvpn_sso_domain：无端口沿用来源端口，未配置返回空
func TestWebVpnPortalMainDomain(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.WebVpnSsoDomain = "vpntest.example.com"
	})

	r1 := httptest.NewRequest(http.MethodGet, "/portal/api/sso", nil)
	r1.Host = "app.wv.example.com:4343"
	ast.Equal("https://vpntest.example.com:4343", portalMainDomain(r1), "子域带端口应沿用端口")

	r2 := httptest.NewRequest(http.MethodGet, "/portal/api/sso", nil)
	r2.Host = "app.wv.example.com"
	ast.Equal("https://vpntest.example.com", portalMainDomain(r2), "子域无端口时门户域名不带端口")

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.WebVpnSsoDomain = ""
	})
	r3 := httptest.NewRequest(http.MethodGet, "/portal/api/sso", nil)
	r3.Host = "app.wv.example.com:4343"
	ast.Equal("", portalMainDomain(r3), "未配置 webvpn_sso_domain 应返回空")
}

// 子域三方登录 302 到 webvpn_sso_domain 并透传 redirect 回跳
func TestWebVpnPortalSSOSubdomainRedirect(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
		c.WebVpnSsoDomain = "vpntest.example.com"
	})

	req := httptest.NewRequest(http.MethodGet,
		"/portal/api/sso?type=wxwork&redirect="+url.QueryEscape("https://app.wv.example.com:4343/"),
		nil)
	req.Host = "app.wv.example.com:4343"
	rec := httptest.NewRecorder()
	PortalSSO(rec, req)

	ast.Equal(http.StatusFound, rec.Code, "子域第三方登录应 302 跳门户域名")
	loc := rec.Header().Get("Location")
	ast.Contains(loc, "https://vpntest.example.com:4343/portal/api/sso", "应跳转到门户域名 sso 接口，实际: %s", loc)
	ast.Contains(loc, "redirect="+url.QueryEscape("https://app.wv.example.com:4343/"), "应透传回跳地址，实际: %s", loc)

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.WebVpnSsoDomain = ""
	})
	req2 := httptest.NewRequest(http.MethodGet,
		"/portal/api/sso?type=wxwork&redirect="+url.QueryEscape("https://app.wv.example.com:4343/"),
		nil)
	req2.Host = "app.wv.example.com:4343"
	rec2 := httptest.NewRecorder()
	PortalSSO(rec2, req2)
	ast.Equal(http.StatusBadRequest, rec2.Code, "未配置 webvpn_sso_domain 时子域三方登录应返回 400")
}

// Host 大小写不敏感，大写 Host 仍识别为 WebVPN 子域
func TestWebVpnHostPrefixCaseInsensitive(t *testing.T) {
	_, teardown := setupWebVpnTest(t) // = "wv.example.com"
	defer teardown()
	ast := assert.New(t)

	ast.Equal("app", hostPrefixOf(t, "app.wv.example.com"), "小写子域应识别")
	ast.Equal("app", hostPrefixOf(t, "APP.WV.EXAMPLE.COM"), "大写 Host 应识别为小写前缀（防绕过）")
	ast.Equal("app", hostPrefixOf(t, "App.wv.example.com:4343"), "混合大小写带端口应识别")
	ast.Equal("", hostPrefixOf(t, "www.example.com"), "非本域应拒绝")
	ast.Equal("", hostPrefixOf(t, "app.wv.example.com.evil.org"), "子域混淆应拒绝")
}

// 反代剥离 RemLink 自有会话 cookie，不透传网关令牌给后端
func TestWebVpnStripRemLinkCookies(t *testing.T) {
	ast := assert.New(t)

	cookies := []*http.Cookie{
		{Name: "webvpn_session", Value: "s1"},
		{Name: "portal_session", Value: "s2"},
		{Name: "auth-session-id", Value: "s3"},
		{Name: "acSamlv2Token", Value: "s4"},
		{Name: "jsessionid", Value: "backend-java"}, // 后端自身 cookie 应保留
	}
	kept := webvpn.StripRemLinkCookies(cookies)
	ast.NotContains(kept, "webvpn_session", "应剥离 webvpn_session")
	ast.NotContains(kept, "portal_session", "应剥离 portal_session")
	ast.NotContains(kept, "auth-session-id", "应剥离 auth-session-id")
	ast.NotContains(kept, "acSamlv2Token", "应剥离 acSamlv2Token")
	ast.Contains(kept, "jsessionid", "后端自身 cookie 应保留")
	ast.Contains(kept, "backend-java", "后端自身 cookie 值应保留")
}

func hostPrefixOf(_ *testing.T, host string) string {
	p, ok := webVpnHostPrefix(host)
	if !ok {
		return ""
	}
	return p
}

// 子域登录门户只签 webvpn_grant 不写 portal_session（避免父域 cookie 污染）
func TestPortalLoginOnWebVpnSubdomainSkipsPortalCookie(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	req := httptest.NewRequest(http.MethodPost, "/portal/api/login", nil)
	req.Host = "app.wv.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")

	rec := httptest.NewRecorder()
	portalIssueLoginResponse(rec, req, &dbdata.User{Username: "alice"}, "test")

	var hasPortal, hasGrant bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == portalCookieName {
			hasPortal = true
		}
		if c.Name == "webvpn_grant" {
			hasGrant = true
		}
	}
	ast.False(hasPortal, "子域登录不应写 portal_session（避免污染父域门户登录态）")
	ast.True(hasGrant, "子域登录应签发 webvpn_grant（供子域兑换会话）")

	req2 := httptest.NewRequest(http.MethodPost, "/portal/api/login", nil)
	req2.Host = "mv.example.com"
	req2.Header.Set("X-Forwarded-Proto", "https")
	rec2 := httptest.NewRecorder()
	portalIssueLoginResponse(rec2, req2, &dbdata.User{Username: "alice"}, "test")
	var hasPortal2 bool
	for _, c := range rec2.Result().Cookies() {
		if c.Name == portalCookieName {
			hasPortal2 = true
		}
	}
	ast.True(hasPortal2, "父域登录应正常写 portal_session")
}

// 兑换出的会话按目标应用鉴权，不通吃所有应用
func TestWebVpnSessionScopedToAppPermission(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	portalTok, err := jwt.SetJwtData(map[string]any{"portal_user": "alice"}, time.Now().Add(time.Hour).Unix())
	ast.NoError(err)
	portalJTI, err := jwt.JtiOf(portalTok)
	ast.NoError(err)
	grantTok, err := webvpn.GetManager().Session().IssueGrant(
		nil, nil, &dbdata.User{Username: "alice", Type: "local"}, portalJTI)
	ast.NoError(err)
	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true, portal: portalTok})
	req.AddCookie(&http.Cookie{Name: "webvpn_grant", Value: grantTok})
	ast.True(WebVpnHandler(rec, req))
	ast.Equal(http.StatusOK, rec.Code, "已授权应用 app1 应放行")

	var sessTok string
	for _, c := range rec.Result().Cookies() {
		if c.Name == webVpnSessionCookie {
			sessTok = c.Value
		}
	}
	ast.NotEmpty(sessTok, "兑换后应下发 webvpn_session")

	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app2", noSession: true})
	req2.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: sessTok})
	webVpnProxy(rec2, req2, "app2")
	ast.Equal(http.StatusForbidden, rec2.Code, "未授权应用必须 403")

	req3, rec3, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req3.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: sessTok})
	webVpnProxy(rec3, req3, "app1")
	ast.Equal(http.StatusOK, rec3.Code, "已授权应用应仍放行")
}

func TestWebVpnRevokedSessionDeniedOnEveryApp(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	_, _, token := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", user: "alice"})
	ast.NotEmpty(token)

	webvpn.GetManager().Revoker().RevokeUser("alice")

	req1, rec1, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req1.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: token})
	webVpnProxy(rec1, req1, "app1")
	ast.NotEqual(http.StatusOK, rec1.Code, "踢出后该会话访问 app1 必须失效")

	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "apphost", noSession: true})
	req2.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: token})
	webVpnProxy(rec2, req2, "apphost")
	ast.NotEqual(http.StatusOK, rec2.Code, "踢出后该会话访问其他应用也必须失效")
}
