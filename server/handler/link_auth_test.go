package handler

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

func TestLinkAuth_AuthCert(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()

	body := `<?xml version="1.0" encoding="UTF-8"?><config-auth client="vpn" type="auth-reply"><auth><username>test</username><password>test</password></auth><group-select>default</group-select></config-auth>`

	pt := &dbdata.Policy{Name: "cert-auth-policy", Status: 1, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}}
	_ = dbdata.SetPolicy(pt)

	t.Run("CertOnly_NoTLS", func(t *testing.T) {
		g := dbdata.Group{
			Name:        "default",
			Status:      1,
			PolicyId:    pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"cert"}]}`),
		}
		_ = dbdata.SetGroup(&g)

		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		req.Header.Set("User-Agent", "cisco anyconnect vpn agent")
		req.Header.Set("X-Aggregate-Auth", "1")
		req.Header.Set("X-Transcend-Version", "1")
		w := httptest.NewRecorder()
		LinkAuth(w, req)

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
		_ = dbdata.SetGroup(&g)

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
		LinkAuth(w, req)

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
		_ = dbdata.SetGroup(&g)

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
		LinkAuth(w, req)

		// cert 失败直接终止管道，local 步骤不会执行
		if w.Code != http.StatusOK {
			t.Error("expected 200 with auth error page, got", w.Code)
		}
	})
}
