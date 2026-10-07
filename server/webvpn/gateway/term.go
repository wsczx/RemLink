package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn"
	"github.com/wsczx/remlink/webvpn/live"
	"github.com/wsczx/remlink/webvpn/ticket"
	"golang.org/x/crypto/ssh"
)

const (
	wsWriteWait   = 10 * time.Second
	wsPongWait    = 70 * time.Second
	wsPingPeriod  = 25 * time.Second
	wsMaxReadSize = 64 << 10
	termBufSize   = 32 << 10
)

// WebSocket 关闭码（4000-4999 为应用自定义）
const (
	wsCloseExit         = 4000 // shell 正常退出
	wsCloseKicked       = 4001 // 被踢/吊销/授权变化
	wsCloseIdle         = 4002 // 空闲超时
	wsCloseRemoteClosed = 4003 // 目标断开
	wsCloseError        = 4004 // 其它错误
	wsCloseUserClose    = 4005 // 用户主动结束会话
	wsCloseAuthFailed   = 4006 // 目标认证失败（远程桌面）
	wsCloseUnreachable  = 4007 // 目标不可达（远程桌面）
	wsCloseGuacd        = 4008 // guacd 未配置/不可达（远程桌面）
	wsCloseConflict     = 4009 // 已在其它窗口附着
)

// 附加关闭原因（远程桌面）
const (
	reasonExit              = "exit"
	reasonAuthFailed        = "auth_failed"
	reasonTargetUnreachable = "target_unreachable"
	reasonGuacdUnavailable  = "guacd_unavailable"
	reasonConflict          = "conflict"
)

func closeCodeOf(reason string) int {
	switch reason {
	case live.ReasonKicked, live.ReasonRevoked, live.ReasonUnauthorized, live.ReasonAppChanged:
		return wsCloseKicked
	case live.ReasonIdle:
		return wsCloseIdle
	case live.ReasonRemoteClosed:
		return wsCloseRemoteClosed
	case live.ReasonUserClose:
		return wsCloseUserClose
	case live.ReasonShutdown:
		return websocket.CloseGoingAway
	case reasonExit:
		return wsCloseExit
	case reasonAuthFailed:
		return wsCloseAuthFailed
	case reasonTargetUnreachable:
		return wsCloseUnreachable
	case reasonGuacdUnavailable:
		return wsCloseGuacd
	case reasonConflict:
		return wsCloseConflict
	}
	return wsCloseError
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  8 << 10,
	WriteBufferSize: 32 << 10,
	// 严格校验 Origin 为当前应用子域（scheme+host 完全一致）
	CheckOrigin: strictSameOrigin,
}

// 发送关闭通知（JSON 控制帧 + Close 帧），并关闭底层连接
func closeWS(c *websocket.Conn, reason string) {
	msg, _ := json.Marshal(map[string]any{"type": "closed", "reason": reason})
	_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_ = c.WriteMessage(websocket.TextMessage, msg)
	_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(closeCodeOf(reason), reason), time.Now().Add(2*time.Second))
	_ = c.Close()
}

// 终端控制帧（客户端 → 网关，文本帧）
type ctrlMsg struct {
	Type string `json:"type"` // resize | ping | stdin
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
	Data string `json:"data"`
}

func clampSize(v, def int) int {
	if v <= 0 {
		return def
	}
	if v > 1000 {
		return 1000
	}
	return v
}

// GET /webvpn/ws/term?ticket=...&cols=&rows=：附着交互式终端
func (g *Gateway) handleTerm(w http.ResponseWriter, r *http.Request) {
	ref, user, ok := wsPreflight(w, r)
	if !ok {
		return
	}
	prefix := ref.name
	clientIP := webvpn.RealClientIP(r)
	claims, err := g.tickets.Consume(r.URL.Query().Get("ticket"), ticket.Claims{
		Username: user.Username, App: prefix, ClientIP: clientIP, Purpose: ticketPurposeTerm,
	})
	if err != nil {
		http.Error(w, "Forbidden: "+err.Error(), http.StatusForbidden)
		return
	}
	s := g.get(claims.Sid)
	if s == nil || s.ls.Closed() || s.ls.Username != user.Username || s.app.Name != prefix || s.ls.Protocol != dbdata.WebVpnProtoSSH {
		http.Error(w, "Session Not Found", http.StatusNotFound)
		return
	}
	app, err := dbdata.GetWebVpnAppByName(prefix)
	if err != nil || app == nil || app.Id != s.app.Id || !accessAllowed(app, ref) || !webvpn.AuthorizedUserIP(app, user, r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	s.mu.Lock()
	if s.term != nil {
		s.mu.Unlock()
		http.Error(w, "Terminal Already Attached", http.StatusConflict)
		return
	}
	s.mu.Unlock()

	q := r.URL.Query()
	cols, _ := strconv.Atoi(q.Get("cols"))
	rows, _ := strconv.Atoi(q.Get("rows"))
	cols, rows = clampSize(cols, 80), clampSize(rows, 24)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade 已写回错误
	}
	conn.SetReadLimit(wsMaxReadSize)

	s.mu.Lock()
	if s.term != nil || s.ls.Closed() {
		s.mu.Unlock()
		closeWS(conn, "conflict")
		return
	}
	s.term = conn
	s.mu.Unlock()
	s.ls.TerminalDelta(1)
	s.ls.Touch()
	defer func() {
		s.ls.TerminalDelta(-1)
		s.mu.Lock()
		if s.term == conn {
			s.term = nil
		}
		s.mu.Unlock()
	}()

	g.runTerminal(s, conn, cols, rows)
}

