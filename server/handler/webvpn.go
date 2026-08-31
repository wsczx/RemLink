package handler

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn"
)

// WebVPN 子域的反向代理入口：按子域（host 第一段）匹配应用
// 串起「认证 → 免登兑换 → 续期 → 授权 → 反代 → 审计」完整链路
// 返回 true 表示请求已被本处理器使用（是 WebVPN 子域请求），false 表示应交由其它路由处理
func WebVpnHandler(w http.ResponseWriter, r *http.Request) bool {
	if base.GetCfg().WebVpnDomain == "" {
		return false
	}
	prefix, isWebVpnHost := webVpnHostPrefix(r.Host)
	if !isWebVpnHost {
		return false
	}
	// WebVPN 自有端点（/webvpn/*，如登录/登出/me）不走代理，交由 initRoute 处理
	if strings.HasPrefix(r.URL.Path, "/webvpn/") {
		return false
	}
	// 门户相关路径一律禁止在 WebVPN 子域下访问（防止跨子域携带通配 portal_session cookie 越权调用门户接口）：
	// 仅放行 GET /portal（登录页）与白名单内的登录前置接口（见 portalLoginEndpoints）。
	if r.URL.Path == "/portal" && r.Method == http.MethodGet {
		return false
	}
	if r.URL.Path == "/portal" || strings.HasPrefix(r.URL.Path, "/portal/") {
		if webVpnPortalLoginEndpoint(r.URL.Path, r.Method) {
			return false
		}
		http.Error(w, "Forbidden", http.StatusForbidden)
		return true
	}

	mgr := webvpn.GetManager()
	app, err := dbdata.GetWebVpnAppByName(prefix)
	// 仅对明确开启跨站访问且配置了来源白名单的应用放宽会话 Cookie。
	r = webvpn.WithCrossSiteCookie(r, err == nil && app != nil && app.Status == 1 && app.AllowCrossSite && len(app.CorsAllowedOrigins) > 0)

	// 跨域预检（OPTIONS）由网关直接回应 CORS 头，不走认证、不反代后端。
	if r.Method == http.MethodOptions {
		if err != nil || app == nil || app.Status != 1 {
			w.WriteHeader(http.StatusNotFound)
			return true
		}
		webVpnHandlePreflight(w, r, app)
		return true
	}

	// 认证优先：已登录 WebVPN 会话用户直接放行。
	user, ok := mgr.Session().CurrentUser(r)
	grantRedirect := r.URL.Query().Get(webvpnGrantQuery) != ""
	if !ok || user == nil {
		// 门户登录后下发的 webvpn_grant 一次性换取正式会话（并写入会话 cookie）
		if token, gu, exchanged := mgr.Session().ExchangeGrant(w, r); exchanged {
			// 注入请求 cookie，使同一次请求内后续 webVpnProxy 的 CurrentUser 能读到，
			// 避免兑换成功却又误判未登录而跳登录页。
			r.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: token})
			user = gu
			ok = true
		}
	}
	if grantRedirect {
		u := *r.URL
		q := u.Query()
		q.Del(webvpnGrantQuery)
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
		return true
	}
	// 未登录 /ui/ 由 RemLink 前端渲染登录页
	// 登录后 /ui/ 透传至后端，以支持 JumpServer 等后端应用自身使用 /ui/ 的前端入口
	if strings.HasPrefix(r.URL.Path, "/ui/") && (!ok || user == nil) {
		return false
	}
	if !ok || user == nil {
		// 门户会话有效但免登兑换失败（权限中途被取消 / grant 过期 / 会话已被吊销）：
		// 直接渲染无权限提示页，而非跳登录页。否则门户已登录的前端会自动回跳、
		// 后端又判定未登录再次跳转，形成高频率刷新死循环。
		if puser, pok := portalCurrentUser(r); pok && puser != nil {
			if webVpnWriteCrossOriginStatus(w, r, app, http.StatusForbidden) {
				return true
			}
			webVpnForbiddenPage(w, r, puser.Username, "")
			return true
		}
		// 跨站接口未登录时返回可识别的 401，避免把接口请求重定向为登录 HTML。
		// 普通浏览器直接打开 WebVPN 页面仍跳转到当前子域的登录页。
		if webVpnWriteCrossOriginStatus(w, r, app, http.StatusUnauthorized) {
			return true
		}
		// 完全未登录：跳转到当前 WebVPN 子域自身的登录页（/ui/#/portal，
		// 由门户登录卡片前端承载），登录成功后门户下发一次性 webvpn_grant，
		// 回域后由 ExchangeGrant 自动兑换正式会话，无需重复登录。
		http.Redirect(w, r, webVpnLoginURL(r), http.StatusFound)
		return true
	}

	// 认证与免登兑换已完成，后续查应用/续期/授权/反代/审计统一交给 webVpnProxy.
	webVpnProxy(w, r, prefix)
	return true
}

