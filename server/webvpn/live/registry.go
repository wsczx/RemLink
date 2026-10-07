// Package live 维护 WebVPN 非 HTTP 协议（SSH/SFTP/…）的在线长连接注册表。
//
// 按 sid / 用户 / 应用索引，支持：
//   - 管理端列出与踢会话（admin 包可直接引用，本包不依赖 admin，避免循环引用）
//   - 接入 dbdata 的吊销钩子：踢下线/权限收窄/删用户/改组 → 立即断开该用户全部长连接
//   - 接入应用变更钩子：应用删除/停用/目标变更/授权收窄 → 断开受影响的长连接
//   - 周期复核（兜底）：吊销阈值、应用授权、IP 白名单
package live

import (
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 关闭原因
const (
	ReasonUserClose    = "user_close"
	ReasonIdle         = "idle"
	ReasonKicked       = "kicked"
	ReasonRevoked      = "revoked"
	ReasonAppChanged   = "app_changed"
	ReasonUnauthorized = "unauthorized"
	ReasonRemoteClosed = "remote_closed"
	ReasonError        = "error"
	ReasonShutdown     = "shutdown"
)

var ErrTooManySessions = errors.New("已达到该应用的并发会话上限")

// 在线会话
type Session struct {
	Sid       string
	Username  string
	Groups    []string
	AppId     int
	AppName   string
	Protocol  string
	Target    string // host:port
	ClientIP  string
	StartedAt time.Time

	bytesIn    atomic.Int64
	bytesOut   atomic.Int64
	lastActive atomic.Int64 // unix nano
	terminals  atomic.Int32
	closed     atomic.Bool

	// 由会话持有者设置：执行实际断开（关闭 SSH/WS 等），须幂等、不阻塞
	CloseFunc func(reason string)
}

func (s *Session) AddIn(n int64)  { s.bytesIn.Add(n) }
func (s *Session) AddOut(n int64) { s.bytesOut.Add(n) }
func (s *Session) BytesIn() int64 { return s.bytesIn.Load() }
func (s *Session) BytesOut() int64 {
	return s.bytesOut.Load()
}
func (s *Session) Touch()                 { s.lastActive.Store(time.Now().UnixNano()) }
func (s *Session) LastActive() time.Time  { return time.Unix(0, s.lastActive.Load()) }
func (s *Session) TerminalDelta(d int32)  { s.terminals.Add(d) }
func (s *Session) Terminals() int32       { return s.terminals.Load() }
func (s *Session) Closed() bool           { return s.closed.Load() }
func (s *Session) IdleFor() time.Duration { return time.Since(s.LastActive()) }
func (s *Session) markClosed() bool       { return s.closed.CompareAndSwap(false, true) }
func (s *Session) key() (string, string)  { return s.Username, s.AppName }
func (s *Session) IsTerminalActive() bool { return s.terminals.Load() > 0 }
func (s *Session) setStartIfZero() {
	if s.StartedAt.IsZero() {
		s.StartedAt = time.Now()
	}
}
func (s *Session) touchIfZero() {
	if s.lastActive.Load() == 0 {
		s.Touch()
	}
}
func (s *Session) groupsJoined() string { return strings.Join(s.Groups, ",") }

// 对外快照
type Info struct {
	Sid        string    `json:"sid"`
	Username   string    `json:"username"`
	GroupName  string    `json:"group_name"`
	AppName    string    `json:"app_name"`
	Protocol   string    `json:"protocol"`
	Target     string    `json:"target"`
	ClientIP   string    `json:"client_ip"`
	StartedAt  time.Time `json:"started_at"`
	LastActive time.Time `json:"last_active"`
	BytesIn    int64     `json:"bytes_in"`
	BytesOut   int64     `json:"bytes_out"`
	Terminals  int32     `json:"terminals"`
}

func (s *Session) Info() Info {
	return Info{
		Sid: s.Sid, Username: s.Username, GroupName: s.groupsJoined(), AppName: s.AppName,
		Protocol: s.Protocol, Target: s.Target, ClientIP: s.ClientIP, StartedAt: s.StartedAt,
		LastActive: s.LastActive(), BytesIn: s.BytesIn(), BytesOut: s.BytesOut(), Terminals: s.Terminals(),
	}
}

// 注册表
type Registry struct {
	mu     sync.RWMutex
	bySid  map[string]*Session
	byUser map[string]map[string]*Session // username -> sid -> s
	byApp  map[string]map[string]*Session // app -> sid -> s
}

func NewRegistry() *Registry {
	return &Registry{
		bySid:  make(map[string]*Session),
		byUser: make(map[string]map[string]*Session),
		byApp:  make(map[string]map[string]*Session),
	}
}

// 注册会话；max>0 时校验该用户在该应用上的并发上限（检查与写入原子完成）
func (r *Registry) Register(s *Session, max int) error {
	s.setStartIfZero()
	s.touchIfZero()
	r.mu.Lock()
	defer r.mu.Unlock()
	if max > 0 {
		n := 0
		for _, o := range r.byUser[s.Username] {
			if o.AppName == s.AppName {
				n++
			}
		}
		if n >= max {
			return ErrTooManySessions
		}
	}
	r.bySid[s.Sid] = s
	u, a := s.key()
	if r.byUser[u] == nil {
		r.byUser[u] = make(map[string]*Session)
	}
	r.byUser[u][s.Sid] = s
	if r.byApp[a] == nil {
		r.byApp[a] = make(map[string]*Session)
	}
	r.byApp[a][s.Sid] = s
	return nil
}

// 从注册表移除（不触发 CloseFunc）
func (r *Registry) Unregister(sid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.bySid[sid]
	if !ok {
		return
	}
	delete(r.bySid, sid)
	u, a := s.key()
	if m := r.byUser[u]; m != nil {
		delete(m, sid)
		if len(m) == 0 {
			delete(r.byUser, u)
		}
	}
	if m := r.byApp[a]; m != nil {
		delete(m, sid)
		if len(m) == 0 {
			delete(r.byApp, a)
		}
	}
}

func (r *Registry) Get(sid string) *Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bySid[sid]
}

