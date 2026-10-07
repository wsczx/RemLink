package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/pkg/sftp"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn/live"
	"github.com/wsczx/remlink/webvpn/remotefs"
	"github.com/wsczx/remlink/webvpn/ticket"
	"golang.org/x/crypto/ssh"
)

const (
	ticketPurposeTerm    = "term"
	ticketPurposeDesktop = "desktop"
)

// 一个远程会话：
//   - ssh/sftp：一条到目标的 SSH 连接（终端通道 + 可选 SFTP 子系统复用该连接）
//   - smb/ftp：一个已认证的文件后端（SMB 会话 / FTP 连接池）
//   - rdp/vnc：仅在内存中保存认证材料，每次附着桌面时经 guacd 建立一次到目标的图形连接
type rsession struct {
	g    *Gateway
	ls   *live.Session
	app  dbdata.WebVpnApp // 建立时的应用快照（凭据字段已清空）
	idle time.Duration
	ssh  *ssh.Client       // 仅 ssh/sftp
	rfs  remotefs.RemoteFS // 仅 smb/ftp
	home string            // smb/ftp：未配置 root 时的默认根目录

	mu      sync.Mutex
	auth    *targetAuth // 仅 rdp/vnc：附着桌面时使用的目标认证材料（会话结束即清除）
	sftpc   *sftp.Client
	jail    *remotefs.Jail
	term    *websocket.Conn // 当前附着的终端（同一时刻至多一个）
	desk    *websocket.Conn // 当前附着的桌面（同一时刻至多一个）
	deskN   int             // 桌面附着次数（用于录像文件命名）
	rec       *recorder
	recPath   string
	recFailed bool // 已记录过录像失败原因
	reason  string
	done    chan struct{}
}

func newSid() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// 实际断开（由 live.Registry.Close 调用，仅执行一次）
func (s *rsession) shutdown(reason string) {
	s.mu.Lock()
	s.reason = reason
	sc := s.sftpc
	rec := s.rec
	recPath := s.recPath
	s.auth = nil
	s.mu.Unlock()
	// 已附着的终端/桌面监听 done，自行发送关闭通知并断开 WebSocket（避免并发写 WS）
	close(s.done)
	if sc != nil {
		_ = sc.Close()
	}
	if s.ssh != nil {
		_ = s.ssh.Close()
	}
	if s.rfs != nil {
		_ = s.rfs.Close()
	}
	if rec != nil {
		rec.Close()
	}
	s.g.remove(s.ls.Sid)
	if err := dbdata.EndWebVpnSession(s.ls.Sid, s.ls.BytesIn(), s.ls.BytesOut(), reason, recPath); err != nil {
		base.Warn("WebVPN 会话结束记录写入失败:", s.ls.Sid, err)
	}
	base.Info("WebVPN 远程会话结束:", s.ls.Sid, s.ls.Username, s.ls.AppName, reason)
}

func (s *rsession) close(reason string) { s.g.reg.Close(s.ls, reason) }

var errNoFileAccess = errors.New("该会话不支持文件管理")

// 懒加载文件后端与根目录限定（SSH/SFTP 复用同一 SSH 连接；SMB/FTP 使用建会话时的后端）
func (s *rsession) fs() (*remotefs.Jail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jail != nil {
		return s.jail, nil
	}
	if s.ls.Closed() {
		return nil, errors.New("会话已结束")
	}
	if s.rfs != nil {
		root := s.app.OptionsOf().Root
		if root == "" {
			root = s.home
		}
		j, err := remotefs.NewJail(s.rfs, root)
		if err != nil {
			return nil, err
		}
		s.jail = j
		return j, nil
	}
	if s.ssh == nil {
		return nil, errNoFileAccess
	}
	c, err := sftp.NewClient(s.ssh, sftp.UseConcurrentWrites(true), sftp.UseConcurrentReads(true))
	if err != nil {
		return nil, err
	}
	root := s.app.OptionsOf().Root
	if root == "" {
		// 未配置根目录时限定在目标账号的家目录
		if wd, err := c.Getwd(); err == nil && strings.HasPrefix(wd, "/") {
			root = wd
		} else {
			root = "/"
		}
	}
	j, err := remotefs.NewJail(remotefs.NewSFTP(c), root)
	if err != nil {
		c.Close()
		return nil, err
	}
	s.sftpc = c
	s.jail = j
	return j, nil
}

