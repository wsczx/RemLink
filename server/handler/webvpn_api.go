package handler

import (
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/webvpn"
	"net/http"
)

// 返回当前 WebVPN 会话用户基本信息
func webVpnMe(mgr *webvpn.WebVPNManager, w http.ResponseWriter, r *http.Request) {
	user, ok := mgr.Session().UserFromRequest(r)
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

// 单点登出：清除 WebVPN 会话 cookie 并吊销当前 jti。仅影响 WebVPN，门户登录态不受影响
// GET 返回一个最小退出确认页（同源表单 POST 到自身），方便用户在子域名下直接退出
func webVpnLogout(mgr *webvpn.WebVPNManager, w http.ResponseWriter, r *http.Request) {
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
	mgr.Session().RevokeGrantFromRequest(r)
	mgr.Session().RevokeFromRequest(r)
	mgr.Session().ClearCookie(w, r)
	mgr.Session().ClearGrantCookie(w, r)
	portalOK(w, nil)
}

// 供门户"我的应用"展示当前用户有权访问的 WebVPN 应用列表
func webVpnMyApps(mgr *webvpn.WebVPNManager, w http.ResponseWriter, r *http.Request) {
	user, ok := mgr.Session().UserFromRequest(r)
	if !ok || user == nil {
		user, ok = portalCurrentUser(r)
		if !ok || user == nil {
			portalError(w, "未登录")
			return
		}
	}
	apps, err := mgr.Apps().ForUser(user)
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

// 子域名可放行的门户登录前置接口
// 仅在未登录状态下登录流程必需、不依赖已登录 portal_session，放行它们保证
// WebVPN 子域名登录页正常加载配置、完成登录并检测登录态；其余门户写接口仍由
// 403 拦截，杜绝跨子域携带门户 cookie 越权调用
type portalLoginEndpoint struct {
	path    string
	method  string
	handler func(http.ResponseWriter, *http.Request)
}

// 判断子域名下是否放行某门户登录接口（以 portalLoginEndpoints 为唯一来源）
func isPortalLoginEndpoint(p *PortalService, path, method string) bool {
	for _, ep := range portalLoginEndpoints(p) {
		if ep.path == path && ep.method == method {
			return true
		}
	}
	return false
}
