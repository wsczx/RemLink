package admin

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/wsczx/remlink/dbdata"
)

// 证书解析信息
type certInfo struct {
	Slot          string    `json:"slot"` // main / wild
	SubjectCN     string    `json:"subject_cn"`
	Subject       string    `json:"subject"`
	SANs          []string  `json:"sans"`
	IPSANs        []string  `json:"ip_sans"`
	Issuer        string    `json:"issuer"`
	IssuerCN      string    `json:"issuer_cn"`
	IssuerOrg     string    `json:"issuer_org"`
	NotBefore     time.Time `json:"not_before"`
	NotAfter      time.Time `json:"not_after"`
	DaysRemaining int       `json:"days_remaining"` // 剩余天数（向下取整，已过期为负数）
	Expired       bool      `json:"expired"`
	SelfSigned    bool      `json:"self_signed"`
	ChainLength   int       `json:"chain_length"` // 服务端下发的证书链长度（含叶子证书）
	SerialNumber  string    `json:"serial_number"`
	SHA256        string    `json:"sha256"` // 叶子证书 DER 的 SHA256 指纹（大写、冒号分隔）
	KeyAlgorithm  string    `json:"key_algorithm"`
	KeyBits       int       `json:"key_bits"`
	SigAlgorithm  string    `json:"sig_algorithm"`
	Source        string    `json:"source"` // acme / upload / self_signed（按颁发者启发式推断，数据库未记录来源）
}

// 当前服务器正在使用的证书信息：主证书 + WebVPN 泛域名证书（若有）
func CertInfo(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{"main": nil, "wild": nil}

	// 主证书：取 SNI 表中实际对外提供的默认证书（与 TLS 握手 GetCertificateBySNI 同源）
	if mainCert := dbdata.CurrentDefaultCertificate(); mainCert == nil {
		resp["main_error"] = "主证书尚未加载"
	} else if info, err := buildCertInfo("main", mainCert); err != nil {
		resp["main_error"] = err.Error()
	} else {
		resp["main"] = info
	}

	// 泛域名证书：与服务启动/热更新时相同的加载函数
	wildCert, _, err := dbdata.ParseCertWild()
	if err != nil {
		resp["wild_error"] = err.Error()
	} else if wildCert != nil {
		if info, err := buildCertInfo("wild", wildCert); err != nil {
			resp["wild_error"] = err.Error()
		} else {
			resp["wild"] = info
		}
	}

	RespSucess(w, resp)
}

func buildCertInfo(slot string, c *tls.Certificate) (*certInfo, error) {
	if c == nil || len(c.Certificate) == 0 {
		return nil, fmt.Errorf("证书为空")
	}
	leaf := c.Leaf
	if leaf == nil {
		var err error
		leaf, err = x509.ParseCertificate(c.Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("解析证书失败: %w", err)
		}
	}

	now := time.Now()
	sum := sha256.Sum256(leaf.Raw)
	info := &certInfo{
		Slot:          slot,
		SubjectCN:     leaf.Subject.CommonName,
		Subject:       leaf.Subject.String(),
		SANs:          append([]string{}, leaf.DNSNames...),
		IPSANs:        []string{},
		Issuer:        leaf.Issuer.String(),
		IssuerCN:      leaf.Issuer.CommonName,
		IssuerOrg:     strings.Join(leaf.Issuer.Organization, ", "),
		NotBefore:     leaf.NotBefore,
		NotAfter:      leaf.NotAfter,
		DaysRemaining: int(math.Floor(leaf.NotAfter.Sub(now).Hours() / 24)),
		Expired:       now.After(leaf.NotAfter),
		SelfSigned:    isSelfSigned(leaf),
		ChainLength:   len(c.Certificate),
		SerialNumber:  formatHexColon(leaf.SerialNumber.Bytes()),
		SHA256:        formatHexColon(sum[:]),
		SigAlgorithm:  leaf.SignatureAlgorithm.String(),
	}
	for _, ip := range leaf.IPAddresses {
		info.IPSANs = append(info.IPSANs, ip.String())
	}
	switch k := leaf.PublicKey.(type) {
	case *rsa.PublicKey:
		info.KeyAlgorithm, info.KeyBits = "RSA", k.N.BitLen()
	case *ecdsa.PublicKey:
		info.KeyAlgorithm, info.KeyBits = "ECDSA", k.Curve.Params().BitSize
	case ed25519.PublicKey:
		info.KeyAlgorithm, info.KeyBits = "Ed25519", 256
	default:
		info.KeyAlgorithm = leaf.PublicKeyAlgorithm.String()
	}
	info.Source = guessCertSource(leaf, info.SelfSigned)
	return info, nil
}

// 自签：颁发者与主体一致且能用自身公钥验证签名
func isSelfSigned(c *x509.Certificate) bool {
	if !bytes.Equal(c.RawIssuer, c.RawSubject) {
		return false
	}
	return c.CheckSignatureFrom(c) == nil || c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature) == nil
}

// 数据库未记录证书来源，按颁发者推断：内置 ACME 客户端仅对接 Let's Encrypt
func guessCertSource(c *x509.Certificate, selfSigned bool) string {
	if selfSigned {
		return "self_signed"
	}
	org := strings.ToLower(strings.Join(c.Issuer.Organization, " "))
	if strings.Contains(org, "let's encrypt") {
		return "acme"
	}
	return "upload"
}

func formatHexColon(b []byte) string {
	if len(b) == 0 {
		return "00"
	}
	h := strings.ToUpper(hex.EncodeToString(b))
	var sb strings.Builder
	for i := 0; i < len(h); i += 2 {
		if i > 0 {
			sb.WriteByte(':')
		}
		sb.WriteString(h[i : i+2])
	}
	return sb.String()
}