// 判断 host 是否属于 WebVPN 子域，返回 (子域前缀, 是否匹配)。
// DNS 主机名大小写不敏感，统一转小写比较，且前缀以小写返回，避免大写 Host 绕过 WebVPN 分支。
func webVpnHostPrefix(host string) (string, bool) {
	domain := base.GetCfg().WebVpnDomain
	if domain == "" {
		return "", false
	}
	d := strings.ToLower(stripPort(domain))
	host = strings.ToLower(stripPort(host))
	if host == d {
		return "", true // 恰好是根域，无前缀
	}
	if before, ok := strings.CutSuffix(host, "."+d); ok {
		prefix := before
		if prefix == "" {
			return "", true
		}
		return prefix, true
	}
	return "", false
}

// 返回当前 WebVPN 子域自身的登录页地址（含端口与协议）
func webVpnLoginURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	redirect := r.URL.RequestURI()
	return scheme + "://" + r.Host + "/ui/#/portal?redirect=" + url.QueryEscape(redirect)
}

func webVpnCrossOriginAllowed(r *http.Request, app *dbdata.WebVpnApp) bool {
	origin := r.Header.Get("Origin")
	return origin != "" && !webVpnSameOrigin(r) && dbdata.WebVpnCorsOriginAllowed(app, origin)
}

func webVpnWriteCrossOriginStatus(w http.ResponseWriter, r *http.Request, app *dbdata.WebVpnApp, status int) bool {
	if !webVpnCrossOriginAllowed(r, app) {
		return false
	}
	writeWebVpnCORSHeaders(w, r)
	w.WriteHeader(status)
	return true
}

func writeWebVpnCORSHeadersForApp(w http.ResponseWriter, r *http.Request, app *dbdata.WebVpnApp) {
	origin := r.Header.Get("Origin")
	if origin == "" || (!webVpnSameOrigin(r) && !dbdata.WebVpnCorsOriginAllowed(app, origin)) {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Vary", "Origin")
}

func writeWebVpnCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Vary", "Origin")
}

// 处理跨域预检（OPTIONS）：直接回应 204 + CORS 头，不反代到后端。
func webVpnAllowedMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions:
		return true
	default:
		return false
	}
}

func webVpnValidRequestHeaders(value string) bool {
	for header := range strings.SplitSeq(value, ",") {
		header = strings.TrimSpace(header)
		if header == "" || !webVpnValidHeaderName(header) {
			return false
		}
	}
	return true
}

func webVpnValidHeaderName(name string) bool {
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", c) {
			continue
		}
		return false
	}
	return true
}

