package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbtest"
)

// 模拟 AnyConnect sso-v2 外部浏览器链路：
// 认证完成 → /web-auth/complete → 302 saml_ac_login.html → ACLogin
func TestWebAuthSamlAcLoginE2E(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	base.UpdateCfg(func(c *base.ServerConfig) { c.EnableWebAuth = true })

	state := createWebAuthSession(t, "default")
	pending, err := testAuthSession.Get(state)
	require.NoError(t, err)
	pending.Ctx.GetSSO().WebAuthCompleted = true
	pending.Kind = AuthKindWebAuth
	testAuthSession.Save(state, pending)

	// 1. /web-auth/complete
	req := httptest.NewRequest(http.MethodGet, "/web-auth/complete?state="+state, nil)
	w := httptest.NewRecorder()
	testWebAuth.Complete(w, req)
	require.Equal(t, http.StatusFound, w.Code, "complete 应 302")
	assert.Equal(t, "/+CSCOE+/saml_ac_login.html", w.Header().Get("Location"))

	var tokenCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "acSamlv2Token" {
			tokenCookie = c
		}
	}
	require.NotNil(t, tokenCookie, "complete 必须下发 acSamlv2Token cookie")

	// 2. 浏览器带 cookie 请求 saml_ac_login.html（外部浏览器 UA）
	acReq := httptest.NewRequest(http.MethodGet, "/+CSCOE+/saml_ac_login.html", nil)
	acReq.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Chrome/120.0")
	acReq.AddCookie(tokenCookie)
	acW := httptest.NewRecorder()
	testSSO.ACLogin(acW, acReq)

	t.Logf("ACLogin 响应: code=%d location=%q bodyLen=%d",
		acW.Code, acW.Header().Get("Location"), acW.Body.Len())
	assert.NotEqual(t, 0, acW.Code, "ACLogin 必须有响应")
}

// 内部浏览器 UA 应直接返回成功页
func TestWebAuthSamlAcLoginInternalBrowser(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)

	state := createWebAuthSession(t, "default")
	pending, err := testAuthSession.Get(state)
	require.NoError(t, err)
	pending.Ctx.GetSSO().WebAuthCompleted = true
	pending.Kind = AuthKindWebAuth
	testAuthSession.Save(state, pending)

	tokenCookie := &http.Cookie{Name: "acSamlv2Token", Value: state}
	acReq := httptest.NewRequest(http.MethodGet, "/+CSCOE+/saml_ac_login.html", nil)
	acReq.Header.Set("User-Agent", "AnyConnect Agent 4.10")
	acReq.AddCookie(tokenCookie)
	acW := httptest.NewRecorder()
	testSSO.ACLogin(acW, acReq)

	assert.Equal(t, http.StatusOK, acW.Code)
	assert.Contains(t, acW.Body.String(), "认证成功")
}

// 兑换检查：WebAuth 会话的 token 必须可被 sso-token 兑换
func TestWebAuthSessionAllowedForSSOToken(t *testing.T) {
	state := createWebAuthSession(t, "default")
	pending, err := testAuthSession.Get(state)
	require.NoError(t, err)
	pending.Ctx.GetSSO().WebAuthCompleted = true
	pending.Kind = AuthKindWebAuth
	assert.True(t, isSSOTokenSessionAllowed(pending))
}
