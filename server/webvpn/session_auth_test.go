package webvpn

import (
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

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
