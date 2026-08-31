package handler

import (
	"strings"
	"testing"
)

func TestSSOCallbackPathsMatchRoutes(t *testing.T) {
	expected := map[string]string{
		"wxwork":   "WXAuth",
		"feishu":   "FeishuAuth",
		"dingtalk": "DingtalkAuth",
	}
	for ssoType, want := range expected {
		p, ok := ssoProviders[ssoType]
		if !ok {
			t.Fatalf("ssoProviders 缺少类型 %q", ssoType)
		}
		if p.callbackPath != want {
			t.Errorf("ssoType=%q callbackPath=%q，期望 %q（须与 server.go 路由表一致）",
				ssoType, p.callbackPath, want)
		}
	}
}

// SAML 成功页文案应为通用「认证成功」，不应写死特定 IdP（如企业微信）。
// 回归：2026-08-08 将成功页硬编码的「已成功通过企业微信认证」改为通用文案。
func TestSAMLSuccessHTML_Generic(t *testing.T) {
	html := samlSuccessHTML
	if !strings.Contains(html, "认证成功") {
		t.Fatal("成功页应含「认证成功」标题")
	}
	if strings.Contains(html, "企业微信") {
		t.Fatal("成功页不应写死「企业微信」等特定 IdP 名称")
	}
	if !strings.Contains(html, "已成功完成认证") {
		t.Fatal("成功页应包含通用认证成功描述")
	}
}
