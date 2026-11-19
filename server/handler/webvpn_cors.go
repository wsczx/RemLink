package handler

import (
	"github.com/wsczx/remlink/dbdata"
	"net/http"
	"strings"
)

// 写入跨域响应头；w.Header() 与反代 resp.Header 共用同一实现
func setCORSHeaders(h http.Header, origin string) {
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Credentials", "true")
	h.Set("Vary", "Origin")
}

func webVpnCrossOriginAllowed(r *http.Request, app *dbdata.WebVpnApp) bool {
	origin := r.Header.Get("Origin")
	return origin != "" && !webVpnSameOrigin(r) && dbdata.WebVpn.CorsOriginAllowed(app, origin)
}

func webVpnWriteCrossOriginStatus(w http.ResponseWriter, r *http.Request, app *dbdata.WebVpnApp, status int) bool {
	if !webVpnCrossOriginAllowed(r, app) {
		return false
	}
	writeWebVpnCORSHeaders(w, r)
	w.WriteHeader(status)
	return true
}

func writeWebVpnCORSHeadersForApp(w http.ResponseWriter, r *http.Request, app *dbdata.WebVpnApp) {
	origin := r.Header.Get("Origin")
	if origin == "" || (!webVpnSameOrigin(r) && !dbdata.WebVpn.CorsOriginAllowed(app, origin)) {
		return
	}
	setCORSHeaders(w.Header(), origin)
}

func writeWebVpnCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	setCORSHeaders(w.Header(), origin)
}

// 判断请求方法是否在 WebVPN 允许的方法白名单内
func webVpnAllowedMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions:
		return true
	default:
		return false
	}
}

func webVpnValidRequestHeaders(value string) bool {
	for header := range strings.SplitSeq(value, ",") {
		header = strings.TrimSpace(header)
		if header == "" || !webVpnValidHeaderName(header) {
			return false
		}
	}
	return true
}

func webVpnValidHeaderName(name string) bool {
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", c) {
			continue
		}
		return false
	}
	return true
}

// 处理跨域预检（OPTIONS）：直接回应 204 + CORS 头，不反代到后端
func webVpnHandlePreflight(w http.ResponseWriter, r *http.Request, app *dbdata.WebVpnApp) {
	if r.Header.Get("Origin") == "" || (!webVpnSameOrigin(r) && !dbdata.WebVpn.CorsOriginAllowed(app, r.Header.Get("Origin"))) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	requestedMethod := r.Header.Get("Access-Control-Request-Method")
	if !webVpnAllowedMethod(requestedMethod) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	requestedHeaders := r.Header.Get("Access-Control-Request-Headers")
	if requestedHeaders != "" && !webVpnValidRequestHeaders(requestedHeaders) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	writeWebVpnCORSHeaders(w, r)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
	if requestedHeaders == "" {
		requestedHeaders = "Authorization, Content-Type, Accept, Origin, X-Requested-With"
	}
	w.Header().Set("Access-Control-Allow-Headers", requestedHeaders)
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}
