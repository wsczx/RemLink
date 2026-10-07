// Package gateway 实现 WebVPN 非 HTTP 协议接入的用户侧网关：
// SSH 终端 + SFTP/SMB/FTP(S) 文件 + RDP/VNC 远程桌面（经 guacd，Guacamole 协议隧道）。
//
// 链路：浏览器 ⇄ HTTPS/WebSocket ⇄ 网关 ⇄ 目标（地址仅来自管理员配置）
//
// 路由有两套挂载点，处理逻辑完全复用（见 resolveApp「应用解析器」）：
//   - 子域名模式：WebVPN 子域名的保留前缀 /webvpn/*（WebVpnHandler 对 /webvpn/* 不做反代），应用由 Host 第一段确定，
//     用户来自 webvpn_session；
//   - 路径模式：VPN 主域名下 /web/<协议>/<应用>/webvpn/*，应用由路径确定，用户来自门户会话。
//
// 请求体中的 app 字段只做一致性校验。
package gateway

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn"
	"github.com/wsczx/remlink/webvpn/live"
	"github.com/wsczx/remlink/webvpn/ticket"
)

const (
	defaultIdleTimeout      = 30 * time.Minute
	defaultMaxSessionsPerUA = 5 // 单用户单应用默认并发上限
	reapInterval            = 10 * time.Second
)

// 网关
type Gateway struct {
	reg     *live.Registry
	tickets *ticket.Store
	fails   *failLimiter
	files   *fileLogger

	mu       sync.Mutex
	sessions map[string]*rsession

	startOnce sync.Once
}

var gw = &Gateway{
	reg:      live.Default(),
	tickets:  ticket.NewStore(ticket.DefaultTTL),
	fails:    newFailLimiter(5, 5*time.Minute),
	files:    newFileLogger(),
	sessions: make(map[string]*rsession),
}

// 默认网关实例
func Default() *Gateway { return gw }

// 集中注册用户侧路由（挂在主站 initRoute 的 mux.Router 上）
func RegisterRoutes(r *mux.Router) { gw.RegisterRoutes(r) }

// 路径模式挂载点：/web/<协议>/<应用>（协议段只接受远程协议，HTTP 应用的 /web/http/* 由反代处理）
const pathMount = "/web/{proto:ssh|sftp|smb|ftp|rdp|vnc}/{app:[a-z0-9-]+}"

func (g *Gateway) RegisterRoutes(r *mux.Router) {
	g.start()
	for _, mount := range []string{"", pathMount} {
		p := mount + "/webvpn"
		r.HandleFunc(p+"/app-info", g.handleAppInfo).Methods(http.MethodGet)
		r.HandleFunc(p+"/sessions", g.handleSessionList).Methods(http.MethodGet)
		r.HandleFunc(p+"/sessions", g.handleSessionCreate).Methods(http.MethodPost)
		r.HandleFunc(p+"/sessions", g.handleSessionDelete).Methods(http.MethodDelete) // ?sid=
		r.HandleFunc(p+"/sessions/{sid}", g.handleSessionDelete).Methods(http.MethodDelete)
		r.HandleFunc(p+"/sessions/{sid}/ticket", g.handleTicket).Methods(http.MethodPost)
		r.HandleFunc(p+"/ws/term", g.handleTerm).Methods(http.MethodGet)
		r.HandleFunc(p+"/ws/desktop", g.handleDesktop).Methods(http.MethodGet)
		r.HandleFunc(p+"/fs/{sid}/list", g.handleList).Methods(http.MethodGet)
		r.HandleFunc(p+"/fs/{sid}/stat", g.handleStat).Methods(http.MethodGet)
		r.HandleFunc(p+"/fs/{sid}/download", g.handleDownload).Methods(http.MethodGet)
		r.HandleFunc(p+"/fs/{sid}/upload", g.handleUpload).Methods(http.MethodPost)
		r.HandleFunc(p+"/fs/{sid}/mkdir", g.handleMkdir).Methods(http.MethodPost)
		r.HandleFunc(p+"/fs/{sid}/rename", g.handleRename).Methods(http.MethodPost)
		r.HandleFunc(p+"/fs/{sid}/delete", g.handleDelete).Methods(http.MethodPost)
	}
}

// 启动后台任务（空闲回收、长连接周期复核、文件日志写入），幂等
func (g *Gateway) start() {
	g.startOnce.Do(func() {
		live.StartSweeper(30 * time.Second)
		g.files.start()
		go g.reaper()
	})
}