// 处理跨域预检（OPTIONS）：直接回应 204 + CORS 头，不反代到后端。
func webVpnHandlePreflight(w http.ResponseWriter, r *http.Request, app *dbdata.WebVpnApp) {
	if r.Header.Get("Origin") == "" || (!webVpnSameOrigin(r) && !dbdata.WebVpnCorsOriginAllowed(app, r.Header.Get("Origin"))) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	requestedMethod := r.Header.Get("Access-Control-Request-Method")
	if !webVpnAllowedMethod(requestedMethod) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	requestedHeaders := r.Header.Get("Access-Control-Request-Headers")
	if requestedHeaders != "" && !webVpnValidRequestHeaders(requestedHeaders) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	writeWebVpnCORSHeaders(w, r)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
	if requestedHeaders == "" {
		requestedHeaders = "Authorization, Content-Type, Accept, Origin, X-Requested-With"
	}
	w.Header().Set("Access-Control-Allow-Headers", requestedHeaders)
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

// 返回当前 WebVPN 会话用户基本信息
func webVpnMe(w http.ResponseWriter, r *http.Request) {
	user, ok := webvpn.GetManager().Session().CurrentUser(r)
	if !ok || user == nil {
		portalError(w, "未登录")
		return
	}
	portalOK(w, map[string]any{
		"username": user.Username,
		"nickname": user.Nickname,
		"type":     user.Type,
		"email":    user.Email,
	})
}

// 单点登出：清除 WebVPN 会话 cookie 并吊销当前 jti。仅影响 WebVPN，门户登录态不受影响。
// GET 返回一个最小退出确认页（同源表单 POST 到自身），方便用户在子域名下直接退出；
func webVpnLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(webVpnLogoutPageHTML))
		return
	}
	if !webVpnSameOrigin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	mgr := webvpn.GetManager()
	mgr.Session().RevokeCurrent(r)
	mgr.Session().ClearCookie(w, r)
	mgr.Session().ClearGrantCookie(w, r)
	portalOK(w, nil)
}

// 子域下的退出确认页。表单同源 POST 到 /webvpn/logout，
// 登出后跳转回子域登录页，让用户明确知道会话已失效
const webVpnLogoutPageHTML = `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>退出 WebVPN</title>
<style>body{font-family:system-ui,"Microsoft YaHei",sans-serif;background:#f5f7fa;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}
.card{background:#fff;border-radius:12px;box-shadow:0 4px 24px rgba(0,0,0,.08);padding:32px 40px;text-align:center;max-width:360px}
h1{font-size:18px;margin:0 0 12px;color:#1f2d3d}
p{color:#5e6d82;font-size:14px;line-height:1.6;margin:0 0 24px}
button{background:#409eff;color:#fff;border:0;border-radius:6px;padding:10px 28px;font-size:15px;cursor:pointer}
button:hover{background:#66b1ff}</style>
</head>
<body><div class="card">
<h1>退出 WebVPN 访问</h1>
<p>退出后，本子域名下的内网应用将需要重新登录才能访问。门户其它功能的登录态不受影响。</p>
<form method="post" action="/webvpn/logout">
<button type="submit">确认退出</button>
</form>
</div></body>
</html>`

// 供门户"我的应用"展示当前用户有权访问的 WebVPN 应用列表
func webVpnMyApps(w http.ResponseWriter, r *http.Request) {
	user, ok := webvpn.GetManager().Session().CurrentUser(r)
	if !ok || user == nil {
		user, ok = portalCurrentUser(r)
		if !ok || user == nil {
			portalError(w, "未登录")
			return
		}
	}
	apps, err := webvpn.GetManager().Apps().AppsForUser(user)
	if err != nil {
		portalError(w, "获取应用列表失败")
		return
	}
	domain := base.GetCfg().WebVpnDomain
	host := stripPort(domain)
	port := portOf(domain)
	if port == "" {
		port = portOf(r.Host)
	}
	datas := make([]map[string]any, 0, len(apps))
	for _, a := range apps {
		urlStr := ""
		if domain != "" {
			urlStr = "https://" + a.Name + "." + host
			if port != "" {
				urlStr += ":" + port
			}
		}
		datas = append(datas, map[string]any{
			"name": a.Name,
			"note": a.Note,
			"url":  urlStr,
		})
	}
	portalOK(w, datas)
}

// 当前登录入口复用门户登录页，仅返回前端所需子域等基础配置
func webVpnLoginConfig(w http.ResponseWriter, r *http.Request) {
	portalOK(w, map[string]any{
		"domain": base.GetCfg().WebVpnDomain,
	})
}

