package gateway

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"golang.org/x/crypto/ssh"
)

func TestMain(m *testing.M) {
	base.Test()
	os.Exit(m.Run())
}

func withDomain(t *testing.T, d string) {
	t.Helper()
	old := base.GetCfg().WebVpnDomain
	base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = d })
	t.Cleanup(func() { base.UpdateCfg(func(c *base.ServerConfig) { c.WebVpnDomain = old }) })
}

func TestHostPrefix(t *testing.T) {
	withDomain(t, "vpn.example.com:8443")
	cases := map[string]string{
		"ssh1.vpn.example.com":      "ssh1",
		"SSH1.VPN.EXAMPLE.COM:8443": "ssh1",
		"vpn.example.com":           "",
		"a.b.vpn.example.com":       "",
		"ssh1.vpn.example.com.evil": "",
		"evil.com":                  "",
	}
	for host, want := range cases {
		got, ok := hostPrefix(host)
		if (want != "") != ok || got != want {
			t.Errorf("hostPrefix(%q)=%q,%v want %q", host, got, ok, want)
		}
	}
}

func TestStrictSameOrigin(t *testing.T) {
	mk := func(host, origin string, tls bool) bool {
		r := httptest.NewRequest("POST", "https://"+host+"/webvpn/sessions", nil)
		r.Host = host
		if !tls {
			r.TLS = nil
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return strictSameOrigin(r)
	}
	if !mk("ssh1.vpn.example.com", "https://ssh1.vpn.example.com", true) {
		t.Error("同源应通过")
	}
	if !mk("ssh1.vpn.example.com:8443", "https://SSH1.vpn.example.com:8443", true) {
		t.Error("大小写不同的同源应通过")
	}
	for _, bad := range []string{
		"",                                      // 缺失 Origin
		"null",                                  // 沙箱 iframe
		"https://web1.vpn.example.com",          // 兄弟子域（同站不同源）
		"http://ssh1.vpn.example.com",           // 协议降级
		"https://ssh1.vpn.example.com:444",      // 端口不同
		"https://ssh1.vpn.example.com.evil.com", // 后缀伪造
		"https://user@ssh1.vpn.example.com",     // userinfo
		"https://ssh1.vpn.example.com/path?x=1", // 带路径
	} {
		if mk("ssh1.vpn.example.com", bad, true) {
			t.Errorf("Origin %q 应拒绝", bad)
		}
	}
	if !mk("ssh1.vpn.example.com", "http://ssh1.vpn.example.com", false) {
		t.Error("明文 HTTP 同源应通过")
	}
}

func TestFetchSite(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if !fetchSiteOK(r) {
		t.Error("无 Sec-Fetch-Site 应放行（非浏览器/旧浏览器）")
	}
	r.Header.Set("Sec-Fetch-Site", "same-site")
	if fetchSiteOK(r) {
		t.Error("兄弟子域发起的请求应拒绝")
	}
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if !fetchSiteOK(r) {
		t.Error("同源应放行")
	}
}

func genKeyPEM(t *testing.T, passphrase string) string {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	var blk *pem.Block
	var err error
	if passphrase == "" {
		blk, err = ssh.MarshalPrivateKey(priv, "")
	} else {
		blk, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(blk))
}

func TestResolveAuthModes(t *testing.T) {
	base.UpdateCfg(func(c *base.ServerConfig) {
		if c.JwtSecret == "" {
			c.JwtSecret = "test-jwt"
		}
	})
	portal := &dbdata.User{Username: "alice"}

	// prompt：需要用户名与凭据
	app := &dbdata.WebVpnApp{Name: "a", Protocol: "ssh", CredMode: "prompt"}
	if _, err := resolveAuth(app, portal, &credInput{Password: "x"}); err != errNeedUsername {
		t.Errorf("应要求用户名: %v", err)
	}
	if _, err := resolveAuth(app, portal, &credInput{Username: "root"}); err != errNeedCred {
		t.Errorf("应要求凭据: %v", err)
	}
	if _, err := resolveAuth(app, portal, &credInput{Username: "ro ot", Password: "x"}); err != errBadUsername {
		t.Errorf("非法用户名应拒绝: %v", err)
	}
	a, err := resolveAuth(app, portal, &credInput{Username: "root", PrivateKey: genKeyPEM(t, "pp"), Passphrase: "pp"})
	if err != nil || a.Signer == nil || a.User != "root" {
		t.Errorf("带口令私钥解析失败: %v", err)
	}
	if _, err := resolveAuth(app, portal, &credInput{Username: "root", PrivateKey: genKeyPEM(t, "pp"), Passphrase: "wrong"}); err != errBadKey {
		t.Errorf("错误口令应失败: %v", err)
	}
	// prompt + 固定账号：忽略用户提交的用户名
	app.CredUser = "deploy"
	a, _ = resolveAuth(app, portal, &credInput{Username: "root", Password: "x"})
	if a.User != "deploy" {
		t.Errorf("固定账号未生效: %s", a.User)
	}

	// stored：只用托管凭据，忽略用户输入
	sec, _ := dbdata.EncryptWebVpnCred("stored-pw")
	key, _ := dbdata.EncryptWebVpnCred(genKeyPEM(t, ""))
	st := &dbdata.WebVpnApp{Name: "s", Protocol: "ssh", CredMode: "stored", CredUser: "svc", CredSecret: sec, CredKey: key}
	a, err = resolveAuth(st, portal, &credInput{Username: "root", Password: "evil"})
	if err != nil || a.User != "svc" || a.Password != "stored-pw" || a.Signer == nil {
		t.Errorf("托管凭据解析错误: %+v %v", a, err)
	}
	st.CredSecret = "$WVK1$corrupted"
	if _, err := resolveAuth(st, portal, &credInput{}); err != errStoredBroken {
		t.Errorf("损坏的托管凭据应报错: %v", err)
	}

	// passthrough：用门户账号；无缓存时要求输入
	pt := &dbdata.WebVpnApp{Name: "p", Protocol: "ssh", CredMode: "passthrough"}
	ForgetLoginPassword("alice")
	if _, err := resolveAuth(pt, portal, &credInput{Username: "root"}); err != errNeedPassword {
		t.Errorf("无缓存应要求密码: %v", err)
	}
	RememberLoginPassword("alice", "portal-pw")
	a, err = resolveAuth(pt, portal, &credInput{Username: "root", Password: "other"})
	if err != nil || a.User != "alice" || a.Password != "portal-pw" {
		t.Errorf("passthrough 解析错误: %+v %v", a, err)
	}
	ForgetLoginPassword("alice")
}

func TestPwCacheExpire(t *testing.T) {
	c := newPwCache()
	c.put("u", "pw", 50*time.Millisecond)
	if v, ok := c.get("u"); !ok || v != "pw" {
		t.Fatal("缓存读取失败")
	}
	// 内存中保存的是密文
	if strings.Contains(string(c.m["u"].ct), "pw") {
		t.Fatal("缓存不应保存明文")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := c.get("u"); ok {
		t.Fatal("应已过期")
	}
}

func TestFailLimiter(t *testing.T) {
	f := newFailLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if f.blocked("k") {
			t.Fatal("未达阈值不应锁定")
		}
		f.fail("k")
	}
	if !f.blocked("k") {
		t.Fatal("应锁定")
	}
	f.reset("k")
	if f.blocked("k") {
		t.Fatal("重置后应解锁")
	}
}

func TestNormalizeFps(t *testing.T) {
	got := normalizeFps(" SHA256:abc= , def ,")
	if len(got) != 2 || got[0] != "SHA256:abc" || got[1] != "SHA256:def" {
		t.Fatalf("normalizeFps=%v", got)
	}
}

func TestHostKeyPinnedMismatch(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sp, _ := ssh.NewPublicKey(pub)
	fp := ssh.FingerprintSHA256(sp)
	var seen string
	ok := hostKeyCallback(&dbdata.WebVpnApp{HostKeyFp: fp}, &seen)
	if err := ok("h", nil, sp); err != nil {
		t.Fatalf("指纹一致应通过: %v", err)
	}
	bad := hostKeyCallback(&dbdata.WebVpnApp{HostKeyFp: "SHA256:other"}, &seen)
	if err := bad("h", nil, sp); err != errHostKeyMismatch {
		t.Fatalf("指纹不一致应拒绝: %v", err)
	}
	if seen != strings.TrimRight(fp, "=") {
		t.Fatal("应回传实际指纹")
	}
}

func TestRecorderAsciicast(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d", "x.cast")
	r, err := newRecorder(p, 80, 24, "u@app")
	if err != nil {
		t.Fatal(err)
	}
	s := []byte("中文ok")
	// 在多字节字符中间截断
	r.Output(s[:2])
	r.Output(s[2:])
	r.Resize(100, 30)
	r.Close()
	r.Output([]byte("after close"))

	f, _ := os.Open(p)
	defer f.Close()
	sc := bufio.NewScanner(f)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) < 3 {
		t.Fatalf("录像行数不足: %v", lines)
	}
	var hdr map[string]any
	if json.Unmarshal([]byte(lines[0]), &hdr) != nil || hdr["version"].(float64) != 2 || hdr["width"].(float64) != 80 {
		t.Fatalf("头部错误: %s", lines[0])
	}
	var out strings.Builder
	sawResize := false
	for _, l := range lines[1:] {
		var ev []any
		if err := json.Unmarshal([]byte(l), &ev); err != nil || len(ev) != 3 {
			t.Fatalf("事件格式错误: %s", l)
		}
		switch ev[1] {
		case "o":
			out.WriteString(ev[2].(string))
		case "r":
			sawResize = ev[2] == "100x30"
		}
	}
	if out.String() != "中文ok" {
		t.Fatalf("UTF-8 截断处理错误: %q", out.String())
	}
	if !sawResize {
		t.Fatal("缺少 resize 事件")
	}
	if strings.Contains(strings.Join(lines, "\n"), "after close") {
		t.Fatal("关闭后不应继续写入")
	}
}

func TestClampAndCloseCode(t *testing.T) {
	if clampSize(0, 80) != 80 || clampSize(5000, 80) != 1000 || clampSize(120, 80) != 120 {
		t.Fatal("clampSize 错误")
	}
	if closeCodeOf("revoked") != wsCloseKicked || closeCodeOf("idle") != wsCloseIdle || closeCodeOf("user_close") != wsCloseUserClose {
		t.Fatal("关闭码映射错误")
	}
}
