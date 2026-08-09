package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/admin"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/webvpn"
)

// webVpnProxy 的集成测试：用 httptest 起后端，直接调用真实代理代码，
// 覆盖 design-webvpn.md checklist M5 的代理层断言（入站头清洗 / Host 改写 /
// cookie 剥离 / 302 Location 改写 / 安全头剥离 / 502 错误页 / 授权拒绝 /
// 未登录重定向 / 整用户踢出失效）。

var backendSeen struct {
	sync.Mutex
	last *http.Request
}

func setupWebVpnTest(t *testing.T) (backend *httptest.Server, teardown func()) {
	// 配置
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.DbType = "sqlite3"
		c.DbSource = path.Join(t.TempDir(), "webvpn_test.db")
		c.WebVpnDomain = "wv.example.com"
		c.JwtSecret = "unit-test-secret"
	})
	base.ReinitLog() // 初始化日志器，避免 base.Info/Error 在测试中 nil panic

	// 清空跨测试残留的整用户吊销阈值，避免前一个测试 WebVpnRevokeAllForUser 的副作用
	// 导致本测试会话被误判为已吊销（吊销状态存于包级全局 map，需显式重置）。
	dbdata.WebVpnRevokeReset()

	// 初始化全局数据库
	dbdata.Start()

	// 后端：记录收到的请求头，并按路径返回特定响应
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
			// 模拟慢响应：proxy 侧不应被 100s deadline 掐断（此处用短等待验证通道可用）
			time.Sleep(50 * time.Millisecond)
		default:
			w.Header().Set("X-Backend", "hit")
			w.Write([]byte("OK backend"))
		}
	}))

	beURL, _ := url.Parse(backend.URL)

	// 应用：app1 授权给 alice；app2 仅授权给 bob（alice 访问应 403）
	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "app1", Backend: beURL.String(), Status: 1, Users: []string{"alice"},
	}))
	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "app2", Backend: beURL.String(), Status: 1, Users: []string{"bob"},
	}))
	// appg：仅授权给 ops 组（alice 无该组应 403）
	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "appg", Backend: beURL.String(), Status: 1, Users: []string{"alice"}, Groups: []string{"ops"},
	}))
	// appip：仅允许来源 IP 在 10.0.0.0/8（客户端 203.0.113.9 不在内，应 403）
	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "appip", Backend: beURL.String(), Status: 1, Users: []string{"alice"},
		IpAllowList: []string{"10.0.0.0/8"},
	}))
	// apppath：仅允许路径前缀 /api/（访问 /admin 应 403）
	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "apppath", Backend: beURL.String(), Status: 1, Users: []string{"alice"},
		AllowPath: []string{"/api/"},
	}))
	// apphost：反向代理时改写后端 Host 为 backend.internal
	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "apphost", Backend: beURL.String(), Status: 1, Users: []string{"alice"},
		HostRewrite: "backend.internal",
	}))
	// 禁用应用：先创建（默认启用），再更新为禁用
	appx := &dbdata.WebVpnApp{Name: "appx", Backend: beURL.String(), Users: []string{"alice"}}
	assert.NoError(t, dbdata.SetWebVpnApp(appx))
	got, err := dbdata.GetWebVpnAppByName("appx")
	assert.NoError(t, err)
	got.Status = 0
	assert.NoError(t, dbdata.SetWebVpnApp(got))

	// 用户
	alice := &dbdata.User{Username: "alice", Type: "local", Status: 1}
	assert.NoError(t, dbdata.Add(alice))
	bob := &dbdata.User{Username: "bob", Type: "local", Status: 1}
	assert.NoError(t, dbdata.Add(bob))

	// 重置跨用例的全局状态，消除用例间相互污染（踢出水位、用户缓存）
	dbdata.WebVpnRevokeReset()
	webvpn.GetManager().Session().ResetCache()

	// 审计批处理协程：每用例独立启停（日志器已在 base.ReinitLog 初始化后才启动）
	webvpn.GetManager().Audit().Start()
	teardown = func() {
		webvpn.GetManager().Audit().Stop()
		// 失效 AppStore 缓存：每个用例 upsert 的 app 含不同后端端口，
		// 不清会残留在 60s TTL 缓存里，导致后续用例反代连到已关闭端口（502）。
		webvpn.GetManager().Apps().Invalidate()
		backend.Close()
		// 先排空异步日志 worker pool 再关库，避免 Stop 关库后残留 worker 读 xdb 竞态
		dbdata.UserActLogIns.Pool.Release()
		dbdata.UserActLogIns.Pool = utils.NewWorkerPool(1, 100)
		dbdata.Stop()
	}
	return backend, teardown
}

