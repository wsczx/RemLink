package handler

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/webvpn"
)

// WebVPN 子域的反向代理入口：按子域（host 第一段）匹配应用
// 串起「认证 → 免登兑换 → 续期 → 授权 → 反代 → 审计」完整链路
// 返回 true 表示请求已被本处理器使用（是 WebVPN 子域请求），false 表示应交由其它路由处理
func WebVpnHandler(services Services, w http.ResponseWriter, r *http.Request) bool {
	mgr := services.Webvpn
	if base.GetCfg().WebVpnDomain == "" {
		return false
	}
	prefix, isWebVpnHost := webVpnHostPrefix(r.Host)
	if !isWebVpnHost {
		return false
	}

	if prefix == "" {
		return false
	}
	// WebVPN 自有端点（/webvpn/*，如登录/登出/me）不走代理，交由 initRoute 处理
	if strings.HasPrefix(r.URL.Path, "/webvpn/") {
		return false
	}
	// 门户相关路径一律禁止在 WebVPN 子域下访问（防止跨子域携带通配 portal_session cookie 越权调用门户接口）：
	// 仅放行 GET /portal（登录页）与白名单内的登录前置接口（见 portalLoginEndpoints）
	if r.URL.Path == "/portal" && r.Method == http.MethodGet {
		return false
	}
	if r.URL.Path == "/portal" || strings.HasPrefix(r.URL.Path, "/portal/") {
		if isPortalLoginEndpoint(services.Portal, r.URL.Path, r.Method) {
			return false
		}
		http.Error(w, "Forbidden", http.StatusForbidden)
		return true
	}

	app, err := dbdata.WebVpn.GetAppByName(prefix)
	// 仅对明确开启跨站访问且配置了来源白名单的应用放宽会话 Cookie
	r = webvpn.WithCrossSiteCookie(r, err == nil && app != nil && app.Status == 1 && app.AllowCrossSite && len(app.CorsAllowedOrigins) > 0)

	// 跨域预检（OPTIONS）由网关直接回应 CORS 头，不走认证、不反代后端
	if r.Method == http.MethodOptions {
		if err != nil || app == nil || app.Status != 1 {
			w.WriteHeader(http.StatusNotFound)
			return true
		}
		webVpnHandlePreflight(w, r, app)
		return true
	}

	// 认证优先：已登录 WebVPN 会话用户直接放行
	user, ok := services.Webvpn.Session().UserFromRequest(r)
	grantRedirect := r.URL.Query().Get(webvpnGrantQuery) != ""
	if !ok || user == nil {
		// 门户登录后下发的 webvpn_grant 一次性换取正式会话（并写入会话 cookie）
		if token, gu, exchanged := mgr.Session().ExchangeGrant(w, r); exchanged {
			// 注入请求 cookie，使同一次请求内后续 webVpnProxy 的 CurrentUser 能读到，
			// 避免兑换成功却又误判未登录而跳登录页
			r.AddCookie(&http.Cookie{Name: webVpnSessionCookie, Value: token})
			user = gu
			ok = true
		}
	}
	if grantRedirect {
		w.Header().Set("Referrer-Policy", "no-referrer")
		u := *r.URL
		q := u.Query()
		q.Del(webvpnGrantQuery)
		u.Scheme = ""
		u.Host = ""
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
		return true
	}
	// 未登录 /ui/ 由 RemLink 前端渲染登录页
	// 登录后 /ui/ 透传至后端，以支持 JumpServer 等后端应用自身使用 /ui/ 的前端入口
	if strings.HasPrefix(r.URL.Path, "/ui/") {
		if app != nil && err == nil {
			if !ok || user == nil {
				return false
			}
		} else {
			return false
		}
	}
	if !ok || user == nil {
		// 门户会话有效但免登兑换失败（权限中途被取消 / grant 过期 / 会话已被吊销）：
		// 直接渲染无权限提示页，而非跳登录页。否则门户已登录的前端会自动回跳、
		// 后端又判定未登录再次跳转，形成高频率刷新死循环
		if puser, pok := portalCurrentUser(r); pok && puser != nil {
			if webVpnWriteCrossOriginStatus(w, r, app, http.StatusForbidden) {
				return true
			}
			webVpnForbiddenPage(mgr, w, r, puser.Username, "")
			return true
		}
		// 跨站接口未登录时返回可识别的 401，避免把接口请求重定向为登录 HTML
		// 普通浏览器直接打开 WebVPN 页面仍跳转到当前子域的登录页
		if webVpnWriteCrossOriginStatus(w, r, app, http.StatusUnauthorized) {
			return true
		}
		// 完全未登录：跳转到当前 WebVPN 子域自身的登录页（/ui/#/portal，
		// 由门户登录卡片前端承载），登录成功后门户下发一次性 webvpn_grant，
		// 回域后由 ExchangeGrant 自动兑换正式会话，无需重复登录
		http.Redirect(w, r, webVpnLoginURL(r), http.StatusFound)
		return true
	}

	// 认证与免登兑换已完成，后续查应用/续期/授权/反代/审计统一交给 webVpnProxy
	webVpnProxy(mgr, w, r, prefix)
	return true
}

