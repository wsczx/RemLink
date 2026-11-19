package handler

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/dbtest"
	"github.com/wsczx/remlink/service"
	"github.com/wsczx/remlink/sessdata"
	"github.com/wsczx/remlink/webvpn"
)

// 测试共享服务实例（生产代码已无全局，测试包内自建完整装配）
var (
	testSessions    *sessdata.SessionManager
	testAuthSession *AuthSessionManager
	testLocks       *auth.LockManager
	testWebvpn      *webvpn.WebVPNManager
	testSSO         *SSOHandler
	testWebAuth     *WebAuthService
	testLinkAuth    *LinkAuthService
	testPortal      *PortalService
	testServices    Services
)

func TestMain(m *testing.M) {
	auth.SetRegistry(auth.NewProviderRegistry())
	authsrv.RegisterProviders(auth.Registry)
	sessdata.SetPayloads(sessdata.NewBufferPool())

	testSessions = sessdata.NewSessionManager()
	testAuthSession = NewAuthSessionManager()
	testLocks = auth.NewLockManager()
	testWebvpn = webvpn.NewManager()
	testSSO = NewSSOHandler(testAuthSession)
	testWebAuth = NewWebAuthService(testAuthSession, testLocks, testWebvpn, testSSO)
	testLinkAuth = NewLinkAuthService(testSessions, testAuthSession, testLocks, testWebAuth)
	testPortal = NewPortalService(testSessions, testAuthSession, testLocks, testWebvpn, testSSO)
	testSSO.UsePortal(testPortal)
	testServices = Services{
		SyslogHub:    NewSyslogHub(),
		AccessAudit:  NewAccessAudit(testSessions),
		Sessions:     testSessions,
		Locks:        testLocks,
		Webvpn:       testWebvpn,
		AuthSessions: testAuthSession,
		Portal:       testPortal,
		LinkAuth:     testLinkAuth,
		WebAuth:      testWebAuth,
		SSO:          testSSO,
	}
	service.SetActions(service.ProductionActions(testSessions))
	os.Exit(m.Run())
}

func TestLinkAuth_AuthCert(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)

	body := `<?xml version="1.0" encoding="UTF-8"?><config-auth client="vpn" type="auth-reply"><auth><username>test</username><password>test</password></auth><group-select>default</group-select></config-auth>`

	pt := &dbdata.Policy{Name: "cert-auth-policy", Status: 1, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}}
	_ = dbdata.Policies.SetPolicy(pt)

	t.Run("CertOnly_NoTLS", func(t *testing.T) {
		g := dbdata.Group{
			Name:        "default",
			Status:      1,
			PolicyId:    pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"cert"}]}`),
		}
		_ = dbdata.Groups.SetGroup(&g)

		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		req.Header.Set("User-Agent", "cisco anyconnect vpn agent")
		req.Header.Set("X-Aggregate-Auth", "1")
		req.Header.Set("X-Transcend-Version", "1")
		w := httptest.NewRecorder()
		testLinkAuth.Handle(w, req)

		if w.Code != http.StatusOK {
			t.Error("expected 200 with auth error page, got", w.Code)
		}
	})

	t.Run("CertOnly_NoOU", func(t *testing.T) {
		g := dbdata.Group{
			Name:        "default",
			Status:      1,
			PolicyId:    pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"cert"}]}`),
		}
		_ = dbdata.Groups.SetGroup(&g)

		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		req.Header.Set("User-Agent", "cisco anyconnect vpn agent")
		req.Header.Set("X-Aggregate-Auth", "1")
		req.Header.Set("X-Transcend-Version", "1")
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{{
				Subject: pkix.Name{
					CommonName:         "",
					OrganizationalUnit: []string{""},
				},
			}},
		}
		w := httptest.NewRecorder()
		testLinkAuth.Handle(w, req)

		if w.Code != http.StatusOK {
			t.Error("expected 200 with auth error page, got", w.Code)
		}
	})

	t.Run("CertFail", func(t *testing.T) {
		g := dbdata.Group{
			Name:        "default",
			Status:      1,
			PolicyId:    pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"cert"},{"type":"local"}]}`),
		}
		_ = dbdata.Groups.SetGroup(&g)

		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		req.Header.Set("User-Agent", "cisco anyconnect vpn agent")
		req.Header.Set("X-Aggregate-Auth", "1")
		req.Header.Set("X-Transcend-Version", "1")
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{{
				Subject: pkix.Name{
					CommonName:         "",
					OrganizationalUnit: []string{""},
				},
			}},
		}
		w := httptest.NewRecorder()
		testLinkAuth.Handle(w, req)

		// cert 失败直接终止管道，local 步骤不会执行
		if w.Code != http.StatusOK {
			t.Error("expected 200 with auth error page, got", w.Code)
		}
	})
}