// 关闭并移除会话；CloseFunc 只会被调用一次
func (r *Registry) Close(s *Session, reason string) bool {
	if s == nil || !s.markClosed() {
		return false
	}
	r.Unregister(s.Sid)
	if s.CloseFunc != nil {
		s.CloseFunc(reason)
	}
	return true
}

func (r *Registry) Kick(sid, reason string) bool {
	return r.Close(r.Get(sid), reason)
}

func (r *Registry) snapshot(filter func(*Session) bool) []*Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Session, 0)
	for _, s := range r.bySid {
		if filter == nil || filter(s) {
			out = append(out, s)
		}
	}
	return out
}

func (r *Registry) userSessions(username string) []*Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Session, 0, len(r.byUser[username]))
	for _, s := range r.byUser[username] {
		out = append(out, s)
	}
	return out
}

func (r *Registry) appSessions(app string) []*Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Session, 0, len(r.byApp[app]))
	for _, s := range r.byApp[app] {
		out = append(out, s)
	}
	return out
}

// 断开指定用户全部长连接
func (r *Registry) KickUser(username, reason string) int {
	n := 0
	for _, s := range r.userSessions(username) {
		if r.Close(s, reason) {
			n++
		}
	}
	return n
}

// 断开指定应用的长连接；username 非空时仅断开该用户
func (r *Registry) KickApp(app, username, reason string) int {
	n := 0
	for _, s := range r.appSessions(app) {
		if username != "" && s.Username != username {
			continue
		}
		if r.Close(s, reason) {
			n++
		}
	}
	return n
}

// 列出在线会话（按开始时间倒序）
func (r *Registry) List(username, app string) []Info {
	ss := r.snapshot(func(s *Session) bool {
		return (username == "" || s.Username == username) && (app == "" || s.AppName == app)
	})
	out := make([]Info, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Info())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.bySid)
}

// 复核函数：返回 false 时以 reason 断开
type Validator func(s *Session) (ok bool, reason string)

// 用 v 复核 filter 命中的会话，返回断开数
func (r *Registry) Revalidate(filter func(*Session) bool, v Validator) int {
	n := 0
	for _, s := range r.snapshot(filter) {
		if ok, reason := v(s); !ok {
			if r.Close(s, reason) {
				n++
			}
		}
	}
	return n
}

// IP 是否命中白名单（CIDR 或单 IP）
func IPAllowed(ipStr string, list []string) bool {
	if len(list) == 0 {
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			if _, cidr, err := net.ParseCIDR(item); err == nil && cidr.Contains(ip) {
				return true
			}
			continue
		}
		if p := net.ParseIP(item); p != nil && p.Equal(ip) {
			return true
		}
	}
	return false
}