// 在 WebVPN 子域本地渲染「无权限」错误页。
func webVpnForbiddenPage(w http.ResponseWriter, r *http.Request, username, appName string) {
	msg := "您当前没有访问该应用的权限，如需开通请联系系统管理员。"
	if username != "" {
		msg = username + "，您当前没有访问「" + appName + "」的权限，如需开通请联系系统管理员。"
	}
	webVpnErrorHTML(w, r, webVpnErrorData{
		Reason:   "forbidden",
		Title:    "无权访问该应用",
		Subtitle: msg,
		AppName:  appName,
	})
}

// 应用不存在/已禁用时,同样在 WebVPN 子域本地渲染错误页
// reason 取值：notfound（不存在，返回 404）/ disabled（已禁用，返回 403）。
func webVpnAppErrorPage(w http.ResponseWriter, r *http.Request, appName, reason, msg string) {
	title := "应用不存在"
	status := http.StatusNotFound
	if reason == "disabled" {
		title = "应用已停用"
		status = http.StatusForbidden
	}
	webVpnErrorHTML(w, r, webVpnErrorData{
		Reason:     reason,
		Title:      title,
		Subtitle:   msg,
		AppName:    appName,
		StatusCode: status,
	})
}

// 错误页模板数据。
type webVpnErrorData struct {
	Reason     string
	Title      string
	Subtitle   string
	AppName    string
	StatusCode int
}

