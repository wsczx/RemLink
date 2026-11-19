package webvpn

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	_ "github.com/wsczx/remlink/auth/authsrv" // 触发认证器 init 注册
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbtest"
	"github.com/wsczx/remlink/pkg/jwt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/dbdata"
)

// 测试共享实例
var testManager = NewManager()

func setupDB(t *testing.T) {
	t.Helper()
	dbtest.SetupDB(t)
	// 每个用例使用独立库：重置 manager 缓存，避免读到上一个库的会话
	testManager.Session().ResetCache()
	testManager.Apps().Invalidate()
	t.Cleanup(func() {
		testManager.Session().ResetCache()
		testManager.Apps().Invalidate()
	})
}

func TestSessionClearDomainMatchesIssue(t *testing.T) {
	setupDB(t)

	require.NoError(t, dbdata.GetDB().Insert(&dbdata.User{Username: "alice", Status: 1}))

	m := testManager
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

func TestExchangeGrantMissing(t *testing.T) {
	setupDB(t)

	m := testManager
	r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
	w := httptest.NewRecorder()
	_, _, ok := m.Session().ExchangeGrant(w, r)
	assert.False(t, ok, "无 grant 且无门户会话时不得兑换出会话")
}

func TestExchangeGrantRevokedUser(t *testing.T) {
	setupDB(t)

	m := testManager
	require.NoError(t, dbdata.GetDB().Insert(&dbdata.User{Username: "alice", Status: 1}))

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

func TestExchangeGrantRevokedFallsBackToPortalSession(t *testing.T) {
	setupDB(t)

	m := testManager
	require.NoError(t, dbdata.GetDB().Insert(&dbdata.User{Username: "carol", Status: 1}))

	// 先签发 grant，再整用户吊销，使该 grant 明确早于吊销阈值
	grantResp := httptest.NewRecorder()
	grantReq := httptest.NewRequest(http.MethodGet, "https://portal.example.com/", nil)
	_, err := m.Session().IssueGrant(grantResp, grantReq, &dbdata.User{Username: "carol"}, "portal-jti-old")
	require.NoError(t, err)
	grant := findCookie(t, grantResp, grantCookieName)
	require.NotNil(t, grant, "应写出 grant cookie")
	m.Session().RevokeUser("carol")
	// 吊销阈值按 Unix 秒记录：等待跨秒，确保门户会话的 iat 严格晚于阈值
	sec := time.Now().Unix()
	for time.Now().Unix() == sec {
		time.Sleep(10 * time.Millisecond)
	}

	// 吊销后重新签发门户会话；它代表仍有效的门户登录态
	portalToken, err := jwt.SetJwtData(map[string]any{
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

func TestExchangeGrantWorksWithoutPortalSessionCookie(t *testing.T) {
	setupDB(t)

	m := testManager
	require.NoError(t, dbdata.GetDB().Insert(&dbdata.User{Username: "grant-user", Status: 1}))
	grantResp := httptest.NewRecorder()
	grantReq := httptest.NewRequest(http.MethodGet, "https://portal.example.com/", nil)
	_, err := m.Session().IssueGrant(grantResp, grantReq, &dbdata.User{Username: "grant-user"}, "portal-jti")
	require.NoError(t, err)
	grant := findCookie(t, grantResp, grantCookieName)
	require.NotNil(t, grant)

	r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
	r.AddCookie(grant)
	_, _, ok := m.Session().ExchangeGrant(httptest.NewRecorder(), r)
	assert.True(t, ok, "缺少门户会话 Cookie 时，有效 grant 仍应允许兑换")
}
func TestExchangeGrantCookieRequiresClientIP(t *testing.T) {
	setupDB(t)

	m := testManager
	require.NoError(t, dbdata.GetDB().Insert(&dbdata.User{Username: "alice", Status: 1}))

	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "https://portal.example.com/", nil)
	r1.RemoteAddr = "198.51.100.10:1234"
	_, err := m.Session().IssueGrant(w1, r1, &dbdata.User{Username: "alice"}, "portal-jti-1")
	require.NoError(t, err)
	grant := findCookie(t, w1, grantCookieName)
	require.NotNil(t, grant)

	t.Run("不同IP被拒绝", func(t *testing.T) {
		w2 := httptest.NewRecorder()
		r2 := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
		r2.RemoteAddr = "203.0.113.77:1234"
		r2.AddCookie(grant)
		_, _, ok := m.Session().ExchangeGrant(w2, r2)
		assert.False(t, ok, "grant cookie 被带到另一 IP 时不得兑换")
		r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
		r.RemoteAddr = "198.51.100.10:9999"
		r.AddCookie(grant)
		_, _, ok = m.Session().ExchangeGrant(httptest.NewRecorder(), r)
		assert.True(t, ok, "同一客户端 IP 应可兑换，证明拒绝未误伤 grant")
	})
}

func TestRenewKeepsExternalUserWithoutDBRow(t *testing.T) {
	setupDB(t)

	m := testManager
	base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnSessionTTL = 60 })

	// 第三方用户未落库，token 由既有登录流程签发（含三方身份声明）
	iat := time.Now().Add(-31 * time.Minute).Unix()
	token, err := jwt.SetJwtData(map[string]any{
		"iat":           iat,
		"webvpn_user":   "feishu-user",
		"webvpn_type":   "feishu",
		"webvpn_groups": []string{"group-a"},
	}, time.Now().Add(time.Hour).Unix())
	require.NoError(t, err)

	r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	w := httptest.NewRecorder()

	renewed, err := m.Session().Renew(w, r)
	require.NoError(t, err)
	assert.True(t, renewed, "未落库的第三方用户会话应可续期")

	c := findCookie(t, w, sessionCookieName)
	require.NotNil(t, c, "续期应刷新会话 cookie")
	assert.True(t, c.MaxAge != 0 || c.Value != "", "会话 cookie 不应被清除")
}

func TestExchangeGrantConcurrentConsume(t *testing.T) {
	setupDB(t)

	m := testManager
	require.NoError(t, dbdata.GetDB().Insert(&dbdata.User{Username: "race-user", Status: 1}))
	portalToken, err := jwt.SetJwtData(map[string]any{}, time.Now().Add(time.Hour).Unix())
	require.NoError(t, err)
	portalJTI, err := jwt.JtiOf(portalToken)
	require.NoError(t, err)
	grantResp := httptest.NewRecorder()
	_, err = m.Session().IssueGrant(grantResp, httptest.NewRequest(http.MethodGet, "https://portal.example.com/", nil), &dbdata.User{Username: "race-user"}, portalJTI)
	require.NoError(t, err)
	grant := findCookie(t, grantResp, grantCookieName)
	require.NotNil(t, grant)

	const workers = 16
	start := make(chan struct{})
	results := make(chan bool, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			<-start
			r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
			r.AddCookie(&http.Cookie{Name: "portal_session", Value: portalToken})
			r.AddCookie(grant)
			_, _, ok := m.Session().ExchangeGrant(httptest.NewRecorder(), r)
			results <- ok
		})
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for ok := range results {
		if ok {
			successes++
		}
	}
	assert.Equal(t, 1, successes, "同一 grant 并发兑换只能成功一次")
}

func TestExchangeGrantConsumes(t *testing.T) {
	setupDB(t)

	m := testManager
	// 用独立用户名，避免受前序踢出测试的吊销阈值影响
	require.NoError(t, dbdata.GetDB().Insert(&dbdata.User{Username: "bob", Status: 1}))

	// 签发门户会话，再用其 jti 绑定 grant
	portalToken, err := jwt.SetJwtData(map[string]any{"portal_user": "bob"}, time.Now().Add(time.Hour).Unix())
	require.NoError(t, err)
	portalJTI, err := jwt.JtiOf(portalToken)
	require.NoError(t, err)
	w0 := httptest.NewRecorder()
	r0 := httptest.NewRequest(http.MethodGet, "https://portal.example.com/", nil)
	_, err = m.Session().IssueGrant(w0, r0, &dbdata.User{Username: "bob"}, portalJTI)
	require.NoError(t, err)
	grant := findCookie(t, w0, grantCookieName)
	require.NotNil(t, grant, "应写出 grant cookie")

	// 兑换成功：grant 必须携带匹配的门户会话
	r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
	r.AddCookie(&http.Cookie{Name: "portal_session", Value: portalToken})
	r.AddCookie(grant)
	w := httptest.NewRecorder()
	_, _, ok := m.Session().ExchangeGrant(w, r)
	assert.True(t, ok, "正常 grant 应能兑换")

	cleared := findCookie(t, w, grantCookieName)
	require.NotNil(t, cleared, "兑换成功后应清除 grant cookie")
	assert.Equal(t, -1, cleared.MaxAge, "清除指令应立即使 grant 过期")
}
