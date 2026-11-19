package dbdata

import "testing"

// 自签默认证书的 SAN 应包含本机 IP，保证未配置商业证书时 IP 直连可通过 SAN 校验
func TestLocalIPAddresses(t *testing.T) {
	ips := localIPAddresses()
	if len(ips) < 2 {
		t.Fatalf("应至少包含 IPv4/IPv6 回环地址，实际 %d 个", len(ips))
	}
	foundV4, foundV6 := false, false
	for _, ip := range ips {
		switch ip.String() {
		case "127.0.0.1":
			foundV4 = true
		case "::1":
			foundV6 = true
		}
	}
	if !foundV4 || !foundV6 {
		t.Fatalf("回环地址缺失: v4=%v v6=%v", foundV4, foundV6)
	}
}
