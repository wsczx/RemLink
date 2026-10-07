package dbdata

import "testing"

func TestWebVpnAccessOf(t *testing.T) {
	cases := []struct {
		proto, mode     string
		domain          bool
		wantSub, wantPa bool
	}{
		{"http", "", true, true, false},
		{"http", "subdomain", true, true, false},
		{"http", "path", true, false, true},
		{"http", "both", true, true, true},
		// 未配置域名：子域名不可用；HTTP 不自动开放路径方式（须显式选择）
		{"http", "subdomain", false, false, false},
		{"http", "both", false, false, true},
		// 未配置域名：远程协议自动回落到路径方式
		{"ssh", "subdomain", false, false, true},
		{"rdp", "", false, false, true},
		{"ssh", "path", true, false, true},
	}
	for _, c := range cases {
		a := &WebVpnApp{Protocol: c.proto, AccessMode: c.mode}
		sub, path := a.AccessOf(c.domain)
		if sub != c.wantSub || path != c.wantPa {
			t.Errorf("%s/%s domain=%v: got (%v,%v) want (%v,%v)", c.proto, c.mode, c.domain, sub, path, c.wantSub, c.wantPa)
		}
	}
	a := &WebVpnApp{AccessMode: " PATH "}
	if err := normalizeWebVpnAccessMode(a); err != nil || a.AccessMode != WebVpnAccessPath {
		t.Errorf("规范化失败: %v %q", err, a.AccessMode)
	}
	if err := normalizeWebVpnAccessMode(&WebVpnApp{AccessMode: "x"}); err == nil {
		t.Error("非法访问方式应报错")
	}
}
