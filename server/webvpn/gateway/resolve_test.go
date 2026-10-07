package gateway

import (
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/wsczx/remlink/dbdata"
)

func TestResolveApp(t *testing.T) {
	withDomain(t, "wv.example.com")

	// 子域名模式：应用由 Host 决定
	r := httptest.NewRequest("GET", "https://ssh1.wv.example.com/webvpn/app-info", nil)
	ref, ok := resolveApp(r)
	if !ok || ref.path || ref.name != "ssh1" || ref.base != "" {
		t.Fatalf("子域名解析错误: %+v %v", ref, ok)
	}

	// 路径模式：应用由路由变量决定
	r = httptest.NewRequest("GET", "https://vpn.example.com/web/ssh/test/webvpn/app-info", nil)
	r = mux.SetURLVars(r, map[string]string{"proto": "ssh", "app": "test"})
	ref, ok = resolveApp(r)
	if !ok || !ref.path || ref.name != "test" || ref.proto != "ssh" || ref.base != "/web/ssh/test" {
		t.Fatalf("路径解析错误: %+v %v", ref, ok)
	}

	// 路径模式不在 WebVPN 子域名上提供
	r = httptest.NewRequest("GET", "https://ssh1.wv.example.com/web/ssh/test/webvpn/app-info", nil)
	r = mux.SetURLVars(r, map[string]string{"proto": "ssh", "app": "test"})
	if _, ok = resolveApp(r); ok {
		t.Fatal("子域名下的路径模式接口应拒绝")
	}

	// 主域名下的子域名模式接口无法解析应用
	r = httptest.NewRequest("GET", "https://vpn.example.com/webvpn/app-info", nil)
	if _, ok = resolveApp(r); ok {
		t.Fatal("主域名 /webvpn/* 不应解析出应用")
	}
}

func TestAccessAllowed(t *testing.T) {
	withDomain(t, "wv.example.com")
	sub := appRef{name: "a"}
	pathSSH := appRef{name: "a", proto: "ssh", path: true}
	pathRDP := appRef{name: "a", proto: "rdp", path: true}

	app := &dbdata.WebVpnApp{Name: "a", Protocol: "ssh"} // 旧数据：空访问方式 = 子域名
	if !accessAllowed(app, sub) || accessAllowed(app, pathSSH) {
		t.Error("默认仅子域名")
	}
	app.AccessMode = dbdata.WebVpnAccessPath
	if accessAllowed(app, sub) || !accessAllowed(app, pathSSH) || accessAllowed(app, pathRDP) {
		t.Error("path：仅路径且协议须一致")
	}
	app.AccessMode = dbdata.WebVpnAccessBoth
	if !accessAllowed(app, sub) || !accessAllowed(app, pathSSH) {
		t.Error("both：两者皆可")
	}

	// 未配置域名：远程协议自动回落到路径方式
	withDomain(t, "")
	app.AccessMode = dbdata.WebVpnAccessSubdomain
	if accessAllowed(app, sub) || !accessAllowed(app, pathSSH) {
		t.Error("未配置域名时远程协议应只能用路径方式")
	}
}

func TestReqCtxWsPath(t *testing.T) {
	c := &reqCtx{}
	if got := c.wsPath(ticketPurposeTerm); got != "/webvpn/ws/term" {
		t.Errorf("子域名 ws_path=%q", got)
	}
	c.base = "/web/rdp/win10"
	if got := c.wsPath(ticketPurposeDesktop); got != "/web/rdp/win10/webvpn/ws/desktop" {
		t.Errorf("路径 ws_path=%q", got)
	}
	if got := c.wsPath(""); got != "" {
		t.Errorf("纯文件协议 ws_path 应为空: %q", got)
	}
}