// 判断 host 是否属于 WebVPN 子域，返回 (子域前缀, 是否匹配)
// DNS 主机名大小写不敏感，统一转小写比较，且前缀以小写返回，避免大写 Host 绕过 WebVPN 分支
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
	if utils.IsTLS(r) {
		scheme = "https"
	}
	redirect := r.URL.RequestURI()
	return scheme + "://" + r.Host + "/ui/#/portal?redirect=" + url.QueryEscape(redirect)
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
			((u.Scheme == "https" && utils.IsTLS(r)) || (u.Scheme == "http" && !utils.IsTLS(r)))
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

// WebVPN 子域名登录所放行的门户接口唯一来源：
// initRoute 据此注册路由（见 server.go），WebVpnHandler 据此判断放行。新增子域名登录
// 必需的门户接口只需改此处
func portalLoginEndpoints(p *PortalService) []portalLoginEndpoint {
	return []portalLoginEndpoint{
		{"/portal/api/login", http.MethodPost, p.Login},
		{"/portal/api/verify", http.MethodPost, p.Verify},
		{"/portal/api/sms/send", http.MethodPost, p.SendSMS},
		{"/portal/api/sms/verify", http.MethodPost, p.VerifySMS},
		{"/portal/api/login-config", http.MethodGet, p.LoginConfig},
		{"/portal/api/me", http.MethodGet, p.Me},
		{"/portal/api/otp/status", http.MethodGet, p.OTPStatus},
		{"/portal/api/sso", http.MethodGet, p.SSOLogin},
	}
}

// 去掉 host:port 中的端口部分，仅保留主机名
func stripPort(host string) string {
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		if strings.Contains(host, "]") {
			return host[:strings.LastIndexByte(host, ':')]
		}
		return host[:i]
	}
	return host
}

// 提取 host:port 或 scheme://host:port 中的端口部分（不含冒号），无端口返回空
func portOf(host string) string {
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		if strings.Contains(host, "]") {
			return host[strings.LastIndexByte(host, ':')+1:]
		}
		return host[i+1:]
	}
	return ""
}

// WebVPN 会话 cookie 的规范名称
const (
	webVpnSessionCookie = "webvpn_session"
	webvpnGrantQuery    = "webvpn_grant"
)

// WebVPN 反向代理的内部执行体。负责：认证取用户 → 查应用配置
// → 滑动续期 → 请求级授权 → 构造反代 → 投递审计 → 转发。WebVpnHandler 完成
// 免登兑换后调用本函数，测试亦直接调用以覆盖授权/审计逻辑，二者共用同一实现
func webVpnProxy(mgr *webvpn.WebVPNManager, w http.ResponseWriter, r *http.Request, prefix string) {
	user, _ := mgr.Session().UserFromRequest(r)
	if user == nil {
		// 未登录：跳转到当前 WebVPN 子域自身的登录页，与 WebVpnHandler 入口一致，
		// 避免跳错父域/丢失端口。登录成功后由 ExchangeGrant 自动兑换会话
		http.Redirect(w, r, webVpnLoginURL(r), http.StatusFound)
		return
	}
	app, err := mgr.Apps().GetByName(prefix)
	if err != nil || app == nil {
		webVpnAppErrorPage(mgr, w, r, prefix, "notfound", "应用不存在或已删除")
		return
	}
	// 滑动续期（距签发超过 TTL-1h 时重签）
	if _, err := mgr.Session().Renew(w, r); err != nil {
		base.Error("WebVPN 会话续期失败:", err)
	}
	// 请求级完整授权（用户/组/IP/路径白名单）
	if !webvpn.RequestAuthorized(app, user, r) {
		webVpnForbiddenPage(mgr, w, r, user.Username, app.Name)
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