func idleOf(app *dbdata.WebVpnApp) time.Duration {
	if app.IdleTimeout > 0 {
		return time.Duration(app.IdleTimeout) * time.Second
	}
	return defaultIdleTimeout
}

func permsOf(app *dbdata.WebVpnApp) map[string]any {
	return map[string]any{
		"upload":        app.AllowUpload,
		"download":      app.AllowDownload,
		"delete":        app.AllowDelete,
		"clipboard":     app.AllowClipboard,
		"max_file_size": app.MaxFileSize,
	}
}

func maxSessions(app *dbdata.WebVpnApp) int {
	if app.MaxSessionsPerUser > 0 {
		return app.MaxSessionsPerUser
	}
	return defaultMaxSessionsPerUA
}

// 协议对应的长连接票据用途；无长连接（纯文件协议）返回空
func ticketPurposeOf(proto string) string {
	switch proto {
	case dbdata.WebVpnProtoSSH:
		return ticketPurposeTerm
	case dbdata.WebVpnProtoRDP, dbdata.WebVpnProtoVNC:
		return ticketPurposeDesktop
	}
	return ""
}

func wsPathOf(purpose string) string {
	switch purpose {
	case ticketPurposeTerm:
		return "/webvpn/ws/term"
	case ticketPurposeDesktop:
		return "/webvpn/ws/desktop"
	}
	return ""
}

func fsInitErrCode(proto string) string {
	if proto == dbdata.WebVpnProtoSSH || proto == dbdata.WebVpnProtoSFTP {
		return "sftp_failed"
	}
	return "fs_failed"
}

// GET /webvpn/app-info：当前子域应用的前端展示信息（不含任何凭据）
func (g *Gateway) handleAppInfo(w http.ResponseWriter, r *http.Request) {
	c, ok := g.guard(w, r, false)
	if !ok {
		return
	}
	mode := c.app.CredModeOf()
	proto := c.app.ProtocolOf()
	_, cached := passwords.get(c.user.Username)
	fixedUser := ""
	switch mode {
	case dbdata.WebVpnCredStored, dbdata.WebVpnCredPrompt:
		fixedUser = c.app.CredUser
	case dbdata.WebVpnCredPassthrough:
		fixedUser = c.user.Username
	}
	isVNC := proto == dbdata.WebVpnProtoVNC
	desktop := c.app.IsDesktop()
	info := map[string]any{
		"name":               c.app.Name,
		"note":               c.app.Note,
		"protocol":           proto,
		"cred_mode":          mode,
		"fixed_username":     fixedUser,
		"need_username":      mode == dbdata.WebVpnCredPrompt && c.app.CredUser == "" && !isVNC,
		"username_optional":  isVNC, // VNC 多为仅密码认证，账号可留空
		"need_password":      mode == dbdata.WebVpnCredPrompt || (mode == dbdata.WebVpnCredPassthrough && !cached),
		"password_optional":  isVNC, // VNC 可能未设置密码
		"private_key":        proto == dbdata.WebVpnProtoSSH || proto == dbdata.WebVpnProtoSFTP,
		"terminal":           proto == dbdata.WebVpnProtoSSH,
		"file":               c.app.HasFileAccess(),
		"desktop":            desktop,
		"ws_path":            c.wsPath(ticketPurposeOf(proto)),
		"record":             c.app.Record,
		"record_format":      recordFormatOf(proto),
		"idle_timeout":       int(idleOf(c.app).Seconds()),
		"permissions":        permsOf(c.app),
		"max_sessions":       maxSessions(c.app),
		"username":           c.user.Username,
		"ticket_ttl_seconds": int(ticket.DefaultTTL.Seconds()),
	}
	if desktop {
		info["guacd_available"] = guacdAddr() != ""
	}
	writeOK(w, info)
}

