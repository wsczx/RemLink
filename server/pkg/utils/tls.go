package utils

import (
	"net/http"
	"strings"
)

// 判断请求是否经 TLS/HTTPS 传输（含反向代理透传的 X-Forwarded-Proto）
func IsTLS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
