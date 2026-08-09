package webvpn

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

func setupCookieTest(t *testing.T) {
	t.Helper()
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.WebVpnDomain = "wv.example.com"
		c.JwtSecret = "unit-test-secret"
	})
}

func findCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestIssueSessionCookieDomain(t *testing.T) {
	setupCookieTest(t)
	m := GetManager()
	require.NotNil(t, m)

	r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
	w := httptest.NewRecorder()

	g, err := m.Session().Issue(w, r, &dbdata.User{Username: "alice"}, time.Now().Unix())
	require.NoError(t, err)
	require.NotEmpty(t, g)

	ck := findCookie(t, w, sessionCookieName)
	require.NotNil(t, ck, "应写入 webvpn_session cookie")
	assert.Equal(t, "example.com", ck.Domain, "webvpn_session 域须与 ClearCookie 一致，否则子域免登或清登录残留")
	assert.Equal(t, "/", ck.Path)
	assert.True(t, ck.HttpOnly)
	assert.True(t, ck.Secure)
}

func TestIssueGrantCookieDomain(t *testing.T) {
	setupCookieTest(t)
	m := GetManager()
	require.NotNil(t, m)

	r := httptest.NewRequest(http.MethodGet, "https://portal.example.com/", nil)
	w := httptest.NewRecorder()

	g, err := m.Session().IssueGrant(w, r, &dbdata.User{Username: "alice"}, "portal-jti-1")
	require.NoError(t, err)
	require.NotEmpty(t, g)

	ck := findCookie(t, w, grantCookieName)
	require.NotNil(t, ck, "应写入 webvpn_grant cookie")
	assert.Equal(t, "example.com", ck.Domain, "grant 必须通配，否则门户子域拿不到授权")
	assert.True(t, ck.HttpOnly)
	assert.True(t, ck.Secure)
}

func TestClearGrantCookieDomain(t *testing.T) {
	setupCookieTest(t)
	m := GetManager()
	require.NotNil(t, m)

	r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
	w := httptest.NewRecorder()

	m.Session().ClearGrantCookie(w, r)
	ck := findCookie(t, w, grantCookieName)
	require.NotNil(t, ck)
	assert.Equal(t, "example.com", ck.Domain)
	assert.Equal(t, -1, ck.MaxAge, "清 cookie 应立即使其过期")
}

func TestClearSessionCookieDomain(t *testing.T) {
	setupCookieTest(t)
	m := GetManager()
	require.NotNil(t, m)

	r := httptest.NewRequest(http.MethodGet, "https://app.wv.example.com/", nil)
	w := httptest.NewRecorder()

	m.Session().ClearCookie(w, r)
	ck := findCookie(t, w, sessionCookieName)
	require.NotNil(t, ck)
	assert.Equal(t, "example.com", ck.Domain)
	assert.Equal(t, -1, ck.MaxAge)
}

func TestClearSessionCookieOnIP(t *testing.T) {
	setupCookieTest(t)
	m := GetManager()
	require.NotNil(t, m)

	r := httptest.NewRequest(http.MethodGet, "https://192.168.1.10/", nil)
	w := httptest.NewRecorder()

	m.Session().ClearCookie(w, r)
	ck := findCookie(t, w, sessionCookieName)
	require.NotNil(t, ck)
	assert.Equal(t, "", ck.Domain, "IP 访问清 cookie 应精确 host")
}