func recordFormatOf(proto string) string {
	switch proto {
	case dbdata.WebVpnProtoSSH:
		return "asciicast"
	case dbdata.WebVpnProtoRDP, dbdata.WebVpnProtoVNC:
		return "guac"
	}
	return ""
}

// GET /webvpn/sessions：当前用户在本应用上的在线会话
func (g *Gateway) handleSessionList(w http.ResponseWriter, r *http.Request) {
	c, ok := g.guard(w, r, false)
	if !ok {
		return
	}
	writeOK(w, g.reg.List(c.user.Username, c.appName))
}

// POST /webvpn/sessions：建立会话 → {sid, ticket}
func (g *Gateway) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	c, ok := g.guard(w, r, true)
	if !ok {
		return
	}
	var in credInput
	if !decodeJSON(w, r, &in, 64<<10) {
		return
	}
	if in.App != "" && !strings.EqualFold(in.App, c.appName) {
		writeErr(w, http.StatusBadRequest, "app_mismatch", "应用与当前域名不一致")
		return
	}
	failKey := c.user.Username + "|" + c.appName
	if g.fails.blocked(failKey) {
		writeErr(w, http.StatusTooManyRequests, "too_many_failures", "认证失败次数过多，请稍后再试")
		return
	}
	auth, err := resolveAuth(c.app, c.user, &in)
	if err != nil {
		var ce *credError
		if errors.As(err, &ce) {
			writeErr(w, http.StatusBadRequest, ce.code, ce.msg)
		} else {
			writeErr(w, http.StatusBadRequest, "bad_credential", err.Error())
		}
		return
	}
	// 预检并发上限，避免无谓地连接目标；注册时再原子校验一次
	max := maxSessions(c.app)
	if len(g.reg.List(c.user.Username, c.appName)) >= max {
		writeErr(w, http.StatusTooManyRequests, "too_many_sessions", live.ErrTooManySessions.Error())
		return
	}

	conn, ok := g.connectTarget(w, r, c, auth, failKey)
	if !ok {
		return
	}

	snap := c.app.Redacted()
	s := &rsession{
		g:    g,
		app:  snap,
		idle: idleOf(c.app),
		ssh:  conn.ssh,
		rfs:  conn.rfs,
		home: conn.home,
		done: make(chan struct{}),
		ls: &live.Session{
			Sid:       newSid(),
			Username:  c.user.Username,
			Groups:    append([]string(nil), c.user.Groups...),
			AppId:     c.app.Id,
			AppName:   c.appName,
			Protocol:  c.app.ProtocolOf(),
			Target:    c.app.TargetAddr(),
			ClientIP:  c.clientIP,
			StartedAt: time.Now(),
		},
	}
	if c.app.IsDesktop() {
		s.auth = auth
	}
	s.ls.CloseFunc = s.shutdown
	if err := g.reg.Register(s.ls, max); err != nil {
		conn.close()
		writeErr(w, http.StatusTooManyRequests, "too_many_sessions", err.Error())
		return
	}
	g.put(s)
	if err := dbdata.AddWebVpnSession(&dbdata.WebVpnSession{
		Sid: s.ls.Sid, Username: s.ls.Username, GroupName: strings.Join(s.ls.Groups, ","),
		AppName: s.ls.AppName, Protocol: s.ls.Protocol, Target: s.ls.Target, ClientIP: s.ls.ClientIP,
		StartedAt: s.ls.StartedAt, RecordMode: dbdata.WebVpnRecordModeOf(&snap),
	}); err != nil {
		base.Warn("WebVPN 会话记录写入失败:", err)
	}
	// 目标断开 → 结束会话
	if s.ssh != nil {
		go func() {
			_ = s.ssh.Wait()
			s.close(live.ReasonRemoteClosed)
		}()
	}
	// 长连接型文件后端（SMB）周期探活，兼防服务端空闲断开
	if p, ok := s.rfs.(interface{ Ping() error }); ok {
		go s.keepAliveFS(p)
	}
	// 纯文件协议立即建立根目录限定，以便尽早暴露根目录配置错误
	switch s.ls.Protocol {
	case dbdata.WebVpnProtoSFTP, dbdata.WebVpnProtoSMB, dbdata.WebVpnProtoFTP:
		if _, err := s.fs(); err != nil {
			s.close(live.ReasonError)
			base.Warn("WebVPN 文件服务初始化失败:", c.appName, s.ls.Protocol, err)
			writeErr(w, http.StatusBadGateway, fsInitErrCode(s.ls.Protocol), "文件服务初始化失败")
			return
		}
	}
	base.Info("WebVPN 远程会话建立:", s.ls.Sid, s.ls.Username, s.ls.AppName, s.ls.Target, auth.User)

	resp := map[string]any{
		"sid":            s.ls.Sid,
		"protocol":       s.ls.Protocol,
		"target_user":    auth.User,
		"host_key_fp":    conn.fp,
		"idle_timeout":   int(s.idle.Seconds()),
		"record":         snap.Record,
		"permissions":    permsOf(&snap),
		"ticket":         "",
		"ticket_expires": 0,
		"ws_path":        "",
	}
	if purpose := ticketPurposeOf(s.ls.Protocol); purpose != "" {
		tk, err := g.issueTicket(s, c, purpose)
		if err != nil {
			s.close(live.ReasonError)
			writeErr(w, http.StatusInternalServerError, "ticket_failed", "票据签发失败")
			return
		}
		resp["ticket"] = tk
		resp["ticket_expires"] = int(ticket.DefaultTTL.Seconds())
		resp["ws_path"] = c.wsPath(purpose)
	}
	writeOK(w, resp)
}

