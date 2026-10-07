package gateway

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn"
	"github.com/wsczx/remlink/webvpn/guac"
	"github.com/wsczx/remlink/webvpn/live"
	"github.com/wsczx/remlink/webvpn/remotefs"
	"github.com/wsczx/remlink/webvpn/ticket"
)

// 远程桌面（RDP/VNC）：浏览器 guacamole-common-js ⇄ WebSocket(子协议 "guacamole") ⇄ 网关 ⇄ guacd(TCP) ⇄ 目标
//
// 网关代替 guacamole-client（Java）完成与 guacd 的握手（select/args/size/audio/video/image/timezone/connect），
// 连接参数（目标地址、凭据、剪贴板/录像策略）全部来自管理员配置与网关会话，浏览器无法指定；
// 握手完成后按指令边界双向转发，并在网关侧二次执行剪贴板策略、屏蔽可改写连接参数的指令。

const (
	guacdDialTimeout      = 5 * time.Second
	guacdHandshakeTimeout = 15 * time.Second
	guacdWriteWait        = 10 * time.Second
	desktopMaxReadSize    = 1 << 20
	desktopBatchSize      = 64 << 10

	// 网关自定义指令：关闭原因通知（guacamole-common-js 会忽略未知 opcode，前端可在 tunnel.oninstruction 中拦截）
	opRemlinkClosed = "x-remlink-closed"
)

// Guacamole 状态码（guacamole-common-js Guacamole.Status.Code）
const (
	guacSuccess             = 0x0000
	guacServerError         = 0x0200
	guacUpstreamTimeout     = 0x0202
	guacUpstreamError       = 0x0203
	guacUpstreamNotFound    = 0x0207
	guacUpstreamUnavailable = 0x0208
	guacSessionConflict     = 0x0209
	guacSessionTimeout      = 0x020A
	guacSessionClosed       = 0x020B
	guacClientBadRequest    = 0x0300
	guacClientUnauthorized  = 0x0301
	guacClientForbidden     = 0x0303
)

var errGuacdUnavailable = errors.New("远程桌面服务（guacd）未配置或不可达，请联系管理员")

var desktopUpgrader = websocket.Upgrader{
	ReadBufferSize:  16 << 10,
	WriteBufferSize: 64 << 10,
	Subprotocols:    []string{"guacamole"},
	CheckOrigin:     strictSameOrigin,
}

func guacdAddr() string { return strings.TrimSpace(base.GetCfg().GuacdAddr) }

func dialGuacd(ctx context.Context) (net.Conn, error) {
	addr := guacdAddr()
	if addr == "" {
		return nil, errGuacdUnavailable
	}
	d := &net.Dialer{Timeout: guacdDialTimeout, KeepAlive: 30 * time.Second}
	return d.DialContext(ctx, "tcp", addr)
}

// 建会话时确认 guacd 已配置且可连通
func checkGuacd(ctx context.Context) error {
	c, err := dialGuacd(ctx)
	if err != nil {
		return err
	}
	return c.Close()
}

// guacd 返回的 error 指令
type guacError struct {
	Code int
	Msg  string
}

func (e *guacError) Error() string { return fmt.Sprintf("guacd error %d: %s", e.Code, e.Msg) }

func guacErrorOf(inst guac.Instruction) *guacError {
	e := &guacError{Code: guacServerError}
	if len(inst.Args) > 0 {
		e.Msg = inst.Args[0]
	}
	if len(inst.Args) > 1 {
		if c, err := strconv.Atoi(inst.Args[1]); err == nil {
			e.Code = c
		}
	}
	return e
}

// guacd 状态码 → 网关关闭原因
func reasonFromGuac(code int) string {
	switch code {
	case guacSuccess:
		return reasonExit
	case guacClientUnauthorized:
		return reasonAuthFailed
	case guacUpstreamTimeout, guacUpstreamError, guacUpstreamNotFound, guacUpstreamUnavailable:
		return reasonTargetUnreachable
	case guacSessionConflict, guacSessionTimeout, guacSessionClosed:
		return live.ReasonRemoteClosed
	}
	return live.ReasonError
}