// 单个终端附着的完整生命周期
func (g *Gateway) runTerminal(s *rsession, conn *websocket.Conn, cols, rows int) {
	var wmu sync.Mutex
	write := func(mt int, b []byte) error {
		wmu.Lock()
		defer wmu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
		return conn.WriteMessage(mt, b)
	}
	writeCtl := func(v any) error {
		b, _ := json.Marshal(v)
		return write(websocket.TextMessage, b)
	}
	fail := func(msg string) {
		_ = writeCtl(map[string]any{"type": "error", "message": msg})
		wmu.Lock()
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(wsCloseError, "error"), time.Now().Add(time.Second))
		wmu.Unlock()
		_ = conn.Close()
	}

	sess, err := s.ssh.NewSession()
	if err != nil {
		fail("打开终端通道失败")
		return
	}
	defer sess.Close()
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		fail("申请伪终端失败")
		return
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		fail("终端初始化失败")
		return
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		fail("终端初始化失败")
		return
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		fail("终端初始化失败")
		return
	}
	if err := sess.Shell(); err != nil {
		fail("启动 shell 失败")
		return
	}

	rec := s.recorderFor(cols, rows)
	_ = writeCtl(map[string]any{"type": "ready", "sid": s.ls.Sid, "cols": cols, "rows": rows, "record": rec != nil})

	// 目标输出 → 浏览器（二进制帧）
	var outWG sync.WaitGroup
	pump := func(rd io.Reader) {
		defer outWG.Done()
		buf := make([]byte, termBufSize)
		for {
			n, err := rd.Read(buf)
			if n > 0 {
				s.ls.AddOut(int64(n))
				if rec != nil {
					rec.Output(buf[:n])
				}
				if werr := write(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
	outWG.Add(2)
	go pump(stdout)
	go pump(stderr)

	// shell 退出 → 通知并关闭
	exited := make(chan struct{})
	go func() {
		err := sess.Wait()
		outWG.Wait()
		code := 0
		if ee, ok := err.(*ssh.ExitError); ok {
			code = ee.ExitStatus()
		} else if err != nil {
			code = -1
		}
		_ = writeCtl(map[string]any{"type": "exit", "code": code})
		wmu.Lock()
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(wsCloseExit, "exit"), time.Now().Add(time.Second))
		wmu.Unlock()
		_ = conn.Close()
		close(exited)
	}()

	// 心跳
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
			case <-s.done:
				// 会话被踢/吊销/空闲回收：先发 JSON 通知再发 Close 帧（写操作经 wmu 串行化）
				s.mu.Lock()
				reason := s.reason
				s.mu.Unlock()
				_ = writeCtl(map[string]any{"type": "closed", "reason": reason})
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(closeCodeOf(reason), reason), time.Now().Add(2*time.Second))
				_ = conn.Close()
				return
			}
		}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	// 浏览器输入 → 目标（二进制帧为 stdin；文本帧为 JSON 控制）
readLoop:
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
		switch mt {
		case websocket.BinaryMessage:
			if len(data) == 0 {
				continue
			}
			s.ls.Touch()
			s.ls.AddIn(int64(len(data)))
			if _, err := stdin.Write(data); err != nil {
				break readLoop
			}
		case websocket.TextMessage:
			var m ctrlMsg
			if json.Unmarshal(data, &m) != nil {
				continue
			}
			switch m.Type {
			case "resize":
				c, r := clampSize(m.Cols, 80), clampSize(m.Rows, 24)
				_ = sess.WindowChange(r, c)
				if rec != nil {
					rec.Resize(c, r)
				}
			case "ping":
				_ = writeCtl(map[string]any{"type": "pong", "ts": time.Now().UnixMilli()})
			case "stdin":
				if m.Data != "" {
					s.ls.Touch()
					s.ls.AddIn(int64(len(m.Data)))
					_, _ = stdin.Write([]byte(m.Data))
				}
			}
		}
	}
	// 浏览器断开：关闭本终端通道（SSH 连接与会话保留，可凭新票据重连或继续使用文件功能）
	_ = sess.Close()
	select {
	case <-exited:
	case <-time.After(3 * time.Second):
	}
	_ = conn.Close()
	base.Debug("WebVPN 终端断开:", s.ls.Sid)
}
