package webvpn

import (
	"net"
	"net/http"

	"github.com/wsczx/remlink/dbdata"
)

// 非 HTTP 协议（SSH/SFTP/…）应用的请求级授权：启用状态 + 用户/组白名单 + 来源 IP 白名单
// 与 Authorized 的区别：不做路径白名单校验（网关自有 /webvpn/* 端点不对应后端路径）
func AuthorizedUserIP(app *dbdata.WebVpnApp, user *dbdata.User, r *http.Request) bool {
	if app == nil || user == nil || app.Status != 1 {
		return false
	}
	if !dbdata.WebVpnUserAllowed(app, user) {
		return false
	}
	if len(app.IpAllowList) > 0 {
		ip := net.ParseIP(realClientIP(r))
		if ip == nil || !ipInAllowList(ip, app.IpAllowList) {
			return false
		}
	}
	return true
}