// 网关关闭原因 → Guacamole 状态码与提示
func guacStatusOf(reason string) (int, string) {
	switch reason {
	case live.ReasonKicked:
		return guacClientForbidden, "会话已被管理员断开"
	case live.ReasonRevoked:
		return guacClientForbidden, "登录状态已失效，请重新登录"
	case live.ReasonUnauthorized:
		return guacClientForbidden, "已无权访问该应用"
	case live.ReasonAppChanged:
		return guacClientForbidden, "应用配置已变更，请重新连接"
	case live.ReasonIdle:
		return guacSessionTimeout, "长时间无操作，会话已断开"
	case live.ReasonUserClose:
		return guacSessionClosed, "会话已结束"
	case live.ReasonRemoteClosed:
		return guacSessionClosed, "远程桌面连接已断开"
	case live.ReasonShutdown:
		return guacSessionClosed, "服务正在重启"
	case reasonExit:
		return guacSuccess, "远程桌面已断开"
	case reasonAuthFailed:
		return guacClientUnauthorized, "目标主机认证失败"
	case reasonTargetUnreachable:
		return guacUpstreamUnavailable, "无法连接目标主机"
	case reasonGuacdUnavailable:
		return guacServerError, errGuacdUnavailable.Error()
	case reasonConflict:
		return guacSessionConflict, "该会话已在其它窗口打开"
	}
	return guacServerError, "远程桌面连接错误"
}

// 浏览器提供的显示参数
type deskParams struct {
	width, height, dpi int
	audio, video       []string
	image              []string
	timezone           string
}

var tzRe = regexp.MustCompile(`^[A-Za-z0-9_+\-/]{1,64}$`)