// newWebVpnReq 构造一个带 webvpn_session 的代理请求
func newWebVpnReq(t *testing.T, _ *httptest.Server, user, path string) (*http.Request, *httptest.ResponseRecorder) {
	token, err := admin.SetJwtData(map[string]any{
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
	// RemoteAddr 模拟客户端来源
	req.RemoteAddr = "203.0.113.9:54321"

	rec := httptest.NewRecorder()
	return req, rec
}

// webVpnReqOpts 构造代理请求的自定义选项
type webVpnReqOpts struct {
	host      string   // 子域名，默认 app1
	user      string   // webvpn_user，默认 alice
	groups    []string // webvpn_groups，默认空
	path      string   // 请求路径，默认 /
	clientIP  string   // RemoteAddr 的 IP 部分，默认 203.0.113.9
	portal    string   // 附加的 portal_session cookie 值（用于兑换测试）
	iatOffset int64    // 签发 iat 相对现在的偏移（秒），默认 0
	expOffset int64    // 过期时间相对现在偏移（秒），默认 +3h
	noSession bool     // 不带 webvpn_session cookie
}

// newWebVpnReqEx 通用请求构造：支持组授权、来源 IP、门户兑换、滑动续期等场景。
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
		// 直接构造 claims 以精确控制 iat（SetJwtData 内部会覆盖 iat 为 now）
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
	// 注意：必须用 AddCookie 追加，不能用 Header.Set("Cookie",...) 覆盖已设置的会话 cookie
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

// issueWebVpnTokenForTest 直接签发 WebVPN 会话 JWT，可精确控制 iat/exp。
func issueWebVpnTokenForTest(t *testing.T, user string, groups []string, iat, exp int64) string {
	return issueWebVpnTokenWithIssued(t, user, groups, iat, exp, 0)
}

// issueWebVpnTokenWithIssued 在 issueWebVpnTokenForTest 基础上可额外控制 webvpn_issued 锚点
// （首次登录时间）。issued=0 时不写入该锚点（旧 token 行为）。
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
	tok, err := admin.SetJwtData(data, exp)
	assert.NoError(t, err)
	return tok
}

func TestWebVpnProxyRewrite(t *testing.T) {
	backend, teardown := setupWebVpnTest(t)
	defer teardown()
	beHost := strings.TrimPrefix(backend.URL, "http://")

	req, rec := newWebVpnReq(t, backend, "alice", "/")
	webVpnProxy(rec, req, "app1")

	// 后端应收到改写后的 Host（= 后端地址），而非子域名
	backendSeen.Lock()
	got := backendSeen.last
	backendSeen.Unlock()
	assert.NotNil(t, got, "后端应收到请求")
	assert.Equal(t, beHost, got.Host, "Host 应改写为后端地址")
	// 入站 XFF 被清洗后用真实客户端 IP 重写
	assert.Equal(t, "203.0.113.9", got.Header.Get("X-Forwarded-For"), "XFF 应重写为真实客户端 IP")
	assert.Equal(t, "app1.wv.example.com", got.Header.Get("X-Forwarded-Host"))
	// RemLink 自有会话 cookie（portal_session）不应泄漏给后端，
	// 但应用自身 cookie（如 theme）必须原样转发，否则无法在后端应用内登录。
	assert.NotContains(t, got.Header.Get("Cookie"), "portal_session", "portal_session 不应泄漏给后端")
	assert.Contains(t, got.Header.Get("Cookie"), "theme=dark", "应用自身 cookie 应转发给后端")
	// 代理标记应注入
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

// 验证 Location 改写的主机匹配逻辑：
// 仅当 Location 主机与后端主机相等或为其后缀子域时改写，避免 strings.Contains 误配无关域名。
func TestWebVpnHostMatchesBackend(t *testing.T) {
	ast := assert.New(t)
	// 相等
	ast.True(webvpn.HostMatchesBackend("10.0.0.5:8080", "10.0.0.5:8080"))
	ast.True(webvpn.HostMatchesBackend("backend.internal", "backend.internal"))
	// 后缀子域
	ast.True(webvpn.HostMatchesBackend("app.backend.internal", "backend.internal"))
	ast.True(webvpn.HostMatchesBackend("api.10.0.0.5", "10.0.0.5"))
	// 无关域名（包含子串但非后缀子域）——必须为 false
	ast.False(webvpn.HostMatchesBackend("badexample.com.evil.org", "example.com"))
	ast.False(webvpn.HostMatchesBackend("notbackend.internal", "backend.internal"))
	// IP 尾缀误配：10.0.0.50 不应命中 10.0.0.5（点边界分隔）
	ast.False(webvpn.HostMatchesBackend("10.0.0.50", "10.0.0.5"))
	// 空值
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

	// alice 不在 app2 的白名单 -> 403
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
	// teardown 仍会调用 backend.Close（幂等）和 eng.Close

	req, rec := newWebVpnReq(t, backend, "alice", "/")
	webVpnProxy(rec, req, "app1")
	assert.Equal(t, http.StatusBadGateway, rec.Code, "后端不可达应 502")
	assert.Contains(t, rec.Body.String(), "后端", "应返回友好错误页")
	teardown()
}

// TestWebVpnProxySkipVerify 验证后端为自签证书时，skip_verify 开启可跳过校验、关闭则 502。
func TestWebVpnProxySkipVerify(t *testing.T) {
	backend, teardown := setupWebVpnTest(t)
	defer teardown()

	// 起一个自签证书的 https 后端（httptest.NewTLSServer 使用不被信任的证书）
	tlsBackend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("tls-ok"))
	}))
	defer tlsBackend.Close()

	// 关闭 skip_verify：证书不受信任，应 502
	assert.NoError(t, dbdata.SetWebVpnApp(&dbdata.WebVpnApp{
		Name: "tlsnoskip", Backend: tlsBackend.URL, Status: 1, Users: []string{"alice"},
	}))
	req, rec := newWebVpnReq(t, backend, "alice", "/")
	req.Host = "tlsnoskip.wv.example.com"
	webVpnProxy(rec, req, "tlsnoskip")
	assert.Equal(t, http.StatusBadGateway, rec.Code, "自签证书且未开启跳过校验应 502")

	// 开启 skip_verify：应成功反代
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

	// 踢出 alice 前应可解析
	token, err := admin.SetJwtData(map[string]any{
		"webvpn_user": "alice", "webvpn_type": "local", "webvpn_groups": []string{},
	}, time.Now().Add(3*time.Hour).Unix())
	assert.NoError(t, err)
	u, ok := webvpn.GetManager().Session().UserFromToken(token)
	assert.True(t, ok)
	assert.Equal(t, "alice", u.Username)

	// 整用户踢出
	webvpn.GetManager().Revoker().RevokeUser("alice")
	u, ok = webvpn.GetManager().Session().UserFromToken(token)
	assert.False(t, ok, "踢出后旧会话应立即失效")
}