func webVpnErrorHTML(w http.ResponseWriter, r *http.Request, data webVpnErrorData) {
	if data.AppName != "" {
		if app, err := webvpn.GetManager().Apps().GetByName(data.AppName); err == nil && app != nil {
			writeWebVpnCORSHeadersForApp(w, r, app)
		}
	}
	if data.Title == "" {
		data.Title = "无法访问"
	}
	if data.Subtitle == "" {
		data.Subtitle = "无权访问该应用"
	}
	if data.StatusCode == 0 {
		data.StatusCode = http.StatusForbidden
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(data.StatusCode)
	if err := webVpnErrorTmpl.Execute(w, data); err != nil {
		base.Error("WebVPN 错误页渲染失败:", err)
	}
}

var webVpnErrorTmpl = template.Must(template.New("webvpnError").Parse(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root{--accent:{{if eq .Reason "forbidden"}}#f59e0b{{else}}#5b7ca8{{end}};--accent-bg:{{if eq .Reason "forbidden"}}rgba(245,158,11,.14){{else}}rgba(91,124,168,.16){{end}};}
*{box-sizing:border-box;margin:0;padding:0;}
html,body{height:100%;}
body{
  font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;
  min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;
  background:linear-gradient(135deg,#1b2138 0%,#2a3a5c 40%,#1a3668 100%);
  color:#e6ebf5;
}
.card{width:100%;max-width:440px;background:rgba(30,38,60,.72);border:1px solid rgba(255,255,255,.08);border-radius:16px;box-shadow:0 20px 60px rgba(0,0,0,.35),0 0 0 1px rgba(255,255,255,.04);padding:44px 36px 36px;text-align:center;}
.icon{width:76px;height:76px;margin:0 auto 24px;border-radius:50%;display:flex;align-items:center;justify-content:center;background:var(--accent-bg);color:var(--accent);}
.icon svg{width:38px;height:38px;}
h1{font-size:22px;font-weight:600;color:#fff;margin-bottom:12px;letter-spacing:.5px;}
.sub{font-size:14px;line-height:1.7;color:#a9b4c9;margin-bottom:8px;word-break:break-all;}
{{if .AppName}}.app{display:inline-block;margin-top:18px;padding:6px 14px;border-radius:8px;font-size:13px;color:#cbd5e8;background:rgba(255,255,255,.05);border:1px solid rgba(255,255,255,.08);}.app b{color:var(--accent);font-weight:600;}{{end}}
.foot{margin-top:28px;padding-top:20px;border-top:1px solid rgba(255,255,255,.07);font-size:12px;color:#6b7793;}
</style>
</head>
<body>
<div class="card">
  <div class="icon">
    {{if eq .Reason "forbidden"}}
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
    {{else}}
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
    {{end}}
  </div>
  <h1>{{.Title}}</h1>
  <p class="sub">{{.Subtitle}}</p>
  {{if .AppName}}<div class="app">应用：<b>{{.AppName}}</b></div>{{end}}
  <div class="foot">RemLink WebVPN 安全网关</div>
</div>
</body>
</html>`))

// 校验请求来源是否属于本站域，用于登出等会改变状态的操作的 CSRF 防护
func webVpnSameOrigin(r *http.Request) bool {
	host := r.Host
	if host == "" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Host == "" {
			return false
		}
		return strings.EqualFold(u.Host, host) &&
			((u.Scheme == "https" && requestTLS(r)) || (u.Scheme == "http" && !requestTLS(r)))
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		u, err := url.Parse(ref)
		if err != nil || u.User != nil || u.Host == "" {
			return false
		}
		return strings.EqualFold(u.Host, host)
	}
	return true
}

func requestTLS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// 记录 WebVPN 请求的审计信息
type webVpnAuditRecord struct {
	user  *dbdata.User
	app   *dbdata.WebVpnApp
	host  string
	start time.Time
	group string
}

func newAuditRecord(user *dbdata.User, app *dbdata.WebVpnApp, host string) *webVpnAuditRecord {
	group := ""
	if len(user.Groups) > 0 {
		group = strings.Join(user.Groups, ",")
	}
	return &webVpnAuditRecord{user: user, app: app, host: host, start: time.Now(), group: group}
}

// 包装 ModifyResponse，在响应返回后投递审计记录。
func withAudit(next func(*http.Response) error, rec *webVpnAuditRecord, audit *webvpn.AuditBatcher, rw *webVpnRespWriter) func(*http.Response) error {
	return func(resp *http.Response) error {
		if next != nil {
			if err := next(resp); err != nil {
				return err
			}
		}
		audit.Log(dbdata.WebVpnAudit{
			Username:   rec.user.Username,
			GroupName:  rec.group,
			AppName:    rec.app.Name,
			Host:       rec.host,
			Method:     rw.req.Method,
			Path:       rw.req.URL.Path,
			StatusCode: resp.StatusCode,
			BytesSent:  rw.bytesWritten,
			ClientIP:   webvpn.RealClientIP(rw.req),
			DurationMs: time.Since(rec.start).Milliseconds(),
			RiskLevel:  webvpn.RiskOf(resp.StatusCode),
		})
		return nil
	}
}

// 给反代响应补 CORS 头：仅当入站带 Origin（浏览器跨域实际请求）时补，与预检一致。
func withCORS(next func(*http.Response) error, r *http.Request, app *dbdata.WebVpnApp) func(*http.Response) error {
	origin := r.Header.Get("Origin")
	if origin == "" || (!webVpnSameOrigin(r) && !dbdata.WebVpnCorsOriginAllowed(app, origin)) {
		return next
	}
	return func(resp *http.Response) error {
		if next != nil {
			if err := next(resp); err != nil {
				return err
			}
		}
		resp.Header.Set("Access-Control-Allow-Origin", origin)
		resp.Header.Set("Access-Control-Allow-Credentials", "true")
		resp.Header.Set("Vary", "Origin")
		return nil
	}
}

// 透出底层 writer 并记录状态码与写出字节数，供访问审计使用。
type webVpnRespWriter struct {
	http.ResponseWriter
	req          *http.Request
	statusCode   int
	bytesWritten int64
}

func (rw *webVpnRespWriter) WriteHeader(code int) {
	if rw.statusCode == 0 {
		rw.statusCode = code
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *webVpnRespWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

func (rw *webVpnRespWriter) Unwrap() http.ResponseWriter { return rw.ResponseWriter }

// 子域名可放行的门户登录前置接口
// 仅在未登录状态下登录流程必需、不依赖已登录 portal_session，放行它们保证
// WebVPN 子域名登录页正常加载配置、完成登录并检测登录态；其余门户写接口仍由
// WebVpnHandler 403 拦截，杜绝跨子域携带门户 cookie 越权调用。
type portalLoginEndpoint struct {
	path    string
	method  string
	handler func(http.ResponseWriter, *http.Request)
}

// WebVPN 子域名登录所放行的门户接口唯一来源：
// initRoute 据此注册路由（见 server.go），WebVpnHandler 据此判断放行。新增子域名登录
// 必需的门户接口只需改此处。
var portalLoginEndpoints = []portalLoginEndpoint{
	{"/portal/api/login", http.MethodPost, PortalLogin},
	{"/portal/api/verify", http.MethodPost, PortalVerify},
	{"/portal/api/sms/send", http.MethodPost, PortalSmsSend},
	{"/portal/api/sms/verify", http.MethodPost, PortalSmsVerify},
	{"/portal/api/login-config", http.MethodGet, PortalLoginConfig},
	{"/portal/api/me", http.MethodGet, PortalMe},
	{"/portal/api/otp/status", http.MethodGet, PortalOTPStatus},
	{"/portal/api/sso", http.MethodGet, PortalSSO},
}

// 判断子域名下是否放行某门户登录接口（以 portalLoginEndpoints 为唯一来源）。
func webVpnPortalLoginEndpoint(path, method string) bool {
	for _, ep := range portalLoginEndpoints {
		if ep.path == path && ep.method == method {
			return true
		}
	}
	return false
}

// 去掉 host:port 中的端口部分，仅保留主机名。
func stripPort(host string) string {
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		if strings.Contains(host, "]") {
			return host[:strings.LastIndexByte(host, ':')]
		}
		return host[:i]
	}
	return host
}

// 提取 host:port 或 scheme://host:port 中的端口部分（不含冒号），无端口返回空。
func portOf(host string) string {
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		if strings.Contains(host, "]") {
			return host[strings.LastIndexByte(host, ':')+1:]
		}
		return host[i+1:]
	}
	return ""
}

// 是 WebVPN 会话 cookie 的规范名称。
const (
	webVpnSessionCookie = "webvpn_session"
	webvpnGrantQuery    = "webvpn_grant"
)

// WebVPN 反向代理的内部执行体。负责：认证取用户 → 查应用配置
// → 滑动续期 → 请求级授权 → 构造反代 → 投递审计 → 转发。WebVpnHandler 完成
// 免登兑换后调用本函数，测试亦直接调用以覆盖授权/审计逻辑，二者共用同一实现。
func webVpnProxy(w http.ResponseWriter, r *http.Request, prefix string) {
	mgr := webvpn.GetManager()
	user, _ := mgr.Session().CurrentUser(r)
	if user == nil {
		// 未登录：跳转到当前 WebVPN 子域自身的登录页，与 WebVpnHandler 入口一致，
		// 避免跳错父域/丢失端口。登录成功后由 ExchangeGrant 自动兑换会话。
		http.Redirect(w, r, webVpnLoginURL(r), http.StatusFound)
		return
	}
	app, err := mgr.Apps().GetByName(prefix)
	if err != nil || app == nil {
		webVpnAppErrorPage(w, r, prefix, "notfound", "应用不存在或已删除")
		return
	}
	// 滑动续期（距签发超过 TTL-1h 时重签）。
	if _, err := mgr.Session().Renew(w, r); err != nil {
		base.Error("WebVPN 会话续期失败:", err)
	}
	// 请求级完整授权（用户/组/IP/路径白名单）。
	if !webvpn.Authorized(app, user, r) {
		webVpnForbiddenPage(w, r, user.Username, app.Name)
		return
	}
	rw := &webVpnRespWriter{ResponseWriter: w, req: r}
	rec := newAuditRecord(user, app, r.Host)
	proxy, err := webvpn.NewReverseProxy(app, r.Host)
	if err != nil {
		base.Error("WebVPN 反代构造失败:", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("WebVPN 配置错误"))
		return
	}
	proxy.ModifyResponse = withCORS(withAudit(proxy.ModifyResponse, rec, mgr.Audit(), rw), r, app)
	proxy.ServeHTTP(rw, r)
}