// 关闭全部会话（进程退出时调用）
func Shutdown() {
	for _, s := range gw.snapshot() {
		gw.reg.Close(s.ls, live.ReasonShutdown)
	}
	gw.files.stop()
}

func (g *Gateway) reaper() {
	t := time.NewTicker(reapInterval)
	defer t.Stop()
	for range t.C {
		for _, s := range g.snapshot() {
			if s.ls.IdleFor() > s.idle {
				g.reg.Close(s.ls, live.ReasonIdle)
			}
		}
	}
}

func (g *Gateway) snapshot() []*rsession {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]*rsession, 0, len(g.sessions))
	for _, s := range g.sessions {
		out = append(out, s)
	}
	return out
}

func (g *Gateway) get(sid string) *rsession {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.sessions[sid]
}

func (g *Gateway) put(s *rsession) {
	g.mu.Lock()
	g.sessions[s.ls.Sid] = s
	g.mu.Unlock()
}

func (g *Gateway) remove(sid string) {
	g.mu.Lock()
	delete(g.sessions, sid)
	g.mu.Unlock()
}

// ---------------- 请求上下文与校验 ----------------

type reqCtx struct {
	user     *dbdata.User
	app      *dbdata.WebVpnApp
	appName  string
	clientIP string
	base     string // 接口基础路径：子域名模式为空，路径模式为 /web/<协议>/<应用>
}

// 返回带基础路径的 WebSocket 路径；无终端/桌面时返回空
func (c *reqCtx) wsPath(purpose string) string {
	p := wsPathOf(purpose)
	if p == "" {
		return ""
	}
	return c.base + p
}

// ---------------- 应用解析器 ----------------

// 一次请求所指向的应用
type appRef struct {
	name  string // 应用名
	proto string // 路径模式下 URL 中的协议段
	base  string // 接口基础路径
	path  bool   // 是否路径模式
}

// 从请求解析应用：路径模式取路由变量（/web/<协议>/<应用>/webvpn/*），否则取 Host 的 WebVPN 子域前缀
func resolveApp(r *http.Request) (appRef, bool) {
	if v := mux.Vars(r); v["app"] != "" {
		// 路径模式只在 VPN 主域名上提供；WebVPN 子域名下不接受
		if _, sub := hostPrefix(r.Host); sub {
			return appRef{}, false
		}
		name, proto := v["app"], v["proto"]
		if !webvpn.WebPathAppValid(name) {
			return appRef{}, false
		}
		return appRef{name: name, proto: proto, base: webvpn.WebPathPrefix(proto, name), path: true}, true
	}
	prefix, ok := hostPrefix(r.Host)
	return appRef{name: prefix}, ok
}

// 路径模式的用户识别（门户会话），由 handler 包注入，避免循环依赖
var pathUserResolver func(*http.Request) (*dbdata.User, bool)

// 注入路径模式的用户识别函数
func SetPathUserResolver(fn func(*http.Request) (*dbdata.User, bool)) { pathUserResolver = fn }

// 按访问方式识别当前用户：子域名模式用 webvpn_session，路径模式用门户会话
func currentUser(r *http.Request, ref appRef) (*dbdata.User, bool) {
	if ref.path {
		if pathUserResolver == nil {
			return nil, false
		}
		return pathUserResolver(r)
	}
	return webvpn.GetManager().Session().CurrentUser(r)
}

// 应用是否允许以该方式访问（路径模式还要求 URL 中的协议与应用一致）
func accessAllowed(app *dbdata.WebVpnApp, ref appRef) bool {
	sub, path := app.AccessOf(base.GetCfg().WebVpnDomain != "")
	if ref.path {
		return path && app.ProtocolOf() == ref.proto
	}
	return sub
}

// WebSocket 入口的公共前置校验：应用解析 → 严格同源 → 用户
func wsPreflight(w http.ResponseWriter, r *http.Request) (appRef, *dbdata.User, bool) {
	ref, ok := resolveApp(r)
	if !ok {
		http.Error(w, "Not Found", http.StatusNotFound)
		return ref, nil, false
	}
	if !strictSameOrigin(r) || (ref.path && webvpn.FromProxiedPage(r)) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return ref, nil, false
	}
	user, ok := currentUser(r, ref)
	if !ok || user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return ref, nil, false
	}
	return ref, user, true
}

