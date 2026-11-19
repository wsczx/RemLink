package dbdata

import (
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/pion/dtls/v3/pkg/crypto/selfsign"
)

// 启动/换主证书必须经 LoadCertificate：既注册 default（IP 直连无 SNI 时返回主证书）
// 又登记认证 XML 下发的 SHA1 指纹。泛域名证书用 LoadCertificates 追加，不得清空二者
func TestMainCertLoadSetsDefaultAndHash(t *testing.T) {
	mainCert, err := selfsign.GenerateSelfSignedWithDNS("vpn.example.com")
	if err != nil {
		t.Fatal(err)
	}
	wildCert, err := selfsign.GenerateSelfSignedWithDNS("*.apps.example.com")
	if err != nil {
		t.Fatal(err)
	}

	// 保存全局状态，避免污染同包其它测试
	ntcMux.Lock()
	saved := make(map[string]*tls.Certificate, len(nameToCertificate))
	for k, v := range nameToCertificate {
		saved[k] = v
	}
	ntcMux.Unlock()
	savedHash := GetMainCertHash()
	t.Cleanup(func() {
		ntcMux.Lock()
		nameToCertificate = saved
		ntcMux.Unlock()
		mainCertHash.Store(savedHash)
	})

	Certs.LoadCertificate(&mainCert)

	want := sha1.Sum(mainCert.Certificate[0])
	if got := GetMainCertHash(); got != strings.ToUpper(hex.EncodeToString(want[:])) {
		t.Fatalf("主证书指纹未登记: got %q", got)
	}
	// 任意未匹配的 SNI（含 IP 直连时的空 SNI）都应回落到主证书
	gotCert, err := Certs.GetCertificateBySNI("192.168.1.10")
	if err != nil {
		t.Fatal(err)
	}
	if gotCert != &mainCert {
		t.Fatal("default 未指向主证书，IP 直连会拿到随机临时证书")
	}

	// 追加泛域名证书后，default 与指纹保持不变
	Certs.LoadCertificates([]*tls.Certificate{&wildCert})
	if got := GetMainCertHash(); got != strings.ToUpper(hex.EncodeToString(want[:])) {
		t.Fatalf("追加泛域名证书篡改了主证书指纹: got %q", got)
	}
	if c, err := Certs.GetCertificateBySNI("192.168.1.10"); err != nil || c != &mainCert {
		t.Fatal("追加泛域名证书后 default 不再是主证书")
	}
	if c, err := Certs.GetCertificateBySNI("app.apps.example.com"); err != nil || c != &wildCert {
		t.Fatal("泛域名证书未生效")
	}

	// 即使指纹登记环节被绕过，也应从 default 现算而非返回空串
	mainCertHash.Store("")
	if got := GetMainCertHash(); got != strings.ToUpper(hex.EncodeToString(want[:])) {
		t.Fatalf("兜底未生效，指纹为空会导致认证 XML 下发空值: got %q", got)
	}
}
