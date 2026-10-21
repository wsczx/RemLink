package utils

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// 验证安全头加固（渗透测试报告项）
// 无 Server 指纹头、CSP 不含 unsafe-eval、包含 Permissions-Policy
func TestSetSecureHeader(t *testing.T) {
	w := httptest.NewRecorder()
	SetSecureHeader(w)

	// Server 头不得泄露产品指纹
	if s := w.Header().Get("Server"); s != "" {
		t.Errorf("Server 头应移除（产品指纹泄露），当前为 %q", s)
	}

	// CSP 不得包含 unsafe-eval（前端为 runtime-only 构建，无需运行时编译）
	csp := w.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP 不应包含 unsafe-eval: %s", csp)
	}
	if !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("CSP script-src 应为 'self': %s", csp)
	}

	// Permissions-Policy 必须存在且禁用非必要能力
	pp := w.Header().Get("Permissions-Policy")
	if pp == "" {
		t.Error("缺少 Permissions-Policy 头")
	}
	for _, feat := range []string{"camera=()", "microphone=()", "geolocation=()"} {
		if !strings.Contains(pp, feat) {
			t.Errorf("Permissions-Policy 应禁用 %s: %s", feat, pp)
		}
	}

	// 既有安全头保持
	for _, h := range []string{"X-Frame-Options", "X-Content-Type-Options", "Strict-Transport-Security", "Referrer-Policy"} {
		if w.Header().Get(h) == "" {
			t.Errorf("安全头 %s 缺失", h)
		}
	}
}