// TestWebVpnProxyStripClientIPSpoofing 验证设计 §3 入站头清洗：
// 客户端伪造的 X-Forwarded-For / X-Real-IP 必须被丢弃，后端只收到真实 RemoteAddr。
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
	// 真实客户端 IP 来自 RemoteAddr，非伪造值
	assert.Equal(t, "203.0.113.9", got.Header.Get("X-Forwarded-For"), "XFF 应重写为真实客户端 IP")
	assert.Empty(t, got.Header.Get("X-Real-IP"), "伪造的 X-Real-IP 必须被剥离")
	assert.NotContains(t, got.Header.Get("X-Forwarded-For"), "1.2.3.4", "伪造 XFF 不应泄漏")
}

// TestWebVpnProxyStripAllSecurityHeaders 验证设计 §3：后端下发的 4 类跨域约束头全部剥离。
func TestWebVpnProxyStripAllSecurityHeaders(t *testing.T) {
	backend, teardown := setupWebVpnTest(t)
	defer teardown()

	// 临时给后端加一个返回全部安全头的路由
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

// TestWebVpnProxyGroupAuthorization 验证设计 §2/§3：组白名单拦截无组用户。
func TestWebVpnProxyGroupAuthorization(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	// alice 无 ops 组 → 403
	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "appg", groups: []string{"dev"}})
	webVpnProxy(rec, req, "appg")
	assert.Equal(t, http.StatusForbidden, rec.Code, "无授权组的用户应 403")

	// alice 有 ops 组 → 200
	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "appg", groups: []string{"ops"}})
	webVpnProxy(rec2, req2, "appg")
	assert.Equal(t, http.StatusOK, rec2.Code, "命中授权组的用户应放行")
}

// TestWebVpnProxyIpAllowList 验证设计 §3：来源 IP 不在白名单应拒绝。
func TestWebVpnProxyIpAllowList(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	// 客户端 203.0.113.9 不在 10.0.0.0/8 → 403
	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "appip", clientIP: "203.0.113.9"})
	webVpnProxy(rec, req, "appip")
	assert.Equal(t, http.StatusForbidden, rec.Code, "不在 IP 白名单应 403")

	// 客户端 10.1.2.3 在 10.0.0.0/8 → 200
	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "appip", clientIP: "10.1.2.3"})
	webVpnProxy(rec2, req2, "appip")
	assert.Equal(t, http.StatusOK, rec2.Code, "命中 IP 白名单应放行")
}

// TestWebVpnProxyPathAllowList 验证设计 §3：路径前缀白名单拦截越权路径。
func TestWebVpnProxyPathAllowList(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	// /admin 不在 /api/ 前缀 → 403
	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "apppath", path: "/admin"})
	webVpnProxy(rec, req, "apppath")
	assert.Equal(t, http.StatusForbidden, rec.Code, "越权路径应 403")

	// /api/users 命中前缀 → 200
	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "apppath", path: "/api/users"})
	webVpnProxy(rec2, req2, "apppath")
	assert.Equal(t, http.StatusOK, rec2.Code, "命中路径白名单应放行")
}

// TestWebVpnProxySlidingRenewal 验证设计 §2 滑动续期：签名时间过早的会话不被直接拒绝，
// 代理层应放行并续期（此处仅验证放行，续期由 WebVpnHandler 外层完成）。
func TestWebVpnProxySlidingRenewal(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	// iat 设为 2 小时前，仍远未到 exp（3h 后），代理层应放行
	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{iatOffset: -2 * 3600})
	webVpnProxy(rec, req, "app1")
	assert.Equal(t, http.StatusOK, rec.Code, "滑动续期：旧但有效的会话应放行")
}

// TestWebVpnProxyHostRewrite 验证设计数据模型 HostRewrite 字段：反代时改写后端 Host。
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

// TestWebVpnProxyAppNotFound 验证设计 early-return：访问未配置的子域返回 404。
func TestWebVpnProxyAppNotFound(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	// 已登录用户访问未配置（不存在）的应用 -> 404；未登录场景由未登录跳转统一处理。
	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "ghost"})
	webVpnProxy(rec, req, "ghost")
	assert.Equal(t, http.StatusNotFound, rec.Code, "未配置的应用应 404")
}