func clampInt(v, def, lo, hi int) int {
	if v <= 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func mimeList(q url.Values, key string, def []string) []string {
	vals, ok := q[key]
	if !ok {
		return def
	}
	var out []string
	for _, v := range vals {
		for _, m := range strings.Split(v, ",") {
			m = strings.TrimSpace(m)
			if guac.ValidMimetype(m) && len(out) < 16 {
				out = append(out, m)
			}
		}
	}
	return out
}

func parseDeskParams(q url.Values) deskParams {
	atoi := func(k string) int { v, _ := strconv.Atoi(q.Get(k)); return v }
	p := deskParams{
		width:  clampInt(atoi("width"), 1024, 64, 8192),
		height: clampInt(atoi("height"), 768, 64, 8192),
		dpi:    clampInt(atoi("dpi"), 96, 48, 480),
		audio:  mimeList(q, "audio", []string{"audio/L8", "audio/L16"}),
		video:  mimeList(q, "video", nil),
		image:  mimeList(q, "image", []string{"image/png", "image/jpeg", "image/webp"}),
	}
	if tz := q.Get("timezone"); tzRe.MatchString(tz) {
		p.timezone = tz
	}
	return p
}

// guacd 视角下的录像根目录
func guacRecordRoot() string {
	if p := strings.TrimSpace(base.GetCfg().GuacdRecordPath); p != "" {
		return path.Clean(filepath.ToSlash(p))
	}
	abs, err := filepath.Abs(dbdata.WebVpnRecordDir())
	if err != nil {
		return filepath.ToSlash(dbdata.WebVpnRecordDir())
	}
	return filepath.ToSlash(abs)
}

// 预建本地日期目录（guacd 与本服务共享录像卷时生效）：
// guacd 通常以非 root 用户运行，而 SSH 录像可能已以 0700 建好同名日期目录，导致 guacd 无法写入；
// 这里把日期目录设为 01733（他人可创建文件但不可列目录/删除他人文件，同 /tmp 语义去掉读权限）
func prepareGuacRecordDir(date string) {
	local, err := dbdata.WebVpnRecordFullPath(date)
	if err != nil {
		return
	}
	if err := os.MkdirAll(local, 0700); err != nil {
		base.Warn("WebVPN 创建录像目录失败:", local, err)
		return
	}
	_ = os.Chmod(local, 0o733|os.ModeSticky)
}

// 录像文件名：首次附着 <sid>.guac，之后 <sid>.<n>.guac（同一日期目录）
func guacRecordName(sid string, n int) string {
	if n <= 1 {
		return sid + ".guac"
	}
	return sid + "." + strconv.Itoa(n) + ".guac"
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return ""
}

// 组装 guacd 连接参数（仅来自管理员配置 + 网关会话）
func desktopConnectParams(app *dbdata.WebVpnApp, auth *targetAuth, p deskParams, recDir, recName string) map[string]string {
	o := app.OptionsOf()
	noClip := boolStr(!app.AllowClipboard)
	m := map[string]string{
		"hostname":      app.TargetHost,
		"port":          strconv.Itoa(app.TargetPortOf()),
		"username":      auth.User,
		"password":      auth.Password,
		"disable-copy":  noClip,
		"disable-paste": noClip,
		// 文件传输通道一律关闭（驱动器重定向/SFTP 暂不支持，避免绕过文件审计）
		"enable-sftp": "",
	}
	if app.ProtocolOf() == dbdata.WebVpnProtoRDP {
		user, domain := remotefs.SplitSMBUser(auth.User, o.Domain)
		sec := o.Security
		if sec == "" {
			sec = "any"
		}
		m["username"] = user
		m["domain"] = domain
		m["security"] = sec
		m["ignore-cert"] = boolStr(o.IgnoreCert)
		m["width"] = strconv.Itoa(p.width)
		m["height"] = strconv.Itoa(p.height)
		m["dpi"] = strconv.Itoa(p.dpi)
		m["resize-method"] = "display-update"
		m["client-name"] = "RemLink"
		m["timezone"] = p.timezone
		m["enable-drive"] = ""
		m["enable-printing"] = ""
		m["disable-upload"] = "true"
		m["disable-download"] = "true"
		m["enable-audio-input"] = ""
	}
	if recDir != "" {
		m["recording-path"] = recDir
		m["recording-name"] = recName
		m["create-recording-path"] = "true"
		// 不记录键盘输入，避免口令落盘（与 SSH 录像策略一致）
		m["recording-include-keys"] = ""
	}
	return m
}

// 与 guacd 完成握手；返回 guacd 连接 ID
func guacHandshake(gconn net.Conn, rd *guac.Reader, proto string, params map[string]string, p deskParams) (string, error) {
	_ = gconn.SetDeadline(time.Now().Add(guacdHandshakeTimeout))
	defer gconn.SetDeadline(time.Time{})
	send := func(op string, args ...string) error {
		_, err := gconn.Write(guac.Encode(op, args...))
		return err
	}
	if err := send("select", proto); err != nil {
		return "", err
	}
	args, err := rd.ReadInstruction()
	if err != nil {
		return "", err
	}
	if args.Opcode == "error" {
		return "", guacErrorOf(args)
	}
	if args.Opcode != "args" {
		return "", fmt.Errorf("guacd 握手应答异常: %q", args.Opcode)
	}
	versioned := len(args.Args) > 0 && strings.HasPrefix(args.Args[0], "VERSION_")
	if err := send("size", strconv.Itoa(p.width), strconv.Itoa(p.height), strconv.Itoa(p.dpi)); err != nil {
		return "", err
	}
	if err := send("audio", p.audio...); err != nil {
		return "", err
	}
	if err := send("video", p.video...); err != nil {
		return "", err
	}
	if err := send("image", p.image...); err != nil {
		return "", err
	}
	if versioned && p.timezone != "" {
		// timezone 指令自协议 1.1.0 起支持
		if err := send("timezone", p.timezone); err != nil {
			return "", err
		}
	}
	vals := make([]string, len(args.Args))
	for i, name := range args.Args {
		if i == 0 && versioned {
			// 回应 guacd 提供的协议版本（与浏览器端 guacamole-common-js 同代即可）
			vals[i] = name
			continue
		}
		vals[i] = params[name]
	}
	if err := send("connect", vals...); err != nil {
		return "", err
	}
	ready, err := rd.ReadInstruction()
	if err != nil {
		return "", err
	}
	switch ready.Opcode {
	case "ready":
		if len(ready.Args) > 0 {
			return ready.Args[0], nil
		}
		return "", nil
	case "error":
		return "", guacErrorOf(ready)
	}
	return "", fmt.Errorf("guacd 握手应答异常: %q", ready.Opcode)
}

func newTunnelUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// ---------------- 指令过滤 ----------------

// 单方向的流过滤：被丢弃的 clipboard/argv 流，其后续 blob/end 一并丢弃
type streamFilter map[string]bool

func (f streamFilter) dropStream(idx string) { f[idx] = true }

// 返回 true 表示该指令属于已丢弃的流
func (f streamFilter) inDropped(inst guac.Instruction) bool {
	if len(f) == 0 || len(inst.Args) == 0 {
		return false
	}
	switch inst.Opcode {
	case "blob", "ack":
		return f[inst.Args[0]]
	case "end":
		if f[inst.Args[0]] {
			delete(f, inst.Args[0])
			return true
		}
	}
	return false
}

// 浏览器 → guacd 的指令策略
type clientPolicy struct {
	allowClipboard bool
	allowArgv      bool // 仅 prompt 凭据模式允许浏览器在 guacd 要求时补充账号/密码/域
	streams        streamFilter
}

// 返回是否转发
func (p *clientPolicy) allow(inst guac.Instruction) bool {
	if p.streams.inDropped(inst) {
		return false
	}
	switch inst.Opcode {
	case "select", "connect", "nest":
		// 握手已由网关完成，禁止浏览器重放握手指令
		return false
	case "clipboard":
		if !p.allowClipboard {
			if len(inst.Args) > 0 {
				p.streams.dropStream(inst.Args[0])
			}
			return false
		}
	case "argv":
		ok := p.allowArgv && len(inst.Args) >= 3
		if ok {
			switch inst.Args[2] {
			case "username", "password", "domain":
			default:
				ok = false
			}
		}
		if !ok {
			if len(inst.Args) > 0 {
				p.streams.dropStream(inst.Args[0])
			}
			return false
		}
	}
	return true
}

// guacd → 浏览器 的指令策略
type serverPolicy struct {
	allowClipboard bool
	streams        streamFilter
}

func (p *serverPolicy) allow(inst guac.Instruction) bool {
	if p.streams.inDropped(inst) {
		return false
	}
	if inst.Opcode == "clipboard" && !p.allowClipboard {
		if len(inst.Args) > 0 {
			p.streams.dropStream(inst.Args[0])
		}
		return false
	}
	return true
}

// 是否为用户操作（用于空闲判定；同步/应答/心跳不计）
func isUserInput(op string) bool {
	switch op {
	case guac.InternalOpcode, "sync", "ack", "nop":
		return false
	}
	return true
}

// ---------------- WebSocket 入口 ----------------

// GET /webvpn/ws/desktop?ticket=&width=&height=&dpi=[&audio=&video=&image=&timezone=]
func (g *Gateway) handleDesktop(w http.ResponseWriter, r *http.Request) {
	ref, user, ok := wsPreflight(w, r)
	if !ok {
		return
	}
	prefix := ref.name
	clientIP := webvpn.RealClientIP(r)
	claims, err := g.tickets.Consume(r.URL.Query().Get("ticket"), ticket.Claims{
		Username: user.Username, App: prefix, ClientIP: clientIP, Purpose: ticketPurposeDesktop,
	})
	if err != nil {
		http.Error(w, "Forbidden: "+err.Error(), http.StatusForbidden)
		return
	}
	s := g.get(claims.Sid)
	if s == nil || s.ls.Closed() || s.ls.Username != user.Username || s.app.Name != prefix || !s.app.IsDesktop() {
		http.Error(w, "Session Not Found", http.StatusNotFound)
		return
	}
	app, err := dbdata.GetWebVpnAppByName(prefix)
	if err != nil || app == nil || app.Id != s.app.Id || !accessAllowed(app, ref) || !webvpn.AuthorizedUserIP(app, user, r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	failKey := user.Username + "|" + prefix
	if g.fails.blocked(failKey) {
		writeErr(w, http.StatusTooManyRequests, "too_many_failures", "认证失败次数过多，请稍后再试")
		return
	}
	s.mu.Lock()
	busy := s.desk != nil
	s.mu.Unlock()
	if busy {
		http.Error(w, "Desktop Already Attached", http.StatusConflict)
		return
	}
	params := parseDeskParams(r.URL.Query())

	ctx, cancel := context.WithTimeout(r.Context(), guacdDialTimeout)
	gconn, err := dialGuacd(ctx)
	cancel()
	if err != nil {
		base.Warn("WebVPN 连接 guacd 失败:", guacdAddr(), err)
		writeErr(w, http.StatusServiceUnavailable, "guacd_unavailable", errGuacdUnavailable.Error())
		return
	}

	conn, err := desktopUpgrader.Upgrade(w, r, nil)
	if err != nil {
		gconn.Close()
		return // Upgrade 已写回错误
	}
	conn.SetReadLimit(desktopMaxReadSize)

	s.mu.Lock()
	if s.desk != nil || s.ls.Closed() || s.auth == nil {
		s.mu.Unlock()
		gconn.Close()
		reason := reasonConflict
		if s.ls.Closed() {
			reason = live.ReasonUserClose
		}
		closeDesktopWS(conn, reason)
		return
	}
	s.desk = conn
	s.deskN++
	n := s.deskN
	auth := *s.auth
	s.mu.Unlock()
	s.ls.TerminalDelta(1)
	s.ls.Touch()
	defer func() {
		s.ls.TerminalDelta(-1)
		s.mu.Lock()
		if s.desk == conn {
			s.desk = nil
		}
		s.mu.Unlock()
	}()

	g.runDesktop(s, conn, gconn, &auth, params, n, failKey)
}

// 未进入隧道前的关闭（无 UUID 指令时 guacamole-common-js 也能正确解析 error 指令）
func closeDesktopWS(c *websocket.Conn, reason string) {
	code, msg := guacStatusOf(reason)
	_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_ = c.WriteMessage(websocket.TextMessage, guac.Encode(opRemlinkClosed, reason, msg))
	if code != guacSuccess {
		_ = c.WriteMessage(websocket.TextMessage, guac.Encode("error", msg, strconv.Itoa(code)))
	}
	_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(closeCodeOf(reason), strconv.Itoa(code)), time.Now().Add(2*time.Second))
	_ = c.Close()
}

// 单次桌面附着的完整生命周期
func (g *Gateway) runDesktop(s *rsession, conn *websocket.Conn, gconn net.Conn, auth *targetAuth, p deskParams, n int, failKey string) {
	var wmu sync.Mutex
	write := func(b []byte) error {
		wmu.Lock()
		defer wmu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
		return conn.WriteMessage(websocket.TextMessage, b)
	}
	var finOnce sync.Once
	var noticeSent atomic.Bool
	// 结束本次附着：通知原因 →（可选）error 指令 → Close 帧（reason 为 Guacamole 状态码，兼容 guacamole-common-js）
	finish := func(reason string, sendError bool, msg string) {
		finOnce.Do(func() {
			code, def := guacStatusOf(reason)
			if msg == "" {
				msg = def
			}
			if !noticeSent.Load() {
				_ = write(guac.Encode(opRemlinkClosed, reason, msg))
			}
			if sendError && code != guacSuccess {
				_ = write(guac.Encode("error", msg, strconv.Itoa(code)))
			}
			wmu.Lock()
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(closeCodeOf(reason), strconv.Itoa(code)), time.Now().Add(2*time.Second))
			wmu.Unlock()
			_ = conn.Close()
			_ = gconn.Close()
		})
	}

	// 隧道 UUID（guacamole-common-js WebSocketTunnel 期望的第一条内部指令）
	if err := write(guac.Encode(guac.InternalOpcode, newTunnelUUID())); err != nil {
		_ = conn.Close()
		_ = gconn.Close()
		return
	}

	// 录像
	recDir, recName, recRel := "", "", ""
	if s.app.Record {
		date := time.Now().Format("20060102")
		recDir = path.Join(guacRecordRoot(), date)
		recName = guacRecordName(s.ls.Sid, n)
		recRel = date + "/" + recName
		prepareGuacRecordDir(date)
	}
	params := desktopConnectParams(&s.app, auth, p, recDir, recName)
	rd := guac.NewReader(gconn, desktopBatchSize)
	connID, err := guacHandshake(gconn, rd, s.ls.Protocol, params, p)
	if err != nil {
		var ge *guacError
		if errors.As(err, &ge) {
			base.Warn("WebVPN guacd 握手被拒绝:", s.ls.Sid, s.ls.AppName, ge.Code, ge.Msg)
			finish(reasonFromGuac(ge.Code), true, "")
		} else {
			base.Warn("WebVPN guacd 握手失败:", s.ls.Sid, s.ls.AppName, err)
			finish(reasonGuacdUnavailable, true, "")
		}
		return
	}
	if recRel != "" {
		s.mu.Lock()
		first := s.recPath == ""
		if first {
			s.recPath = recRel
		}
		s.mu.Unlock()
		if first {
			go s.markRecording(recRel)
		}
	}
	base.Info("WebVPN 远程桌面附着:", s.ls.Sid, s.ls.Username, s.ls.AppName, s.ls.Target, "guacd", connID)

	sp := &serverPolicy{allowClipboard: s.app.AllowClipboard, streams: streamFilter{}}
	cp := &clientPolicy{
		allowClipboard: s.app.AllowClipboard,
		allowArgv:      s.app.CredModeOf() == dbdata.WebVpnCredPrompt,
		streams:        streamFilter{},
	}

	// guacd → 浏览器：按指令边界成批转发（每个 WS 文本帧都是完整指令序列）
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		for {
			raw, insts, err := rd.ReadBatch()
			if err != nil {
				if errors.Is(err, guac.ErrMalformed) || errors.Is(err, guac.ErrTooLarge) {
					base.Warn("WebVPN guacd 输出格式错误:", s.ls.Sid, err)
					finish(live.ReasonError, true, "")
				} else {
					finish(live.ReasonRemoteClosed, true, "")
				}
				return
			}
			var gerr *guacError
			disconnect := false
			dropped := false
			keep := make([]bool, len(insts))
			for i, in := range insts {
				switch in.Opcode {
				case "error":
					gerr = guacErrorOf(in)
				case "disconnect":
					disconnect = true
				}
				keep[i] = sp.allow(in)
				if !keep[i] {
					dropped = true
				}
			}
			if dropped {
				// 有指令被过滤时按保留的指令重新编码（编码结果与原文等价）
				var b []byte
				for i, in := range insts {
					if keep[i] {
						b = append(b, in.Encode()...)
					}
				}
				raw = b
			}
			if gerr != nil {
				// 先发关闭原因：guacamole-common-js 收到 error 后会立即断开隧道，之后的指令不再处理
				noticeSent.Store(true)
				_ = write(guac.Encode(opRemlinkClosed, reasonFromGuac(gerr.Code), gerr.Msg))
			}
			if len(raw) > 0 {
				s.ls.AddOut(int64(len(raw)))
				if err := write(raw); err != nil {
					finish(live.ReasonError, false, "")
					return
				}
			}
			if gerr != nil {
				reason := reasonFromGuac(gerr.Code)
				if reason == reasonAuthFailed {
					g.fails.fail(failKey)
				}
				base.Info("WebVPN 远程桌面 guacd 报错:", s.ls.Sid, s.ls.AppName, gerr.Code, gerr.Msg)
				// error 指令已原样转发，不再重复发送
				finish(reason, false, gerr.Msg)
				return
			}
			if disconnect {
				finish(reasonExit, false, "")
				return
			}
		}
	}()

	// 心跳与会话结束监听
	stopPing := make(chan struct{})
	defer close(stopPing)
	go func() {
		t := time.NewTicker(wsPingPeriod)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				wmu.Lock()
				err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsWriteWait))
				wmu.Unlock()
				if err != nil {
					_ = conn.Close()
					return
				}
			case <-stopPing:
				return
			case <-pumpDone:
				return
			case <-s.done:
				// 会话被踢/吊销/空闲回收/用户结束
				s.mu.Lock()
				reason := s.reason
				s.mu.Unlock()
				finish(reason, true, "")
				return
			}
		}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	// 浏览器 → guacd：每个文本帧应为完整指令序列
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
		if mt != websocket.TextMessage || len(data) == 0 {
			continue
		}
		insts, raws, perr := guac.ParseAll(data)
		if perr != nil {
			finish(live.ReasonError, true, "非法的 Guacamole 指令")
			break
		}
		out := make([]byte, 0, len(data))
		active := false
		for i, in := range insts {
			if in.Opcode == guac.InternalOpcode {
				// 隧道内部指令：ping 原样回显（guacamole-common-js 据此判断隧道健康），不转发给 guacd
				if len(in.Args) > 0 && in.Args[0] == "ping" {
					_ = write(raws[i])
				}
				continue
			}
			if !cp.allow(in) {
				continue
			}
			if isUserInput(in.Opcode) {
				active = true
			}
			out = append(out, raws[i]...)
		}
		if active {
			s.ls.Touch()
		}
		if len(out) == 0 {
			continue
		}
		s.ls.AddIn(int64(len(out)))
		_ = gconn.SetWriteDeadline(time.Now().Add(guacdWriteWait))
		if _, err := gconn.Write(out); err != nil {
			finish(live.ReasonRemoteClosed, true, "")
			break
		}
	}
	// 浏览器断开：关闭本次到 guacd 的连接（会话保留，可凭新票据重新附着）
	_ = gconn.Close()
	select {
	case <-pumpDone:
	case <-time.After(3 * time.Second):
	}
	_ = conn.Close()
	base.Debug("WebVPN 远程桌面断开:", s.ls.Sid)
}
