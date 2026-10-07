package gateway

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn/guac"
	"github.com/wsczx/remlink/webvpn/live"
	"github.com/wsczx/remlink/webvpn/ticket"
)

func TestParseDeskParams(t *testing.T) {
	q := url.Values{
		"width": {"99999"}, "height": {"10"}, "dpi": {"abc"},
		"audio":    {"audio/L16", "nomime,bad\"q/x"},
		"image":    {"image/png,image/webp"},
		"timezone": {"Asia/Shanghai"},
	}
	p := parseDeskParams(q)
	assert.Equal(t, 8192, p.width)
	assert.Equal(t, 64, p.height)
	assert.Equal(t, 96, p.dpi)
	assert.Equal(t, []string{"audio/L16"}, p.audio)
	assert.Equal(t, []string{"image/png", "image/webp"}, p.image)
	assert.Nil(t, p.video)
	assert.Equal(t, "Asia/Shanghai", p.timezone)

	d := parseDeskParams(url.Values{"timezone": {"x;rm -rf"}})
	assert.Equal(t, 1024, d.width)
	assert.Equal(t, 768, d.height)
	assert.Equal(t, "", d.timezone)
	assert.Equal(t, []string{"audio/L8", "audio/L16"}, d.audio)
}

func TestDesktopConnectParams(t *testing.T) {
	app := &dbdata.WebVpnApp{
		Name: "win", Protocol: "rdp", TargetHost: "10.0.0.5", Options: `{"security":"nla","ignore_cert":true}`,
		AllowClipboard: false, Record: true,
	}
	m := desktopConnectParams(app, &targetAuth{User: `CORP\alice`, Password: "pw"}, deskParams{width: 1280, height: 720, dpi: 96}, "/record/20260101", "sid.guac")
	assert.Equal(t, "10.0.0.5", m["hostname"])
	assert.Equal(t, "3389", m["port"])
	assert.Equal(t, "alice", m["username"])
	assert.Equal(t, "CORP", m["domain"])
	assert.Equal(t, "pw", m["password"])
	assert.Equal(t, "nla", m["security"])
	assert.Equal(t, "true", m["ignore-cert"])
	assert.Equal(t, "true", m["disable-copy"])
	assert.Equal(t, "true", m["disable-paste"])
	assert.Equal(t, "", m["enable-drive"])
	assert.Equal(t, "/record/20260101", m["recording-path"])
	assert.Equal(t, "sid.guac", m["recording-name"])
	assert.Equal(t, "true", m["create-recording-path"])
	assert.Equal(t, "1280", m["width"])

	vnc := &dbdata.WebVpnApp{Name: "v", Protocol: "vnc", TargetHost: "h", AllowClipboard: true}
	m = desktopConnectParams(vnc, &targetAuth{Password: "x"}, deskParams{}, "", "")
	assert.Equal(t, "5900", m["port"])
	assert.Equal(t, "", m["disable-copy"])
	_, hasRec := m["recording-path"]
	assert.False(t, hasRec)
	_, hasSec := m["security"]
	assert.False(t, hasSec)

	assert.Equal(t, "s.guac", guacRecordName("s", 1))
	assert.Equal(t, "s.3.guac", guacRecordName("s", 3))
}

func ins(op string, args ...string) guac.Instruction { return guac.Instruction{Opcode: op, Args: args} }

func TestClientPolicy(t *testing.T) {
	p := &clientPolicy{allowClipboard: false, allowArgv: true, streams: streamFilter{}}
	assert.True(t, p.allow(ins("mouse", "1", "2", "0")))
	assert.True(t, p.allow(ins("size", "800", "600")))
	assert.False(t, p.allow(ins("select", "ssh")))
	assert.False(t, p.allow(ins("connect", "evil-host")))
	// 剪贴板流整体丢弃，结束后同一流号可复用
	assert.False(t, p.allow(ins("clipboard", "3", "text/plain")))
	assert.False(t, p.allow(ins("blob", "3", "aGk=")))
	assert.False(t, p.allow(ins("end", "3")))
	assert.True(t, p.allow(ins("blob", "3", "aGk=")))
	// argv：仅允许补充账号/密码/域
	assert.True(t, p.allow(ins("argv", "4", "text/plain", "password")))
	assert.False(t, p.allow(ins("argv", "5", "text/plain", "hostname")))
	assert.False(t, p.allow(ins("blob", "5", "ZXZpbA==")))
	assert.False(t, p.allow(ins("end", "5")))

	np := &clientPolicy{allowClipboard: true, allowArgv: false, streams: streamFilter{}}
	assert.True(t, np.allow(ins("clipboard", "3", "text/plain")))
	assert.False(t, np.allow(ins("argv", "4", "text/plain", "password")))

	sp := &serverPolicy{allowClipboard: false, streams: streamFilter{}}
	assert.False(t, sp.allow(ins("clipboard", "1", "text/plain")))
	assert.False(t, sp.allow(ins("blob", "1", "x")))
	assert.True(t, sp.allow(ins("blob", "2", "x")))
	assert.False(t, sp.allow(ins("end", "1")))
	assert.True(t, sp.allow(ins("img", "1", "14", "0", "image/png", "0", "0")))

	assert.False(t, isUserInput(""))
	assert.False(t, isUserInput("sync"))
	assert.True(t, isUserInput("key"))
}