// TestWebVpnExchangeFromPortal 验证设计 M3 免重复登录：门户登录后下发的
// 一次性 webvpn_grant 授权码可在 WebVPN 子域兑换正式会话（B 方案，避免读
// 门户通配 cookie 造成会话互相踩踏）。
func TestWebVpnExchangeFromPortal(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	// 模拟门户登录成功时签发的免登授权码（绑定门户会话 jti）。
	grantTok, err := webvpn.GetManager().Session().IssueGrant(
		nil, nil, &dbdata.User{Username: "alice", Type: "local", Status: 1}, "portal-jti-alice")
	assert.NoError(t, err)

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req.AddCookie(&http.Cookie{Name: "webvpn_grant", Value: grantTok})

	consumed := WebVpnHandler(rec, req)
	assert.True(t, consumed, "WebVPN 子域请求应被本处理器消费")
	assert.Equal(t, http.StatusOK, rec.Code, "持 webvpn_grant 应自动兑换并放行")
	// 应种回 webvpn_session cookie（跨子域父域 Domain）
	var gotWebVpn bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == webVpnSessionCookie {
			gotWebVpn = true
			assert.Equal(t, "example.com", c.Domain, "会话 cookie 应与门户共用父域 Domain（前导点被标准库归一化）")
		}
	}
	assert.True(t, gotWebVpn, "应种回 webvpn_session cookie")
}

// TestWebVpnGrantIsOneTime 验证 P0 修复：一次性免登授权（grant）兑换后自身 jti 被吊销，
// 同一 grant 二次兑换必须失败（防重放）；且兑换过程不得误杀门户会话 jti。
func TestWebVpnGrantIsOneTime(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	// 构造一个带固定 jti 的门户会话 token（未过期），作为 grant 的门户登录态凭据
	portalJTI := "portal-jti-alice"
	portalTok, err := admin.SetJwtData(map[string]any{"username": "alice"}, time.Now().Add(time.Hour).Unix())
	assert.NoError(t, err)

	grantTok, err := webvpn.GetManager().Session().IssueGrant(
		nil, nil, &dbdata.User{Username: "alice", Type: "local", Status: 1}, portalJTI)
	assert.NoError(t, err)

	// 第一次兑换：成功
	req1, rec1, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req1.AddCookie(&http.Cookie{Name: "webvpn_grant", Value: grantTok})
	assert.True(t, WebVpnHandler(rec1, req1), "首次兑换应消费请求")
	assert.Equal(t, http.StatusOK, rec1.Code, "首次兑换应放行")

	// 第二次用同一 grant 兑换：应失败（grant 自身 jti 已吊销）
	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req2.AddCookie(&http.Cookie{Name: "webvpn_grant", Value: grantTok})
	consumed2 := WebVpnHandler(rec2, req2)
	assert.True(t, consumed2, "二次兑换请求仍应被本处理器消费")
	assert.NotEqual(t, http.StatusOK, rec2.Code, "已兑换的 grant 二次兑换必须失败（防重放）")

	// 门户会话 jti 未被误杀：用门户 token 解析仍有效（GetJwtData 会校验 jti 吊销）
	_, err = admin.GetJwtData(portalTok)
	assert.NoError(t, err, "兑换 grant 不应吊销门户会话 jti，门户登录态仍有效")
}

// TestWebVpnExchangeKeepsPortalSession 验证 P0 修复：兑换 grant 不会把用户踢出门户。
func TestWebVpnExchangeKeepsPortalSession(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	portalJTI := "portal-jti-bob"
	portalTok, err := admin.SetJwtData(map[string]any{"username": "bob"}, time.Now().Add(time.Hour).Unix())
	assert.NoError(t, err)

	// app1 在 setup 中授权给 alice，用 alice 兑换才能拿到 200；
	// 本测试重点是验证兑换流程不误杀门户 jti，用户主体不影响该断言。
	grantTok, err := webvpn.GetManager().Session().IssueGrant(
		nil, nil, &dbdata.User{Username: "alice", Type: "local", Status: 1}, portalJTI)
	assert.NoError(t, err)

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req.AddCookie(&http.Cookie{Name: "webvpn_grant", Value: grantTok})
	WebVpnHandler(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 门户侧用同一 jti 的 token 访问门户接口应仍有效（未被吊销）
	_, err = admin.GetJwtData(portalTok)
	assert.NoError(t, err, "门户会话 jti 不应被 WebVPN 兑换流程吊销")
}

// TestWebVpnLogoutRevokesSession 验证设计 M3 单点登出：登出后原会话立即失效。
func TestWebVpnLogoutRevokesSession(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	// 先以合法会话通过代理
	req, rec, token := newWebVpnReqEx(t, webVpnReqOpts{host: "app1"})
	webVpnProxy(rec, req, "app1")
	assert.Equal(t, http.StatusOK, rec.Code)

	// 调用单点登出（吊销该会话 jti）
	logoutReq := httptest.NewRequest(http.MethodPost, "/webvpn/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: token})
	logoutRec := httptest.NewRecorder()
	webVpnLogout(logoutRec, logoutReq)

	// 用同一会话（被吊销的原 token）再访问 → 应被拒绝（登出后立即失效）
	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req2.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: token})
	webVpnProxy(rec2, req2, "app1")
	assert.NotEqual(t, http.StatusOK, rec2.Code, "登出后原会话应立即失效")
}

