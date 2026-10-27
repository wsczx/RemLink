package handler

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/dbtest"
)

func createWebAuthSession(_ *testing.T, groupName string) string {
	state := GenerateSessionID()

	pending := &AuthSession{
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{GroupName: groupName},
		},
		UserActLog: &dbdata.UserActLog{
			GroupName: groupName,
		},
	}
	AuthSessionManager.Save(state, pending)
	return state
}

func TestWebAuthSPLogin_Valid(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	base.UpdateCfg(func(c *base.ServerConfig) { c.EnableWebAuth = true })

	state := createWebAuthSession(t, "default")

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/sp/login?state="+state, nil)
	w := httptest.NewRecorder()

	WebAuthSPLogin(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	ast.Contains(location, "/ui/#/web-auth?state=")
	ast.Contains(location, state)
}

func TestWebAuthSPLogin_InvalidState(t *testing.T) {
	base.Test()

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/sp/login?state=nonexistent", nil)
	w := httptest.NewRecorder()

	WebAuthSPLogin(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusBadRequest, w.Code)
}

func TestWebAuthSPLogin_EmptyState(t *testing.T) {
	base.Test()

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/sp/login", nil)
	w := httptest.NewRecorder()

	WebAuthSPLogin(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusBadRequest, w.Code)
}

func TestWebAuthStart_Success(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	base.UpdateCfg(func(c *base.ServerConfig) { c.EnableWebAuth = true })

	pt := &dbdata.Policy{
		Name:      "plcy-web",
		ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}},
		Status:    1,
	}
	err := dbdata.SetPolicy(pt)
	assert.New(t).Nil(err)

	g := &dbdata.Group{
		Name:        "webauth-group",
		Status:      1,
		PolicyId:    pt.Id,
		AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`),
	}
	err = dbdata.SetGroup(g)
	assert.New(t).Nil(err)

	state := createWebAuthSession(t, "webauth-group")

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/start?state="+state, nil)
	w := httptest.NewRecorder()

	WebAuthStart(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("select_group", resp["status"])
	ast.NotNil(resp["groups"], "should return group list")
}

func TestWebAuthStart_EmptyState(t *testing.T) {
	base.Test()
	base.SetCfgForTest(&base.ServerConfig{EnableWebAuth: true})

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/start", nil)
	w := httptest.NewRecorder()

	WebAuthStart(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("error", resp["status"])
}

func TestWebAuthStart_InvalidState(t *testing.T) {
	base.Test()
	base.SetCfgForTest(&base.ServerConfig{EnableWebAuth: true})

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/start?state=expired-session", nil)
	w := httptest.NewRecorder()

	WebAuthStart(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("error", resp["status"])
}

func TestWebAuthStart_Disabled(t *testing.T) {
	base.Test()
	base.SetCfgForTest(&base.ServerConfig{EnableWebAuth: false})

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/start?state=any", nil)
	w := httptest.NewRecorder()

	WebAuthStart(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusNotFound, w.Code)
}

func TestWebAuthSelectGroup_Success(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)

	ast := assert.New(t)

	pt := &dbdata.Policy{
		Name:      "plcy-sel",
		ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}},
		Status:    1,
	}
	err := dbdata.SetPolicy(pt)
	ast.Nil(err)

	g := &dbdata.Group{
		Name:        "select-group",
		Status:      1,
		PolicyId:    pt.Id,
		AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`),
	}
	err = dbdata.SetGroup(g)
	ast.Nil(err)

	state := createWebAuthSession(t, "select-group")

	body := `{"group":"select-group","username":"testuser"}`
	req := httptest.NewRequest("POST", "/+CSCOE+/web-auth/select-group?state="+state, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	WebAuthSelectGroup(w, req)

	ast.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("credentials", resp["status"], "should return credentials for local auth first step")
	ast.Equal("请输入登录凭据", resp["hint"])
}

