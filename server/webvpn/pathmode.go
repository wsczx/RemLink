package webvpn

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/wsczx/remlink/dbdata"
)

// 路径模式（后置）：在 VPN 主域名下以 /web/<协议>/<应用>/ 访问 WebVPN 应用
//   - HTTP 应用：/web/http/<应用>/<后端路径> 反代到后端（链接改写见 rewrite.go）
//   - 远程协议：/web/<协议>/<应用>/ 跳转前端远程访问页；接口挂在 /web/<协议>/<应用>/webvpn/*（与子域名模式的 /webvpn/* 一一对应）
const WebPathRoot = "/web/"

// 路径模式下可出现的协议段
func webPathProtoValid(p string) bool {
	switch p {
	case dbdata.WebVpnProtoHTTP, dbdata.WebVpnProtoSSH, dbdata.WebVpnProtoSFTP, dbdata.WebVpnProtoSMB,
		dbdata.WebVpnProtoFTP, dbdata.WebVpnProtoRDP, dbdata.WebVpnProtoVNC:
		return true
	}
	return false
}

// 应用名段：与 dbdata 的命名规则一致（小写字母 / 数字 / 中划线），大小写敏感以避免同名歧义
func WebPathAppValid(name string) bool {
	if name == "" || len(name) > 60 {
		return false
	}
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return false
	}
	return true
}

// 返回路径模式前缀 /web/<协议>/<应用>（不含结尾斜杠）
func WebPathPrefix(proto, app string) string {
	return WebPathRoot + proto + "/" + app
}

// 解析 /web/<协议>/<应用>[/rest]。rest 为应用内路径（以 / 开头；恰为前缀本身时为空）
func ParseWebPath(p string) (proto, app, rest string, ok bool) {
	tail, found := strings.CutPrefix(p, WebPathRoot)
	if !found {
		return "", "", "", false
	}
	proto, tail, _ = strings.Cut(tail, "/")
	if !webPathProtoValid(proto) {
		return "", "", "", false
	}
	app, rest, hasRest := strings.Cut(tail, "/")
	if !WebPathAppValid(app) {
		return "", "", "", false
	}
	if hasRest {
		rest = "/" + rest
	}
	return proto, app, rest, true
}

// 应用内路径是否含 . / .. 段（拒绝，避免路径白名单被 /api/../admin 绕过）
func WebPathHasDotSegment(rest string) bool {
	for seg := range strings.SplitSeq(rest, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}

// 请求是否来自路径模式下被代理的 HTTP 应用页面（Referer 位于 /web/http/ 下）。
// 被代理页面与门户同源，门户写接口与远程协议接口据此拒绝来自它们的请求（纵深防御，主防线是 CSP connect-src）
func FromProxiedPage(r *http.Request) bool {
	ref := r.Header.Get("Referer")
	if ref == "" {
		return false
	}
	u, err := url.Parse(ref)
	if err != nil {
		return true
	}
	if u.Host != "" && !strings.EqualFold(u.Host, r.Host) {
		return false
	}
	return strings.HasPrefix(u.Path, WebPathRoot+dbdata.WebVpnProtoHTTP+"/")
}

// 路径模式下的请求级完整授权：路径白名单作用于 /web/http/<应用>/ 之后的子路径
func AuthorizedPath(app *dbdata.WebVpnApp, user *dbdata.User, r *http.Request, appPath string) bool {
	return authorized(app, user, r, appPath)
}