// TestPortalLogoutRevokesWebVpnSession 验证方案 A：门户主动登出时，
// 联动吊销该用户的 WebVPN 会话（webvpn_session），使子域残留的旧会话立即自愈。
// 否则门户登出后，浏览器里旧的 webvpn_session 仍可用（卡无权限用户 / 滞留旧权限），
// 只能等到期或管理员手动踢。
func TestPortalLogoutRevokesWebVpnSession(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	// 门户登出依赖 EnableUserPortal 开关，测试中显式开启
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
	})

	// 该用户先持有一个合法的 webvpn_session
	_, _, token := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", user: "alice"})
	assert.NotEmpty(t, token)

	// 构造门户登出请求：带 alice 的 portal_session
	portalTok, err := admin.SetJwtData(map[string]any{
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

	// 登出后，alice 的 webvpn_session 应立即失效（整用户吊销生效）
	mgr := webvpn.GetManager()
	_, ok := mgr.Session().UserFromToken(token)
	assert.False(t, ok, "门户登出后该用户的 WebVPN 会话应被联动吊销")

	// 其他用户（bob）的会话不受影响
	bobTok, err := admin.SetJwtData(map[string]any{
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

// TestWebVpnNoLoopWhenPortalLoggedInButDenied 验证修复：门户已登录、但 WebVPN 免登
// 兑换失败（权限中途被取消 / grant 过期 / 会话已被吊销）时，WebVpnHandler 必须直接
// 渲染无权限提示页（403），而不得 302 回登录页。否则门户已登录的前端会自动回跳、
// 后端又判定未登录再次跳转，形成高频率刷新死循环。
// TestWebVpnPortalLoggedInAutoExchange 验证设计核心：门户已登录用户访问 WebVPN 子域时，
// 通过免登兑换自动获得 WebVPN 会话并进入代理，而不会跳转到登录页（避免前端自动回跳→后端又判未登录→
// 再次跳转的刷新死循环）。门户会话 cookie 不会直接被当作 WebVPN 会话，必须经由 ExchangeGrant 兑换。
func TestWebVpnPortalLoggedInAutoExchange(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	// 构造一个合法的门户会话（已登录门户），但不带 webvpn_session、不带 webvpn_grant
	portalTok, err := admin.SetJwtData(map[string]any{
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
	// 门户已登录应免登兑换成功并进入代理（不会跳登录页，避免死循环）
	assert.Equal(t, http.StatusOK, rec.Code, "门户已登录用户应通过免登兑换直接进入，而非跳登录页")
	assert.Empty(t, rec.Header().Get("Location"), "不应重定向到登录页")
	// 兑换成功后应写入 webvpn 会话 cookie，使后续请求直接走 CurrentUser
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

// TestWebVpnProxyAuditLogged 验证设计 §6：每次代理请求落一条审计记录（含真实客户端 IP）。
func TestWebVpnProxyAuditLogged(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()

	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", clientIP: "203.0.113.9"})
	webVpnProxy(rec, req, "app1")
	assert.Equal(t, http.StatusOK, rec.Code)

	// 等待审计批处理落库（ticker 1s）
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

// TestWebVpnRevokedPersistAcrossRestart 验证 P1-8 修复：
// 整用户踢出（webVpnRevokeAllForUser）的阈值持久化到 DB；
// 即使进程内存被清空（模拟重启，WebVpnRevokeReset 清内存缓存），旧 token 仍应被判定为失效，
// 解决原纯内存方案「重启后已踢用户旧会话又能用」的问题。
func TestWebVpnRevokedPersistAcrossRestart(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	// 签发 alice 的 WebVPN 会话
	tok := issueWebVpnTokenForTest(t, "alice", []string{"alice-group"},
		time.Now().Add(-time.Minute).Unix(), time.Now().Add(2*time.Hour).Unix())

	// 未吊销前应可解析
	u, ok := webvpn.GetManager().Session().UserFromToken(tok)
	ast.True(ok, "未吊销前会话应可用")
	ast.Equal("alice", u.Username)

	// 整用户踢出（写入 DB + 内存缓存）
	webvpn.GetManager().Revoker().RevokeUser("alice")

	// 同一次进程内：旧 token 立即失效
	_, ok = webvpn.GetManager().Session().UserFromToken(tok)
	ast.False(ok, "踢出后同进程内旧会话应立即失效")

	// 模拟重启：清空内存中的吊销阈值缓存（DB 仍有记录）
	dbdata.WebVpnRevokeReset()

	// 重启后：从 DB 读回阈值，旧 token 仍应失效（P1-8 核心断言）
	_, ok = webvpn.GetManager().Session().UserFromToken(tok)
	ast.False(ok, "重启（内存清空）后旧会话应仍按 DB 阈值失效")
}

// TestWebVpnRevokedBeforeThreshold 验证整用户踢出仅使「踢出前签发」的会话失效，
// 「踢出后新签发」的会话不受影响（保证管理员踢人后本人重新登录仍可正常使用）。
func TestWebVpnRevokedBeforeThreshold(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	// 早于踢出时间签发的旧会话
	oldTok := issueWebVpnTokenForTest(t, "bob", []string{"g"},
		time.Now().Add(-10*time.Minute).Unix(), time.Now().Add(2*time.Hour).Unix())
	ast.True(webVpnUserFromTokenOK(oldTok), "踢出前旧会话应可用")

	// 踢出 bob（阈值 = 当前秒级时间戳 T）
	webvpn.GetManager().Revoker().RevokeUser("bob")

	// 旧会话失效
	ast.False(webVpnUserFromTokenOK(oldTok), "踢出后旧会话应失效")

	// 跨过一秒，确保新会话的 iat 严格晚于阈值 T（否则会误判为踢出前签发）
	time.Sleep(1100 * time.Millisecond)

	// 踢出后新签发的会话（iat 晚于阈值、且不超过当前时刻以满足 JWT iat<=now 校验）仍可用
	newTok := issueWebVpnTokenForTest(t, "bob", []string{"g"},
		time.Now().Unix(), time.Now().Add(2*time.Hour).Unix())
	ast.True(webVpnUserFromTokenOK(newTok), "踢出后新会话应可用")
}

// TestWebVpnSessionAbsoluteMaxLifetime 验证 WebVPN 会话绝对寿命上限：
// 会话自首次登录（webvpn_issued 锚点）起算，超过 WebVpnSessionMaxLifetime（默认 480 分钟）
// 后无论是否持续活跃、不断滑动续期都强制失效，必须重新登录。
func TestWebVpnSessionAbsoluteMaxLifetime(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	now := time.Now().Unix()
	exp := now + int64((2 * time.Hour).Seconds())

	// 未超寿命：webvpn_issued 在 1 小时前（远小于默认 480 分钟），iat 近期 → 仍可用
	within := issueWebVpnTokenWithIssued(t, "alice", []string{"alice-group"},
		now, exp, now-int64(time.Hour.Seconds()))
	ast.True(webVpnUserFromTokenOK(within), "会话寿命内（issued 1h 前）应可用")

	// 超过绝对寿命：webvpn_issued 在 9 小时前（> 480 分钟），但 iat 近期（模拟刚滑动续期过）
	// → 虽 iat 新鲜，但首次登录已过上限，应强制失效
	expired := issueWebVpnTokenWithIssued(t, "alice", []string{"alice-group"},
		now, exp, now-int64((9*time.Hour).Seconds()))
	ast.False(webVpnUserFromTokenOK(expired), "会话超绝对寿命（issued 9h 前）应强制失效")

	// 旧 token 无锚点（webvpn_issued 缺失）：以 iat 兜底，iat 近期未超寿命 → 可用
	legacy := issueWebVpnTokenForTest(t, "alice", []string{"alice-group"}, now, exp)
	ast.True(webVpnUserFromTokenOK(legacy), "无锚点旧 token 以 iat 兜底、寿命内应可用")
}

// webVpnUserFromTokenOK 是 webVpnUserFromToken 的布尔包装，便于断言。
func webVpnUserFromTokenOK(token string) bool {
	_, ok := webvpn.GetManager().Session().UserFromToken(token)
	return ok
}

// TestWebVpnHandlerSubdomainPortalApiForbidden 验证 P1-6 修复：
// WebVPN 子域（*.WebVpnDomain）请求门户写接口 /portal/api/* 必须被拒绝（403），
// 不得 delegate 回主路由（否则会带上 .WebVpnDomain 通配的 portal_session cookie，
// 形成跨子域门户越权调用，如登出/改密码）。
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
	// 精确 /portal 仅放行 GET（登录页壳）；非 GET（POST/DELETE 等）一律 403，不得反代/委托。
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(m, "/portal", nil)
		req.Host = "evil.wv.example.com"
		rec := httptest.NewRecorder()
		handled := WebVpnHandler(rec, req)
		ast.True(handled, "子域非 GET /portal 应由 WebVpnHandler 拦截，method=%s", m)
		ast.Equal(http.StatusForbidden, rec.Code, "子域非 GET /portal 应返回 403，method=%s", m)
	}
}

// TestWebVpnHandlerSubdomainPortalLoginAllowed 验证：
// WebVPN 子域名登录页需在子域直接完成账号密码/短信/OTP 登录，并加载登录配置、检测登录态。
// 因此以下「未登录状态下登录流程必需、且不依赖已登录 portal_session」的门户接口应在子域放行
// （WebVpnHandler 返回 false，delegate 回主路由），否则登录会被 403 拦死，
// 表现为「网络请求失败」且第三方/短信登录配置加载不出来。
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

// TestWebVpnHandlerSubdomainWhitelistAllowed 验证 P1-6 白名单放行：
// WebVPN 子域下仅以下路径可 delegate 回主路由（WebVpnHandler 返回 false）：
//   - /webvpn/*  —— WebVPN 自有 API（登录/登出/me）
//   - /ui/*      —— 门户前端静态资源（登录卡片需要）
//   - /portal GET —— 登录页壳（只读重定向目标）
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

// TestWebVpnHandlerSubdomainNonWhitelistProxy 验证 P1-6：
// WebVPN 子域下非白名单的普通路径（如 /admin）不应落入门户接口，
// 而是由 WebVpnHandler 转交代理（返回 true，最终因无对应应用而 404），
// 避免被路由到门户的 /portal/* 处理器。
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

// TestWebVpnHandlerNonSubdomainIgnored 验证 WebVpnHandler 仅处理 WebVPN 子域请求：
// 主域名（非 *.WebVpnDomain）的 /portal/api/* 请求应不被拦截，delegate 回主路由。
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

// TestWebVpnSameOrigin 覆盖注销端点的 CSRF 同源校验：
// 同域 Origin / 同域 Referer / 无来源头应放行；跨站 Origin 必须拒绝。
func TestWebVpnSameOrigin(t *testing.T) {
	ast := assert.New(t)

	// 同域 http Origin（测试请求 TLS==nil，与 http 同源匹配）放行
	r1 := httptest.NewRequest(http.MethodPost, "/webvpn/logout", nil)
	r1.Host = "app.wv.example.com"
	r1.Header.Set("Origin", "http://app.wv.example.com")
	ast.True(webVpnSameOrigin(r1), "同域 Origin 应放行")

	// 跨站 Origin（evil.com）必须拒绝
	r2 := httptest.NewRequest(http.MethodPost, "/webvpn/logout", nil)
	r2.Host = "app.wv.example.com"
	r2.Header.Set("Origin", "https://evil.com")
	ast.False(webVpnSameOrigin(r2), "跨站 Origin 必须拒绝")

	// 无 Origin，但有同域 Referer 应放行
	r3 := httptest.NewRequest(http.MethodPost, "/webvpn/logout", nil)
	r3.Host = "app.wv.example.com"
	r3.Header.Set("Referer", "https://app.wv.example.com/some/page")
	ast.True(webVpnSameOrigin(r3), "同域 Referer 应放行")

	// 无 Origin 无 Referer（同域原生导航）应放行
	r4 := httptest.NewRequest(http.MethodPost, "/webvpn/logout", nil)
	r4.Host = "app.wv.example.com"
	ast.True(webVpnSameOrigin(r4), "无来源头应放行（同域原生导航）")
}

// TestWebVpnSafeRedirect 验证第三方登录成功后的回跳地址校验：
// 仅放行 https 且 host 属于 .WebVpnDomain 后缀（主域或其子域），拒绝外部/非 https/相对地址。
func TestWebVpnSafeRedirect(t *testing.T) {
	_, teardown := setupWebVpnTest(t) // WebVpnDomain = "wv.example.com"
	defer teardown()
	ast := assert.New(t)

	// 子域（WebVPN 应用）应放行，且允许带端口（自定义端口场景）
	ast.True(webVpnSafeRedirect("https://app.wv.example.com/foo"), "子域 https 应放行")
	ast.True(webVpnSafeRedirect("https://app.wv.example.com:4343/foo"), "子域带端口 https 应放行")
	// 主域应放行
	ast.True(webVpnSafeRedirect("https://wv.example.com/foo"), "主域 https 应放行")
	// 外部域名拒绝
	ast.False(webVpnSafeRedirect("https://evil.com/foo"), "外部域名应拒绝")
	// 子域混淆（后缀匹配误判）拒绝
	ast.False(webVpnSafeRedirect("https://wv.example.com.evil.org/foo"), "子域混淆应拒绝")
	// 非 https 拒绝
	ast.False(webVpnSafeRedirect("http://app.wv.example.com/foo"), "非 https 应拒绝")
	// 空/相对地址拒绝
	ast.False(webVpnSafeRedirect(""), "空地址应拒绝")
	ast.False(webVpnSafeRedirect("/foo"), "相对地址应拒绝")
}

// TestWebVpnPortalMainDomain 验证第三方登录子域跳主域时主域地址计算：
// portalMainDomain 仅取配置项 webvpn_sso_domain（WebVPN 第三方登录专用门户域名），
// 无端口时沿用请求来源端口；未配置返回 ""（子域名三方登录不可用）。
func TestWebVpnPortalMainDomain(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.WebVpnSsoDomain = "vpntest.example.com"
	})

	// 请求带端口：门户域名沿用该端口
	r1 := httptest.NewRequest(http.MethodGet, "/portal/api/sso", nil)
	r1.Host = "app.wv.example.com:4343"
	ast.Equal("https://vpntest.example.com:4343", portalMainDomain(r1), "子域带端口应沿用端口")

	// 请求无端口：门户域名不带端口
	r2 := httptest.NewRequest(http.MethodGet, "/portal/api/sso", nil)
	r2.Host = "app.wv.example.com"
	ast.Equal("https://vpntest.example.com", portalMainDomain(r2), "子域无端口时门户域名不带端口")

	// 未配置 webvpn_sso_domain：返回空（三方登录不可用）
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.WebVpnSsoDomain = ""
	})
	r3 := httptest.NewRequest(http.MethodGet, "/portal/api/sso", nil)
	r3.Host = "app.wv.example.com:4343"
	ast.Equal("", portalMainDomain(r3), "未配置 webvpn_sso_domain 应返回空")
}

// TestWebVpnPortalSSOSubdomainRedirect 验证子域名发起第三方登录时 PortalSSO 的行为：
// 应 302 跳转到「WebVPN 第三方登录专用门户域名」（webvpn_sso_domain）完成认证，
// 且透传 redirect（回跳子域名的完整 URL），保证认证成功后能回跳回原 WebVPN 子域名。
func TestWebVpnPortalSSOSubdomainRedirect(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
		c.WebVpnSsoDomain = "vpntest.example.com"
	})

	// 子域名发起第三方登录，携带回跳地址（子域完整 URL，含端口）
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

	// 未配置 webvpn_sso_domain：子域三方登录应返回 400（不可用），而非跳到错误域名
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

// TestWebVpnHostPrefixCaseInsensitive 验证大小写 Host 不会被绕过 WebVPN 分支：
// DNS 主机名大小写不敏感，大写 Host（如 APP.WV.EXAMPLE.COM）应仍被识别为 WebVPN 子域，
// 返回小写前缀，避免落入主路由绕过授权/审计/代理边界。
func TestWebVpnHostPrefixCaseInsensitive(t *testing.T) {
	_, teardown := setupWebVpnTest(t) // WebVpnDomain = "wv.example.com"
	defer teardown()
	ast := assert.New(t)

	ast.Equal("app", hostPrefixOf(t, "app.wv.example.com"), "小写子域应识别")
	ast.Equal("app", hostPrefixOf(t, "APP.WV.EXAMPLE.COM"), "大写 Host 应识别为小写前缀（防绕过）")
	ast.Equal("app", hostPrefixOf(t, "App.wv.example.com:4343"), "混合大小写带端口应识别")
	// 非本域/混淆仍应拒绝
	ast.Equal("", hostPrefixOf(t, "www.example.com"), "非本域应拒绝")
	ast.Equal("", hostPrefixOf(t, "app.wv.example.com.evil.org"), "子域混淆应拒绝")
}

// TestWebVpnStripRemLinkCookies 验证反向代理会剥离所有 RemLink 自有会话 cookie，
// 避免把网关会话令牌透传给被代理的内网后端。
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

// hostPrefixOf 是 webVpnHostPrefix 的测试辅助：返回前缀（不匹配时为空串）。
func hostPrefixOf(_ *testing.T, host string) string {
	p, ok := webVpnHostPrefix(host)
	if !ok {
		return ""
	}
	return p
}

// TestPortalLoginOnWebVpnSubdomainSkipsPortalCookie 验证：
// 在 WebVPN 子域下登录门户时，只签发 webvpn_grant（供子域兑换），
// 不写 portal_session。否则门户登录态会被父域共享 cookie 污染，
// 导致用户在父域门户无法切换到别的账号登录（只能沿用子域登录的用户）。
func TestPortalLoginOnWebVpnSubdomainSkipsPortalCookie(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	if base.GetCfg().WebVpnDomain == "" {
		t.Skip("WebVPN 未启用，跳过")
	}

	// 子域 host 下的登录请求
	req := httptest.NewRequest(http.MethodPost, "/portal/api/login", nil)
	req.Host = "app.wv.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")

	// 子域登录：只签发 webvpn_grant，不写 portal_session
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

	// 对照：父域 host 下登录应正常写 portal_session
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

// TestWebVpnSessionScopedToAppPermission 验证兑换出的会话按目标应用鉴权：
// 访问未授权应用应 403，已授权应用仍 200，不会因已建立会话而通吃所有应用。
func TestWebVpnSessionScopedToAppPermission(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	// alice 在已授权的 app1 用 grant 兑换正式会话
	grantTok, err := webvpn.GetManager().Session().IssueGrant(
		nil, nil, &dbdata.User{Username: "alice", Type: "local", Status: 1}, "portal-jti-alice")
	ast.NoError(err)
	req, rec, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
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

	// 同一会话访问未授权的 app2 → 403
	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app2", noSession: true})
	req2.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: sessTok})
	webVpnProxy(rec2, req2, "app2")
	ast.Equal(http.StatusForbidden, rec2.Code, "未授权应用必须 403")

	// 已授权的 app1 仍放行，证明会话有效、仅权限被精准拦截
	req3, rec3, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req3.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: sessTok})
	webVpnProxy(rec3, req3, "app1")
	ast.Equal(http.StatusOK, rec3.Code, "已授权应用应仍放行")
}

// TestWebVpnRevokedSessionDeniedOnEveryApp 验证整用户踢出后，
// 已建立的会话访问任意应用都失效（吊销跨应用生效）。
func TestWebVpnRevokedSessionDeniedOnEveryApp(t *testing.T) {
	_, teardown := setupWebVpnTest(t)
	defer teardown()
	ast := assert.New(t)

	_, _, token := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", user: "alice"})
	ast.NotEmpty(token)

	webvpn.GetManager().Revoker().RevokeUser("alice")

	// 已授权的 app1 → 失效
	req1, rec1, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "app1", noSession: true})
	req1.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: token})
	webVpnProxy(rec1, req1, "app1")
	ast.NotEqual(http.StatusOK, rec1.Code, "踢出后该会话访问 app1 必须失效")

	// 另一应用 apphost → 同样失效
	req2, rec2, _ := newWebVpnReqEx(t, webVpnReqOpts{host: "apphost", noSession: true})
	req2.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: token})
	webVpnProxy(rec2, req2, "apphost")
	ast.NotEqual(http.StatusOK, rec2.Code, "踢出后该会话访问其他应用也必须失效")
}
