package webvpn

import (
	"net"
	"strings"

	"github.com/wsczx/remlink/base"
)

const sessionCookieName = "webvpn_session"

const grantCookieName = "webvpn_grant"

// 去掉 host:port 中的端口部分，处理 [IPv6]:port。
func stripPort(host string) string {
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		if strings.Contains(host, "]") {
			return host[:strings.LastIndexByte(host, ':')]
		}
		return host[:i]
	}
	return host
}

// 返回注册域的最后两段（如 example.com）。
func base2Domain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = stripPort(domain)
	if domain == "" {
		return ""
	}
	parts := strings.Split(strings.TrimSuffix(domain, "."), ".")
	if len(parts) < 2 {
		return domain
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// WebVPN 会话/授权 cookie 的 Domain：属于 WebVpnDomain 注册域时按 .base2 通配，否则返回空。
func CookieDomain(host string) string {
	domain := base.GetCfg().WebVpnDomain
	if domain == "" {
		return ""
	}
	host = stripPort(host)
	if net.ParseIP(host) != nil {
		return ""
	}
	base2 := base2Domain(domain)
	if base2 == "" {
		return ""
	}
	if host == base2 || strings.HasSuffix(host, "."+base2) {
		return "." + base2
	}
	return ""
}

// 返回 WebVpnDomain 对应的通配域（.base2），未配置返回空。
func wildcardDomain() string {
	base2 := base2Domain(base.GetCfg().WebVpnDomain)
	if base2 == "" {
		return ""
	}
	return "." + base2
}
