package webvpn

import (
	"net/http"
	"net/http/httptest"
	"path"
	"testing"
	"time"

	"github.com/wsczx/remlink/admin"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

//   1) 会话清除域必须与写入域一致
//   2) 免登兑换的逆向安全：无 grant 且无门户会话时不得凭空造出会话

func setupWebVpnDB(t *testing.T) {
	t.Helper()
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.DbType = "sqlite3"
		c.DbSource = path.Join(t.TempDir(), "remlink_test.db")
		c.WebVpnDomain = "wv.example.com"
		c.JwtSecret = "unit-test-secret"
	})
	dbdata.Start()
}

// TestSessionClearDomainMatchesIssue 验证 ClearCookie 清除指令的 Domain
// 与写入 webvpn_session 的 Domain 逐字节一致（否则旧 cookie 残留）。
func TestSessionClearDomainMatchesIssue(t *testing.T) {
	setupWebVpnDB(t)
	defer dbdata.Stop()

	require.NoError(t, dbdata.Add(&dbdata.User{Username: "alice", Status: 1}))

	m := GetManager()
	host := "app.wv.example.com"
	r := httptest.NewRequest(http.MethodGet, "https://"+host+"/", nil)
	w := httptest.NewRecorder()

	_, err := m.Session().Issue(w, r, &dbdata.User{Username: "alice"}, 0)
	require.NoError(t, err)

	var issueDomain string
	for _, c := range w.Result().Cookies() {
		if c.Name == "webvpn_session" {
			issueDomain = c.Domain
		}
	}
	require.NotEmpty(t, issueDomain, "应写出 webvpn_session cookie")

	// 清除指令的 Domain 必须与写入域完全相同
	wc := httptest.NewRecorder()
	rc := httptest.NewRequest(http.MethodGet, "https://"+host+"/", nil)
	m.Session().ClearCookie(wc, rc)
	var clearDomain string
	for _, c := range wc.Result().Cookies() {
		if c.Name == "webvpn_session" {
			clearDomain = c.Domain
		}
	}
	require.NotEmpty(t, clearDomain, "应发出清除指令")
	assert.Equal(t, issueDomain, clearDomain,
		"webvpn_session 清除域必须与写入域一致，否则登出/切换用户后旧 cookie 残留")
}

// TestExchangeGrantMissing 免登兑换的逆向安全：既无 grant 也无门户会话时，
// ExchangeGrant 必须返回未兑换（不得凭空造出 WebVPN 会话）。
func TestExchangeGrantMissing(t *testing.T) {
	setupWebVpnDB(t)
	defer dbdata.Stop()

	m := GetManager()
	r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
	w := httptest.NewRecorder()
	_, _, ok := m.Session().ExchangeGrant(w, r)
	assert.False(t, ok, "无 grant 且无门户会话时不得兑换出会话")
}

// TestExchangeGrantRevokedUser 整用户踢出后，残留的免登授权不得再兑换出会话：
// 门户登出会抬高吊销阈值，签发早于阈值的 grant 必须失效，防止旧 grant 绕过吊销
// 兑换出新会话导致身份漂移（偶发"又要登录/两次登录"的根因之一）。
func TestExchangeGrantRevokedUser(t *testing.T) {
	setupWebVpnDB(t)
	defer dbdata.Stop()

	m := GetManager()
	require.NoError(t, dbdata.Add(&dbdata.User{Username: "alice", Status: 1}))

	// 1. 签发 grant（此时未吊销，可兑换）
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "https://portal.example.com/", nil)
	_, err := m.Session().IssueGrant(w1, r1, &dbdata.User{Username: "alice"}, "portal-jti-1")
	require.NoError(t, err)
	grant := findCookie(t, w1, grantCookieName)
	require.NotNil(t, grant, "应写出 grant cookie")

	// 2. 整用户踢出（抬高吊销阈值，晚于 grant 签发）
	m.Session().RevokeUser("alice")

	// 3. 携带残留 grant 尝试兑换，必须失败
	r2 := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
	r2.AddCookie(grant)
	w2 := httptest.NewRecorder()
	_, _, ok := m.Session().ExchangeGrant(w2, r2)
	assert.False(t, ok, "用户被整用户踢出后，残留 grant 不得兑换出会话")
}

// TestExchangeGrantConsumes 免登授权兑换成功后应被清除（一次性消费），
// 避免 grant 长期残留导致身份漂移与偶发"又要登录"。
// TestExchangeGrantRevokedFallsBackToPortalSession 验证残留 grant 被吊销时，
// 只跳过 grant 兑换，仍应使用吊销后签发的有效 portal_session 建立 WebVPN 会话。
func TestExchangeGrantRevokedFallsBackToPortalSession(t *testing.T) {
	setupWebVpnDB(t)
	defer dbdata.Stop()

	m := GetManager()
	require.NoError(t, dbdata.Add(&dbdata.User{Username: "carol", Status: 1}))

	// 先签发 grant，再整用户吊销，使该 grant 明确早于吊销阈值。
	grantResp := httptest.NewRecorder()
	grantReq := httptest.NewRequest(http.MethodGet, "https://portal.example.com/", nil)
	_, err := m.Session().IssueGrant(grantResp, grantReq, &dbdata.User{Username: "carol"}, "portal-jti-old")
	require.NoError(t, err)
	grant := findCookie(t, grantResp, grantCookieName)
	require.NotNil(t, grant, "应写出 grant cookie")
	m.Session().RevokeUser("carol")
	// 吊销阈值按 Unix 秒记录，确保门户会话的 iat 严格晚于阈值。
	time.Sleep(1100 * time.Millisecond)

	// 吊销后重新签发门户会话；它代表仍有效的门户登录态。
	portalToken, err := admin.SetJwtData(map[string]any{
		"portal_user": "carol",
		"portal_type": "local",
	}, time.Now().Add(time.Hour).Unix())
	require.NoError(t, err)

	r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
	r.AddCookie(grant)
	r.AddCookie(&http.Cookie{Name: "portal_session", Value: portalToken})
	w := httptest.NewRecorder()
	_, user, ok := m.Session().ExchangeGrant(w, r)

	assert.True(t, ok, "吊销残留 grant 后，仍有效的门户会话应能回退兑换")
	assert.NotNil(t, user)
	assert.Equal(t, "carol", user.Username)
}

func TestExchangeGrantConsumes(t *testing.T) {
	setupWebVpnDB(t)
	defer dbdata.Stop()

	m := GetManager()
	// 用独立用户名，避免受前序踢出测试的吊销阈值影响
	require.NoError(t, dbdata.Add(&dbdata.User{Username: "bob", Status: 1}))

	// 签发 grant
	w0 := httptest.NewRecorder()
	r0 := httptest.NewRequest(http.MethodGet, "https://portal.example.com/", nil)
	_, err := m.Session().IssueGrant(w0, r0, &dbdata.User{Username: "bob"}, "portal-jti-2")
	require.NoError(t, err)
	grant := findCookie(t, w0, grantCookieName)
	require.NotNil(t, grant, "应写出 grant cookie")

	// 兑换成功
	r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
	r.AddCookie(grant)
	w := httptest.NewRecorder()
	_, _, ok := m.Session().ExchangeGrant(w, r)
	assert.True(t, ok, "正常 grant 应能兑换")

	// 响应中应含清除 grant 的 Set-Cookie（MaxAge=-1）
	cleared := findCookie(t, w, grantCookieName)
	require.NotNil(t, cleared, "兑换成功后应清除 grant cookie")
	assert.Equal(t, -1, cleared.MaxAge, "清除指令应立即使 grant 过期")
}