func TestWebAuthSelectGroup_InvalidState(t *testing.T) {
	base.Test()

	body := `{"group":"any","username":"test"}`
	req := httptest.NewRequest("POST", "/+CSCOE+/web-auth/select-group?state=expired", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	WebAuthSelectGroup(w, req)

	ast := assert.New(t)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("error", resp["status"])
}

func TestWebAuthSelectGroup_EmptyState(t *testing.T) {
	base.Test()

	body := `{"group":"any"}`
	req := httptest.NewRequest("POST", "/+CSCOE+/web-auth/select-group", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	WebAuthSelectGroup(w, req)

	ast := assert.New(t)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("error", resp["status"])
}

func TestWebAuthSelectGroup_GroupNotFound(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)

	state := createWebAuthSession(t, "default")

	body := `{"group":"nonexistent-group","username":"test"}`
	req := httptest.NewRequest("POST", "/+CSCOE+/web-auth/select-group?state="+state, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	WebAuthSelectGroup(w, req)

	ast := assert.New(t)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("error", resp["status"])
}

func TestWebAuthSelectGroup_EmptyBody(t *testing.T) {
	base.Test()

	body := ``
	req := httptest.NewRequest("POST", "/+CSCOE+/web-auth/select-group?state=any", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	WebAuthSelectGroup(w, req)

	ast := assert.New(t)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("error", resp["status"])
}

func TestWebAuthStep_InvalidState(t *testing.T) {
	base.Test()

	body := `{"username":"test","password":"test"}`
	req := httptest.NewRequest("POST", "/+CSCOE+/web-auth/step?state=expired", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	WebAuthStep(w, req)

	ast := assert.New(t)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("error", resp["status"])
}

func TestWebAuthStep_EmptyState(t *testing.T) {
	base.Test()

	body := `{"username":"test","password":"test"}`
	req := httptest.NewRequest("POST", "/+CSCOE+/web-auth/step", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	WebAuthStep(w, req)

	ast := assert.New(t)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("error", resp["status"])
}

func TestWebAuthComplete_InvalidState(t *testing.T) {
	base.Test()

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/complete?state=expired", nil)
	w := httptest.NewRecorder()

	WebAuthComplete(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusBadRequest, w.Code)
}

func TestWebAuthComplete_EmptyState(t *testing.T) {
	base.Test()

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/complete", nil)
	w := httptest.NewRecorder()

	WebAuthComplete(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusBadRequest, w.Code)
}

func TestWebAuthComplete_Success(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)

	state := createWebAuthSession(t, "default")

	sess, err := AuthSessionManager.Get(state)
	assert.New(t).Nil(err)
	sess.Ctx.GetSSO().WebAuthCompleted = true
	AuthSessionManager.Save(state, sess)

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/complete?state="+state, nil)
	w := httptest.NewRecorder()

	WebAuthComplete(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusFound, w.Code)

	location := w.Header().Get("Location")
	ast.Contains(location, "saml_ac_login.html")
	// 回归：2026-08-08 移除了 OpenConnect 专用的 ?oc=1&token= 重定向分支，
	// 成功页应直接 302 到 saml_ac_login.html（token 在 Cookie 中），不再带 oc=1
	ast.NotContains(location, "oc=1", "不应再带 OpenConnect 专用 oc=1 参数")

	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "acSamlv2Token" && c.Value != "" {
			found = true
			break
		}
	}
	ast.True(found, "should set acSamlv2Token cookie")
}

// ---- 以下为 2026-08-08 修复（WebAuth 按需检测证书 + 组过滤预填）的补充测试 ----

func createWebAuthSessionWithCert(t *testing.T, groupName, certCN, certOU string) string {
	state := GenerateSessionID()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: certCN, OrganizationalUnit: []string{certOU}},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	pending := &AuthSession{
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{
				GroupName: groupName,
				TLS: &tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{leaf},
				},
			},
		},
		UserActLog: &dbdata.UserActLog{GroupName: groupName},
	}
	AuthSessionManager.Save(state, pending)
	return state
}