func TestGuacReasonMapping(t *testing.T) {
	assert.Equal(t, reasonAuthFailed, reasonFromGuac(0x0301))
	assert.Equal(t, reasonTargetUnreachable, reasonFromGuac(0x0207))
	assert.Equal(t, live.ReasonRemoteClosed, reasonFromGuac(0x020B))
	assert.Equal(t, live.ReasonError, reasonFromGuac(0x0200))
	c, _ := guacStatusOf(live.ReasonKicked)
	assert.Equal(t, 0x0303, c)
	c, _ = guacStatusOf(live.ReasonIdle)
	assert.Equal(t, 0x020A, c)
	assert.Equal(t, wsCloseKicked, closeCodeOf(live.ReasonKicked))
	assert.Equal(t, wsCloseAuthFailed, closeCodeOf(reasonAuthFailed))
	assert.Equal(t, wsCloseGuacd, closeCodeOf(reasonGuacdUnavailable))
}

func TestResolveAuthNonSSH(t *testing.T) {
	base.UpdateCfg(func(c *base.ServerConfig) {
		if c.JwtSecret == "" {
			c.JwtSecret = "test-jwt"
		}
	})
	portal := &dbdata.User{Username: "alice"}
	// VNC：账号、密码均可为空
	vnc := &dbdata.WebVpnApp{Name: "v", Protocol: "vnc", CredMode: "prompt"}
	a, err := resolveAuth(vnc, portal, &credInput{})
	require.NoError(t, err)
	assert.Equal(t, "", a.User)
	// SMB：必须有密码，私钥被忽略
	smb := &dbdata.WebVpnApp{Name: "s", Protocol: "smb", CredMode: "prompt"}
	_, err = resolveAuth(smb, portal, &credInput{Username: "bob", PrivateKey: genKeyPEM(t, "")})
	assert.Equal(t, errNeedPasswordOnly, err)
	a, err = resolveAuth(smb, portal, &credInput{Username: `CORP\bob`, Password: "x", PrivateKey: genKeyPEM(t, "")})
	require.NoError(t, err)
	assert.Nil(t, a.Signer)
	assert.Equal(t, `CORP\bob`, a.User)
	// 托管 VNC 无账号
	sec, _ := dbdata.EncryptWebVpnCred("vncpw")
	sv := &dbdata.WebVpnApp{Name: "sv", Protocol: "vnc", CredMode: "stored", CredSecret: sec, CredKey: "garbage"}
	a, err = resolveAuth(sv, portal, &credInput{Password: "evil"})
	require.NoError(t, err)
	assert.Equal(t, "vncpw", a.Password)
}

// ---------------- 端到端：浏览器 WS ⇄ 网关 ⇄ 假 guacd ----------------

type fakeGuacd struct {
	t        *testing.T
	ln       net.Listener
	connect  chan []string
	received chan guac.Instruction
	conn     chan net.Conn
	reply    func(c net.Conn)
}

func startFakeGuacd(t *testing.T, afterReady func(c net.Conn)) *fakeGuacd {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	f := &fakeGuacd{t: t, ln: ln, connect: make(chan []string, 4), received: make(chan guac.Instruction, 100), conn: make(chan net.Conn, 4), reply: afterReady}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeGuacd) serve(c net.Conn) {
	defer c.Close()
	rd := guac.NewReader(c, 0)
	sel, err := rd.ReadInstruction()
	if err != nil {
		return // 连通性探测
	}
	if sel.Opcode != "select" {
		return
	}
	_, _ = c.Write(guac.Encode("args", "VERSION_1_5_0", "hostname", "port", "username", "password", "disable-copy", "recording-path", "unknown-arg"))
	for {
		in, err := rd.ReadInstruction()
		if err != nil {
			return
		}
		if in.Opcode == "connect" {
			f.connect <- in.Args
			break
		}
	}
	_, _ = c.Write(guac.Encode("ready", "$conn-id"))
	f.conn <- c
	if f.reply != nil {
		f.reply(c)
	}
	for {
		in, err := rd.ReadInstruction()
		if err != nil {
			return
		}
		f.received <- in
	}
}

type deskHarness struct {
	g    *Gateway
	s    *rsession
	ws   *websocket.Conn
	resp *http.Response
	gd   *fakeGuacd
}

