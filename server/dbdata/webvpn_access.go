package dbdata

import (
	"errors"
	"strings"
)

// WebVPN 应用访问方式
const (
	WebVpnAccessSubdomain = "subdomain" // 前置：https://<应用>.<webvpn_domain>/
	WebVpnAccessPath      = "path"      // 后置：https://<VPN 主域名>/web/<协议>/<应用>/
	WebVpnAccessBoth      = "both"
)

var errWebVpnInvalidAccessMode = errors.New("访问方式必须是 subdomain / path / both")

// 返回规范化访问方式，空视为 subdomain（兼容旧数据）
func (a *WebVpnApp) AccessModeOf() string {
	m := strings.ToLower(strings.TrimSpace(a.AccessMode))
	if m == "" {
		return WebVpnAccessSubdomain
	}
	return m
}

// 返回应用实际可用的访问方式。domainSet 表示是否配置了 webvpn_domain。
//   - 子域名：访问方式含 subdomain 且已配置域名
//   - 路径：访问方式含 path；未配置域名时非 HTTP 协议应用自动回落到路径方式
//     （HTTP 应用的路径方式与门户同源，存在安全取舍，必须由管理员显式选择）
func (a *WebVpnApp) AccessOf(domainSet bool) (subdomain, path bool) {
	m := a.AccessModeOf()
	subdomain = domainSet && (m == WebVpnAccessSubdomain || m == WebVpnAccessBoth)
	path = m == WebVpnAccessPath || m == WebVpnAccessBoth
	if !domainSet && !a.IsHTTP() {
		path = true
	}
	return
}

func normalizeWebVpnAccessMode(a *WebVpnApp) error {
	a.AccessMode = a.AccessModeOf()
	switch a.AccessMode {
	case WebVpnAccessSubdomain, WebVpnAccessPath, WebVpnAccessBoth:
		return nil
	}
	return errWebVpnInvalidAccessMode
}