// 证书守卫：无 cert 组时即使会话含证书也不触发证书自动认证，
// 且不应把证书 CN 写入响应（防用户名锁死）
func TestWebAuthStart_CertGuardDisabled(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	defer dbdata.InvalidateCertAuthCache()
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableWebAuth = true
		c.EnableWebAuthGroupFilter = false
	})

	pt := &dbdata.Policy{Name: "plcy-guard", ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}, Status: 1}
	assert.New(t).Nil(dbdata.SetPolicy(pt))
	assert.New(t).Nil(dbdata.SetGroup(&dbdata.Group{
		Name: "guard-group", Status: 1, PolicyId: pt.Id,
		AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`),
	}))
	dbdata.InvalidateCertAuthCache()
	assert.False(t, dbdata.AnyGroupHasCertAuth(), "无 cert 组应返回 false")

	state := createWebAuthSessionWithCert(t, "guard-group", "test", "guard-group")

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/start?state="+state, nil)
	w := httptest.NewRecorder()
	WebAuthStart(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	// 守卫关闭：证书未被消费，应回退组选择而非被证书 CN 覆盖
	ast.Equal("select_group", resp["status"])
	ast.Nil(resp["username"], "无 cert 组时不应回传证书 CN，避免锁死")
}

// 证书守卫开启但 auto-login 失败：应回退组选择并提示证书认证失败，
// 且清空临时写入的证书 CN
func TestWebAuthStart_CertAutoAuthFallback(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	defer dbdata.InvalidateCertAuthCache()
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableWebAuth = true
		c.EnableWebAuthGroupFilter = false
	})

	// 建一个纯 cert 首步组（触发 AnyGroupHasCertAuth()==true 且 CertAutoAuth 返回 true），
	// 但测试用的自签名证书未登记到签发记录 → 证书认证失败 → 验证回退与清空逻辑
	pt := &dbdata.Policy{Name: "plcy-cfall", ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}, Status: 1}
	assert.New(t).Nil(dbdata.SetPolicy(pt))
	assert.New(t).Nil(dbdata.SetGroup(&dbdata.Group{
		Name: "cert-fallback", Status: 1, PolicyId: pt.Id,
		// 纯 cert 首步组：CertAutoAuth 返回 true，但自签名证书未登记 → auto-login 失败回退
		AuthProfile: json.RawMessage(`{"step":[{"type":"cert"}]}`),
	}))
	dbdata.InvalidateCertAuthCache()
	assert.True(t, dbdata.AnyGroupHasCertAuth(), "含 cert 组应返回 true")

	state := createWebAuthSessionWithCert(t, "cert-fallback", "test", "cert-fallback")

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/start?state="+state, nil)
	w := httptest.NewRecorder()
	WebAuthStart(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("select_group", resp["status"])
	ast.Contains(resp["message"], "证书自动认证失败")
	ast.Nil(resp["username"], "证书 auto-login 失败后不应残留证书 CN")
}

// 组过滤开启：预填已识别用户名，避免重复输入
func TestWebAuthSelectGroup_GroupFilterPrefill(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableWebAuth = true
		c.EnableWebAuthGroupFilter = true
	})

	pt := &dbdata.Policy{Name: "plcy-prefill", ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}, Status: 1}
	assert.New(t).Nil(dbdata.SetPolicy(pt))
	assert.New(t).Nil(dbdata.SetGroup(&dbdata.Group{
		Name: "prefill-group", Status: 1, PolicyId: pt.Id,
		AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`),
	}))

	state := GenerateSessionID()
	AuthSessionManager.Save(state, &AuthSession{
		Ctx:        &auth.Context{Conn: auth.ConnInfo{GroupName: "prefill-group", Username: "alice"}},
		UserActLog: &dbdata.UserActLog{GroupName: "prefill-group", Username: "alice"},
	})

	body := `{"group":"prefill-group","username":"alice"}`
	req := httptest.NewRequest("POST", "/+CSCOE+/web-auth/select-group?state="+state, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	WebAuthSelectGroup(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("credentials", resp["status"])
	ast.Equal("alice", resp["username"], "组过滤模式应预填用户名")
}

// 组过滤关闭：即使会话残留用户名也不预填，避免输入框锁死
func TestWebAuthSelectGroup_NoPrefillWhenFilterOff(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableWebAuth = true
		c.EnableWebAuthGroupFilter = false
	})

	pt := &dbdata.Policy{Name: "plcy-noprefill", ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}, Status: 1}
	assert.New(t).Nil(dbdata.SetPolicy(pt))
	assert.New(t).Nil(dbdata.SetGroup(&dbdata.Group{
		Name: "noprefill-group", Status: 1, PolicyId: pt.Id,
		AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`),
	}))

	// 会话残留用户名（模拟证书 CN 污染：test）
	state := GenerateSessionID()
	AuthSessionManager.Save(state, &AuthSession{
		Ctx:        &auth.Context{Conn: auth.ConnInfo{GroupName: "noprefill-group", Username: "test"}},
		UserActLog: &dbdata.UserActLog{GroupName: "noprefill-group", Username: "test"},
	})

	body := `{"group":"noprefill-group","username":"test"}`
	req := httptest.NewRequest("POST", "/+CSCOE+/web-auth/select-group?state="+state, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	WebAuthSelectGroup(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("credentials", resp["status"])
	ast.Nil(resp["username"], "组过滤关闭时不应预填残留用户名（防锁死）")
}

func TestWebAuthOnPass_NilCtxRejected(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)

	ast := assert.New(t)

	pending := &AuthSession{} // Ctx 为 nil
	result := &auth.PipelineResult{Username: "u1", GroupName: "g1"}
	req := httptest.NewRequest("POST", "/+CSCOE+/web-auth/step", nil)
	w := httptest.NewRecorder()

	webAuthOnPass(w, req, "nil-ctx-state", pending, result)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("error", resp["status"], "nil Ctx 会话必须显式失败")
	ast.Nil(pending.Ctx, "不得把 nil Ctx 修复为空 Context（异常态应保持失败）")

	// 不得签发门户 JWT cookie
	for _, c := range w.Result().Cookies() {
		ast.NotEqual(portalCookieName, c.Name, "nil Ctx 会话不得签发门户 JWT")
	}
}

func TestWebAuthContinue_NilCtxRejected(t *testing.T) {
	base.Test()

	ast := assert.New(t)

	state := GenerateSessionID()
	AuthSessionManager.Save(state, &AuthSession{}) // Ctx 为 nil

	req := httptest.NewRequest("GET", "/+CSCOE+/web-auth/continue?state="+state, nil)
	w := httptest.NewRecorder()
	WebAuthContinue(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal("error", resp["status"], "nil Ctx 会话必须显式失败")
	ast.Contains(resp["message"], "已过期")

	// 残留会话应被清理，不可再被驱动
	_, err := AuthSessionManager.Get(state)
	ast.Error(err, "被拒的 nil Ctx 会话应被删除")
}

// WebAuth SSO 回调必须拒绝跨组冒用
func TestWebAuthSSOCallbackRejectsCrossGroup(t *testing.T) {
	base.Test()
	base.SetCfgForTest(&base.ServerConfig{DisplayError: true})
	dbtest.SetupDB(t)
	ast := assert.New(t)

	// 注册测试用 SSO 源；按组名返回回调，便于断言访问控制用的是哪一组配置
	ssoCallbacks["testxgroup"] = func(groupName string) (ssoCallback, error) {
		return &stubSSOCallbackX{group: groupName}, nil
	}
	defer delete(ssoCallbacks, "testxgroup")

	const remote = "10.9.9.9:5555"

	// A 组的 sso_state：授权结果将按 A 组配置做部门/拒绝名单校验
	ssoState := "sso-state-group-a"
	AuthSessionManager.Save(ssoState, &AuthSession{
		Kind: AuthKindSSOPending,
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{GroupName: "group-a", RemoteAddr: remote},
			SSO:  &auth.SSOState{Type: "testxgroup", From: "web_auth", ClientIP: remote},
		},
	})

	// B 组的 WebAuth 管道会话（攻击目标）
	webState := "web-state-group-b"
	AuthSessionManager.Save(webState, &AuthSession{
		Kind: AuthKindPipeline,
		Ctx: &auth.Context{
			WebAuth: true,
			Conn:    auth.ConnInfo{GroupName: "group-b", RemoteAddr: remote},
		},
	})

	req := httptest.NewRequest("GET",
		"/web-auth/sso-callback?web_state="+webState+"&sso_state="+ssoState+"&code=fake", nil)
	req.RemoteAddr = remote
	w := httptest.NewRecorder()
	WebAuthSSOCallback(w, req)

	// 必须拒绝，且不得把 B 组会话标记为已认证
	ast.Equal(http.StatusBadRequest, w.Code, "跨组回调必须被拒绝")

	webPending, err := AuthSessionManager.Get(webState)
	ast.NoError(err)
	ast.False(webPending.Ctx.SSO != nil && webPending.Ctx.SSO.Authenticated,
		"跨组回调不得把目标会话标记为已认证")

	// sso_state 应被消费，防重放
	_, err = AuthSessionManager.Get(ssoState)
	ast.Error(err, "sso_state 应在拒绝后被清理")
}

// sso_state 必须是 SSO 待回调会话，其它来源（如 OTP 等待的管道会话）不得复用
func TestWebAuthSSOCallbackRejectsNonSsoState(t *testing.T) {
	base.Test()
	base.SetCfgForTest(&base.ServerConfig{DisplayError: true})
	dbtest.SetupDB(t)
	ast := assert.New(t)

	const remote = "10.9.9.9:5555"

	// 把「等待 OTP」的管道会话 id 当作 sso_state 重放
	pipelineState := "pipeline-otp-pending"
	AuthSessionManager.Save(pipelineState, &AuthSession{
		Kind: AuthKindPipeline,
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{GroupName: "group-a", RemoteAddr: remote},
		},
	})
	webState := "web-state-for-nonsso"
	AuthSessionManager.Save(webState, &AuthSession{
		Kind: AuthKindPipeline,
		Ctx:  &auth.Context{Conn: auth.ConnInfo{GroupName: "group-a", RemoteAddr: remote}},
	})

	req := httptest.NewRequest("GET",
		"/web-auth/sso-callback?web_state="+webState+"&sso_state="+pipelineState+"&code=fake", nil)
	req.RemoteAddr = remote
	w := httptest.NewRecorder()
	WebAuthSSOCallback(w, req)

	ast.Equal(http.StatusBadRequest, w.Code, "非 SSO 来源的 sso_state 必须被拒绝")

	// 被冒用的管道会话不得被消费/删除
	_, err := AuthSessionManager.Get(pipelineState)
	ast.NoError(err, "被冒用的管道会话不应被消费")
}

type stubSSOCallbackX struct{ group string }

func (s *stubSSOCallbackX) getUser(code string) (string, string, error) {
	return "attacker", "攻击者", nil
}

func (s *stubSSOCallbackX) checkDept(userID string) error { return nil }

func (s *stubSSOCallbackX) isBlocked(userID string) bool { return false }