func newDeskHarness(t *testing.T, app dbdata.WebVpnApp, afterReady func(c net.Conn)) *deskHarness {
	gd := startFakeGuacd(t, afterReady)
	old := base.GetCfg().GuacdAddr
	base.UpdateCfg(func(c *base.ServerConfig) { c.GuacdAddr = gd.ln.Addr().String() })
	t.Cleanup(func() { base.UpdateCfg(func(c *base.ServerConfig) { c.GuacdAddr = old }) })

	g := &Gateway{
		reg: live.NewRegistry(), tickets: ticket.NewStore(0), fails: newFailLimiter(5, time.Minute),
		files: newFileLogger(), sessions: map[string]*rsession{},
	}
	s := &rsession{
		g: g, app: app, idle: time.Minute, done: make(chan struct{}),
		auth: &targetAuth{User: "alice", Password: "secret"},
		ls:   &live.Session{Sid: "sid1", Username: "alice", AppName: app.Name, Protocol: app.ProtocolOf()},
	}
	s.ls.CloseFunc = func(reason string) {
		s.mu.Lock()
		s.reason = reason
		s.mu.Unlock()
		close(s.done)
	}
	require.NoError(t, g.reg.Register(s.ls, 0))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gconn, err := dialGuacd(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		up := desktopUpgrader
		up.CheckOrigin = func(*http.Request) bool { return true }
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			gconn.Close()
			return
		}
		g.runDesktop(s, conn, gconn, &targetAuth{User: "alice", Password: "secret"}, parseDeskParams(r.URL.Query()), 1, "alice|"+app.Name)
	}))
	t.Cleanup(srv.Close)
	d := websocket.Dialer{Subprotocols: []string{"guacamole"}}
	ws, resp, err := d.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/?width=800&height=600&timezone=Asia/Shanghai", nil)
	require.NoError(t, err)
	t.Cleanup(func() { ws.Close() })
	return &deskHarness{g: g, s: s, ws: ws, resp: resp, gd: gd}
}

// 读取浏览器侧收到的指令直到满足 cond 或超时
func readUntil(t *testing.T, ws *websocket.Conn, cond func(guac.Instruction) bool) []guac.Instruction {
	t.Helper()
	var all []guac.Instruction
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		mt, data, err := ws.ReadMessage()
		require.NoError(t, err, "已收到: %+v", all)
		require.Equal(t, websocket.TextMessage, mt)
		insts, _, perr := guac.ParseAll(data)
		require.NoError(t, perr, "每个帧必须是完整指令序列: %q", data)
		all = append(all, insts...)
		for _, in := range insts {
			if cond(in) {
				return all
			}
		}
	}
}

func TestDesktopTunnelEndToEnd(t *testing.T) {
	t.Chdir(t.TempDir()) // 录像目录 ./conf/webvpn_record 落在临时目录
	app := dbdata.WebVpnApp{Id: 1, Name: "win", Protocol: "rdp", TargetHost: "10.0.0.5", TargetPort: 3390, Record: true, AllowClipboard: false}
	h := newDeskHarness(t, app, func(c net.Conn) {
		// 目标侧剪贴板（应被网关过滤）+ 正常绘制指令，跨多次写入拆分
		out := string(guac.Encode("clipboard", "0", "text/plain")) + string(guac.Encode("blob", "0", "c2VjcmV0")) +
			string(guac.Encode("end", "0")) + string(guac.Encode("name", "桌面😀")) + string(guac.Encode("sync", "100"))
		_, _ = c.Write([]byte(out[:7]))
		time.Sleep(20 * time.Millisecond)
		_, _ = c.Write([]byte(out[7:]))
	})
	assert.Equal(t, "guacamole", h.resp.Header.Get("Sec-WebSocket-Protocol"))

	// 第一条为隧道 UUID 内部指令
	first := readUntil(t, h.ws, func(guac.Instruction) bool { return true })
	require.Equal(t, "", first[0].Opcode)
	require.Len(t, first[0].Args, 1)
	assert.Len(t, first[0].Args[0], 36)

	// 握手参数：按 guacd 给出的 args 顺序回填，未知参数为空
	args := <-h.gd.connect
	assert.Equal(t, []string{"VERSION_1_5_0", "10.0.0.5", "3390", "alice", "secret", "true", args[6], ""}, args)
	assert.True(t, strings.HasSuffix(args[6], "/"+time.Now().Format("20060102")), args[6])

	got := readUntil(t, h.ws, func(in guac.Instruction) bool { return in.Opcode == "sync" })
	for _, in := range got {
		assert.NotEqual(t, "clipboard", in.Opcode, "剪贴板应被过滤")
		assert.NotEqual(t, "blob", in.Opcode)
	}
	assert.Equal(t, "name", got[0].Opcode)
	assert.Equal(t, "桌面😀", got[0].Args[0])
	h.s.mu.Lock()
	assert.Equal(t, time.Now().Format("20060102")+"/sid1.guac", h.s.recPath)
	h.s.mu.Unlock()
	if fi, err := os.Stat(filepath.Join("conf", "webvpn_record", time.Now().Format("20060102"))); assert.NoError(t, err) && runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o733)|os.ModeSticky|os.ModeDir, fi.Mode(), "日期目录应允许 guacd 写入")
	}

	// 浏览器 → guacd：ping 回显、不转发；剪贴板丢弃；鼠标转发
	frame := string(guac.Encode("", "ping", "12345")) + string(guac.Encode("clipboard", "7", "text/plain")) +
		string(guac.Encode("blob", "7", "eA==")) + string(guac.Encode("end", "7")) + string(guac.Encode("mouse", "10", "20", "1"))
	require.NoError(t, h.ws.WriteMessage(websocket.TextMessage, []byte(frame)))
	echo := readUntil(t, h.ws, func(in guac.Instruction) bool { return in.Opcode == "" })
	last := echo[len(echo)-1]
	assert.Equal(t, []string{"ping", "12345"}, last.Args)
	select {
	case in := <-h.gd.received:
		assert.Equal(t, "mouse", in.Opcode)
	case <-time.After(3 * time.Second):
		t.Fatal("鼠标指令未转发")
	}
	assert.False(t, h.s.ls.Closed())

	// 管理员踢下线 → 通知 + error 指令 + Close 帧（code=4001, reason=Guacamole 状态码）
	h.g.reg.Close(h.s.ls, live.ReasonKicked)
	fin := readUntil(t, h.ws, func(in guac.Instruction) bool { return in.Opcode == "error" })
	var sawClosed bool
	for _, in := range fin {
		if in.Opcode == opRemlinkClosed {
			sawClosed = true
			assert.Equal(t, live.ReasonKicked, in.Args[0])
		}
	}
	assert.True(t, sawClosed)
	assert.Equal(t, "771", fin[len(fin)-1].Args[1])
	_, _, err := h.ws.ReadMessage()
	var ce *websocket.CloseError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, wsCloseKicked, ce.Code)
	assert.Equal(t, "771", ce.Text)
}