func (g *Gateway) issueTicket(s *rsession, c *reqCtx, purpose string) (string, error) {
	return g.tickets.Issue(ticket.Claims{
		Sid: s.ls.Sid, Username: c.user.Username, App: c.appName, ClientIP: c.clientIP, Purpose: purpose,
	})
}

// 文件后端周期探活：连续失败视为目标断开
func (s *rsession) keepAliveFS(p interface{ Ping() error }) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	fails := 0
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			if err := p.Ping(); err != nil {
				fails++
				if fails >= 2 {
					base.Warn("WebVPN 文件后端探活失败，结束会话:", s.ls.Sid, s.ls.AppName, err)
					s.close(live.ReasonRemoteClosed)
					return
				}
				continue
			}
			fails = 0
		}
	}
}

// POST /webvpn/sessions/{sid}/ticket：为已有会话签发新的终端/桌面票据（断线重连）
func (g *Gateway) handleTicket(w http.ResponseWriter, r *http.Request) {
	c, ok := g.guard(w, r, true)
	if !ok {
		return
	}
	s, ok := g.ownedSession(w, c, mux.Vars(r)["sid"])
	if !ok {
		return
	}
	purpose := ticketPurposeOf(s.ls.Protocol)
	if purpose == "" {
		writeErr(w, http.StatusBadRequest, "unsupported_protocol", "该会话不支持终端/桌面")
		return
	}
	tk, err := g.issueTicket(s, c, purpose)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ticket_failed", "票据签发失败")
		return
	}
	writeOK(w, map[string]any{
		"sid": s.ls.Sid, "ticket": tk, "ticket_expires": int(ticket.DefaultTTL.Seconds()), "ws_path": c.wsPath(purpose),
	})
}

// DELETE /webvpn/sessions/{sid}（或 ?sid=）：用户主动结束会话
func (g *Gateway) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := g.guard(w, r, true)
	if !ok {
		return
	}
	sid := mux.Vars(r)["sid"]
	if sid == "" {
		sid = r.URL.Query().Get("sid")
	}
	s, ok := g.ownedSession(w, c, sid)
	if !ok {
		return
	}
	s.close(live.ReasonUserClose)
	writeOK(w, nil)
}
