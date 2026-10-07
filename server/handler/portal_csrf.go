package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/wsczx/remlink/webvpn"
)

// 门户前端为所有门户接口请求统一附带的自定义头。跨站页面无法在不触发 CORS 预检的情况下携带自定义头，
// 结合 portal_session 的 SameSite=Lax，可阻断跨站表单 / 简单请求形式的 CSRF
const portalCSRFHeader = "X-RemLink-Portal"

// 门户写接口（/api/v1/portal/* 的非 GET/HEAD/OPTIONS 请求）CSRF 校验：
//  1. 必须携带 X-RemLink-Portal 头
//  2. 带 Origin 时必须与当前 Host 同源；Sec-Fetch-Site 只接受 same-origin / none
//  3. 拒绝来自路径模式被代理 HTTP 应用页面（Referer 位于 /web/http/ 下）的请求
func portalWriteAllowed(r *http.Request) bool {
	if r.Header.Get(portalCSRFHeader) == "" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) {
			return false
		}
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
	default:
		return false
	}
	return !webvpn.FromProxiedPage(r)
}

func portalCSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, portalAPIPrefix) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				if !portalWriteAllowed(r) {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