func TestDesktopGuacdAuthError(t *testing.T) {
	app := dbdata.WebVpnApp{Id: 1, Name: "win2", Protocol: "rdp", TargetHost: "h"}
	h := newDeskHarness(t, app, func(c net.Conn) {
		_, _ = c.Write(guac.Encode("error", "Authentication failure", "769"))
	})
	fin := readUntil(t, h.ws, func(in guac.Instruction) bool { return in.Opcode == "error" })
	// 关闭原因先于 error 指令到达；error 指令原样转发
	require.GreaterOrEqual(t, len(fin), 2)
	assert.Equal(t, opRemlinkClosed, fin[len(fin)-2].Opcode)
	assert.Equal(t, reasonAuthFailed, fin[len(fin)-2].Args[0])
	assert.Equal(t, "769", fin[len(fin)-1].Args[1])
	_, _, err := h.ws.ReadMessage()
	var ce *websocket.CloseError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, wsCloseAuthFailed, ce.Code)
	// 计入认证失败限流
	h.g.fails.mu.Lock()
	n := len(h.g.fails.m["alice|win2"])
	h.g.fails.mu.Unlock()
	assert.Equal(t, 1, n)
	// 会话本身保留，可重新附着
	assert.False(t, h.s.ls.Closed())
}

func TestDesktopBadClientFrame(t *testing.T) {
	app := dbdata.WebVpnApp{Id: 1, Name: "win3", Protocol: "vnc", TargetHost: "h"}
	var once sync.Once
	h := newDeskHarness(t, app, func(c net.Conn) { once.Do(func() { _, _ = c.Write(guac.Encode("sync", "1")) }) })
	readUntil(t, h.ws, func(in guac.Instruction) bool { return in.Opcode == "sync" })
	require.NoError(t, h.ws.WriteMessage(websocket.TextMessage, []byte("4.mouse,1.1")))
	fin := readUntil(t, h.ws, func(in guac.Instruction) bool { return in.Opcode == "error" })
	assert.Equal(t, "512", fin[len(fin)-1].Args[1])
}

func TestDesktopGuacdUnavailable(t *testing.T) {
	old := base.GetCfg().GuacdAddr
	t.Cleanup(func() { base.UpdateCfg(func(c *base.ServerConfig) { c.GuacdAddr = old }) })
	base.UpdateCfg(func(c *base.ServerConfig) { c.GuacdAddr = "" })
	assert.ErrorIs(t, checkGuacd(t.Context()), errGuacdUnavailable)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	base.UpdateCfg(func(c *base.ServerConfig) { c.GuacdAddr = addr })
	assert.Error(t, checkGuacd(t.Context()))
}
