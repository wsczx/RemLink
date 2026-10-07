package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn"
)

// WebVPN 路径模式（后置）入口：https://<VPN 主域名>/web/<协议>/<应用>/
//   - 鉴权直接复用门户会话（portal_session，主域名 host-only cookie），不经 webvpn_grant 兑换
//   - 授权复用 Authorized / AuthorizedUserIP（用户/组/IP 白名单；HTTP 路径白名单作用于前缀之后的子路径）
//   - HTTP 应用：反代并改写链接（webvpn.PathRewriter）
//   - 远程协议：/ 跳转前端远程访问页（mode=path）；/webvpn/* 接口交给主路由上的网关路径版路由
//
// 返回 true 表示请求已处理
func WebPathHandler(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, webvpn.WebPathRoot) {
		return false
	}
	// WebVPN 子域名下的 /web/* 属于被代理应用自身路径，由 WebVpnHandler 处理
	if prefix, ok := webVpnHostPrefix(r.Host); ok && prefix != "" {
		return false
	}
	proto, name, rest, ok := webvpn.ParseWebPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return true
	}
	isHTTP := proto == dbdata.WebVpnProtoHTTP
	// 远程协议的接口（/web/<协议>/<应用>/webvpn/*）交给网关路由
	if !isHTTP && strings.HasPrefix(rest, "/webvpn/") {
		return false
	}
	if rest == "" {
		// /web/<协议>/<应用> → 补斜杠，保证页面内相对路径落在前缀之下
		u := url.URL{Path: r.URL.Path + "/", RawQuery: r.URL.RawQuery}
		http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
		return true
	}
	if webvpn.WebPathHasDotSegment(rest) {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return true
	}
	// 主域名不使用 webvpn_grant：门户登录后回跳可能带上，清理后再访问，避免透传给后端
	if r.URL.Query().Get(webvpnGrantQuery) != "" {
		q := r.URL.Query()
		q.Del(webvpnGrantQuery)
		u := url.URL{Path: r.URL.Path, RawPath: r.URL.RawPath, RawQuery: q.Encode()}
		http.Redirect(w, r, u.String(), http.StatusFound)
		return true
	}

	user, logged := portalCurrentUser(r)
	if !logged || user == nil {
		webPathUnauthorized(w, r)
		return true
	}
	app, err := webvpn.GetManager().Apps().GetByName(name)
	if err != nil || app == nil {
		webVpnAppErrorPage(w, r, name, "notfound", "应用不存在或已删除")
		return true
	}
	if !webPathAccessible(app, proto) {
		webVpnAppErrorPage(w, r, name, "notfound", "该应用未开启路径方式访问")
		return true
	}
	if app.Status != 1 {
		webVpnAppErrorPage(w, r, name, "disabled", "应用已停用")
		return true
	}
	if !isHTTP {
		if !webvpn.AuthorizedUserIP(app, user, r) {
			webVpnForbiddenPage(w, r, user.Username, app.Name)
			return true
		}
		webPathRemoteEntry(w, r, app, rest)
		return true
	}
	if !webvpn.AuthorizedPath(app, user, r, rest) {
		webVpnForbiddenPage(w, r, user.Username, app.Name)
		return true
	}
	webPathProxy(w, r, app, user)
	return true
}

// 应用是否允许以路径方式、按 URL 中的协议访问
func webPathAccessible(app *dbdata.WebVpnApp, proto string) bool {
	if app.ProtocolOf() != proto {
		return false
	}
	_, path := app.AccessOf(base.GetCfg().WebVpnDomain != "")
	return path
}

// 未登录：页面导航跳门户登录（登录后回跳原路径），其余请求返回 401
func webPathUnauthorized(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		http.Redirect(w, r, "/portal?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// 远程协议的路径入口：/web/<协议>/<应用>/ 跳转前端远程访问页
func webPathRemoteEntry(w http.ResponseWriter, r *http.Request, app *dbdata.WebVpnApp, rest string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if rest != "/" {
		http.NotFound(w, r)
		return
	}
	q := url.Values{}
	q.Set("app", app.Name)
	q.Set("protocol", app.ProtocolOf())
	q.Set("mode", "path")
	http.Redirect(w, r, "/ui/#/webvpn/remote?"+q.Encode(), http.StatusFound)
}

// 路径模式 HTTP 反代：复用审计与反代实现，响应经 PathRewriter 改写
func webPathProxy(w http.ResponseWriter, r *http.Request, app *dbdata.WebVpnApp, user *dbdata.User) {
	mgr := webvpn.GetManager()
	rw := &webVpnRespWriter{ResponseWriter: w, req: r}
	rec := newAuditRecord(user, app, r.Host)
	proxy, err := webvpn.NewPathReverseProxy(app, r.Host, requestTLS(r))
	if err != nil {
		base.Error("WebVPN 反代构造失败:", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("WebVPN 配置错误"))
		return
	}
	proxy.ModifyResponse = withAudit(proxy.ModifyResponse, rec, mgr.Audit(), rw)
	proxy.ServeHTTP(rw, r)
}

// 返回应用的路径模式访问地址（相对主域名）
func webPathURL(app *dbdata.WebVpnApp) string {
	return webvpn.WebPathPrefix(app.ProtocolOf(), app.Name) + "/"
}

// 路径模式第三方登录回跳校验：必须是当前主域名（同 Host）下的 https /web/<协议>/<应用>/ 地址
func webPathSafeRedirect(u, host string) bool {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || !strings.EqualFold(parsed.Host, host) {
		return false
	}
	_, _, rest, ok := webvpn.ParseWebPath(parsed.Path)
	return ok && rest != "" && !webvpn.WebPathHasDotSegment(rest)
}