// 返回 Host 对应的 WebVPN 子域前缀（小写）；非 WebVPN 子域返回 false
func hostPrefix(host string) (string, bool) {
	domain := strings.ToLower(stripPort(base.GetCfg().WebVpnDomain))
	if domain == "" {
		return "", false
	}
	h := strings.ToLower(stripPort(host))
	prefix, ok := strings.CutSuffix(h, "."+domain)
	if !ok || prefix == "" || strings.Contains(prefix, ".") {
		return "", false
	}
	return prefix, true
}

func stripPort(host string) string {
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		if strings.Contains(host, "]") {
			return host[:strings.LastIndexByte(host, ':')]
		}
		return host[:i]
	}
	return host
}

func requestTLS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// 严格同源：Origin 必须存在且 scheme+host 与当前请求（即当前应用子域）完全一致
func strictSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || r.Host == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if requestTLS(r) {
		if u.Scheme != "https" {
			return false
		}
	} else if u.Scheme != "http" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// GET 类请求：浏览器给出的 Sec-Fetch-Site 必须为同源（或用户直接导航）
func fetchSiteOK(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return true
	}
	return false
}

// 统一的请求前置校验：应用解析 → (可选)严格同源 → 会话 → 应用与访问方式 → 用户/组/IP 授权 → 协议
func (g *Gateway) guard(w http.ResponseWriter, r *http.Request, strictOrigin bool) (*reqCtx, bool) {
	ref, ok := resolveApp(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "请在 WebVPN 应用子域名或 /web/ 路径下访问")
		return nil, false
	}
	if strictOrigin && !strictSameOrigin(r) {
		writeErr(w, http.StatusForbidden, "bad_origin", "非法的请求来源")
		return nil, false
	}
	if !strictOrigin && !fetchSiteOK(r) {
		writeErr(w, http.StatusForbidden, "bad_origin", "非法的请求来源")
		return nil, false
	}
	// 路径模式与门户同源：拒绝来自被代理 HTTP 应用页面的调用
	if ref.path && webvpn.FromProxiedPage(r) {
		writeErr(w, http.StatusForbidden, "bad_origin", "非法的请求来源")
		return nil, false
	}
	mgr := webvpn.GetManager()
	user, ok := currentUser(r, ref)
	if !ok || user == nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "请先登录")
		return nil, false
	}
	app, err := dbdata.GetWebVpnAppByName(ref.name)
	if err != nil || app == nil || !accessAllowed(app, ref) {
		writeErr(w, http.StatusNotFound, "app_not_found", "应用不存在")
		return nil, false
	}
	if !webvpn.AuthorizedUserIP(app, user, r) {
		writeErr(w, http.StatusForbidden, "forbidden", "无权访问该应用")
		return nil, false
	}
	switch app.ProtocolOf() {
	case dbdata.WebVpnProtoSSH, dbdata.WebVpnProtoSFTP, dbdata.WebVpnProtoSMB, dbdata.WebVpnProtoFTP,
		dbdata.WebVpnProtoRDP, dbdata.WebVpnProtoVNC:
	default:
		writeErr(w, http.StatusBadRequest, "unsupported_protocol", "该应用协议暂不支持网页访问")
		return nil, false
	}
	if !ref.path {
		if _, err := mgr.Session().Renew(w, r); err != nil {
			base.Warn("WebVPN 会话续期失败:", err)
		}
	}
	return &reqCtx{user: user, app: app, appName: ref.name, clientIP: webvpn.RealClientIP(r), base: ref.base}, true
}

// 取会话并校验归属（用户 + 应用）
func (g *Gateway) ownedSession(w http.ResponseWriter, c *reqCtx, sid string) (*rsession, bool) {
	s := g.get(sid)
	if s == nil || s.ls.Closed() || s.ls.Username != c.user.Username || s.app.Name != c.appName || s.app.Id != c.app.Id {
		writeErr(w, http.StatusNotFound, "session_not_found", "会话不存在或已结束")
		return nil, false
	}
	return s, true
}

// ---------------- 响应 ----------------

type apiResp struct {
	Code  int    `json:"code"`
	Msg   string `json:"msg"`
	Error string `json:"error,omitempty"` // 机器可读错误码
	Data  any    `json:"data,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, apiResp{Code: 0, Msg: "ok", Data: data})
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiResp{Code: status, Msg: msg, Error: code})
}

func writeErrData(w http.ResponseWriter, status int, code, msg string, data any) {
	writeJSON(w, status, apiResp{Code: status, Msg: msg, Error: code, Data: data})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "参数错误")
		return false
	}
	return true
}
