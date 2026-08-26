package webvpn

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 反代请求不得向后端转发 RemLink 会话 cookie
var remLinkSessionCookies = []string{
	sessionCookieName, // webvpn_session
	"portal_session",  // 门户会话（跨子域通配，同样须剥离）
	grantCookieName,   // 一次性 WebVPN 免登授权，不得转发给内网后端
	"auth-session-id", // WebAuth/OTP 认证会话
	"acSamlv2Token",   // SAML SSO 会话令牌
}

// 构造 WebVPN 应用的反向代理
func NewReverseProxy(app *dbdata.WebVpnApp, originalHost string) (*httputil.ReverseProxy, error) {
	target, err := dbdata.ParseWebVpnBackendURL(app.Backend)
	if err != nil {
		return nil, err
	}
	proxy := &httputil.ReverseProxy{
		// 后端为自签/内网证书时跳过 TLS 校验（仅当后端是 https 且应用开启 SkipVerify）
		Transport: backendTransport(app.SkipVerify && target.Scheme == "https"),
		Director: func(req *http.Request) {
			req.Header.Del("X-Forwarded-For")
			req.Header.Del("X-Forwarded-Proto")
			req.Header.Del("X-Forwarded-Host")
			req.Header.Del("X-Real-IP")
			req.Header.Del("X-RemLink-WebVpn")

			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			if app.HostRewrite != "" {
				req.Host = app.HostRewrite
			} else {
				req.Host = target.Host
			}
			req.Header.Set("X-Forwarded-Proto", "https")
			req.Header.Set("X-Forwarded-Host", originalHost)
			req.Header.Set("Cookie", StripRemLinkCookies(req.Cookies()))
			req.Header.Set("X-RemLink-WebVpn", "1")
		},
		ModifyResponse: func(resp *http.Response) error {
			scrubBackendCORSHeaders(resp.Header)
			if loc := resp.Header.Get("Location"); loc != "" {
				if u, e := url.Parse(loc); e == nil && u.Host != "" {
					if HostMatchesBackend(u.Host, target.Host) {
						u.Host = originalHost
						resp.Header.Set("Location", u.String())
					}
				}
			}
			scrubSetCookieDomain(resp, target.Host)
			resp.Header.Del("Content-Security-Policy")
			resp.Header.Del("Cross-Origin-Embedder-Policy")
			resp.Header.Del("Cross-Origin-Resource-Policy")
			resp.Header.Del("X-Frame-Options")
			return nil
		},
		ErrorHandler: func(ew http.ResponseWriter, req *http.Request, e error) {
			origin := req.Header.Get("Origin")
			if origin != "" && dbdata.WebVpnCorsOriginAllowed(app, origin) {
				ew.Header().Set("Access-Control-Allow-Origin", origin)
				ew.Header().Set("Access-Control-Allow-Credentials", "true")
				ew.Header().Set("Vary", "Origin")
			}
			if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) ||
				strings.Contains(e.Error(), "client disconnected") ||
				strings.Contains(e.Error(), "connection reset by peer") {
				base.Info("WebVPN 反代中断（客户端取消）:", app.Name, e)
			} else {
				base.Error("WebVPN 反代错误:", app.Name, e)
			}
			ew.WriteHeader(http.StatusBadGateway)
			ew.Write([]byte("WebVPN 后端连接失败"))
		},
	}
	return proxy, nil
}

// 校验用户/组/IP/路径白名单（请求级完整授权）。
func pathAllowed(path, prefix string) bool {
	path = strings.TrimSpace(path)
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return false
	}
	if prefix != "/" {
		prefix = strings.TrimRight(prefix, "/")
	}
	return path == prefix || (prefix == "/" && strings.HasPrefix(path, "/")) || strings.HasPrefix(path, prefix+"/")
}

func Authorized(app *dbdata.WebVpnApp, user *dbdata.User, r *http.Request) bool {
	if app.Status != 1 {
		return false
	}
	if !dbdata.WebVpnUserAllowed(app, user) {
		return false
	}
	if len(app.IpAllowList) > 0 {
		ip := net.ParseIP(realClientIP(r))
		if ip == nil {
			return false
		}
		if !ipInAllowList(ip, app.IpAllowList) {
			return false
		}
	}
	if len(app.AllowPath) > 0 {
		ok := false
		for _, p := range app.AllowPath {
			if pathAllowed(r.URL.Path, p) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// 使用 TCP 连接地址，避免信任客户端提供的 X-Forwarded-For
func RealClientIP(r *http.Request) string { return realClientIP(r) }

func realClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// 剥离 RemLink 自有会话 cookie（含门户会话），避免透传内网后端
func StripRemLinkCookies(cookies []*http.Cookie) string {
	var kept []*http.Cookie
	for _, c := range cookies {
		if isRemLinkSessionCookie(c.Name) {
			continue
		}
		kept = append(kept, c)
	}
	if len(kept) == 0 {
		return ""
	}
	var b strings.Builder
	for i, c := range kept {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(c.Name)
		b.WriteString("=")
		b.WriteString(c.Value)
	}
	return b.String()
}

func scrubBackendCORSHeaders(header http.Header) {
	for _, name := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Credentials",
		"Access-Control-Allow-Headers",
		"Access-Control-Allow-Methods",
		"Access-Control-Expose-Headers",
		"Access-Control-Max-Age",
	} {
		header.Del(name)
	}
}

func isRemLinkSessionCookie(name string) bool {
	for _, reserved := range remLinkSessionCookies {
		if strings.EqualFold(name, reserved) {
			return true
		}
	}
	return false
}

// 清洗后端响应 Set-Cookie：阻止覆盖 RemLink 会话，并剥离指向后端的 Domain 属性
func scrubSetCookieDomain(resp *http.Response, backendHost string) {
	cookies := resp.Header.Values("Set-Cookie")
	if len(cookies) == 0 {
		return
	}
	resp.Header.Del("Set-Cookie")
	bh := strings.ToLower(stripPort(backendHost))
	for _, c := range cookies {
		nameEnd := strings.IndexByte(c, '=')
		if nameEnd > 0 && isRemLinkSessionCookie(strings.TrimSpace(c[:nameEnd])) {
			continue
		}
		parts := strings.Split(c, ";")
		kept := parts[:1]
		for _, part := range parts[1:] {
			key, value, ok := strings.Cut(part, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "Domain") {
				kept = append(kept, part)
				continue
			}
			d := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "."))
			if d == "" || (!strings.EqualFold(d, bh) && !strings.HasSuffix(bh, "."+d)) {
				kept = append(kept, part)
			}
		}
		resp.Header.Add("Set-Cookie", strings.TrimSpace(strings.Join(kept, ";")))
	}
}

// 判断 Location 主机是否属于后端主机或其子域
func HostMatchesBackend(locHost, backendHost string) bool {
	lh := strings.ToLower(stripPort(locHost))
	bh := strings.ToLower(stripPort(backendHost))
	if lh == "" || bh == "" {
		return false
	}
	if lh == bh {
		return true
	}
	return strings.HasSuffix(lh, "."+bh)
}

// 后端 Transport 按 skipVerify 复用两个共享实例，避免每请求新建连接池导致 FD 耗尽。
var (
	transportMu       sync.Mutex
	transportNormal   *http.Transport
	transportInsecure *http.Transport
)

// 返回反代后端用的 http.Transport（进程内复用）。skipVerify 时跳过对端证书校验
func backendTransport(skipVerify bool) *http.Transport {
	transportMu.Lock()
	defer transportMu.Unlock()
	if skipVerify {
		if transportInsecure == nil {
			t := http.DefaultTransport.(*http.Transport).Clone()
			t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			transportInsecure = t
		}
		return transportInsecure
	}
	if transportNormal == nil {
		transportNormal = http.DefaultTransport.(*http.Transport).Clone()
	}
	return transportNormal
}

func ipInAllowList(ip net.IP, list []string) bool {
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			_, cidr, err := net.ParseCIDR(item)
			if err == nil && cidr.Contains(ip) {
				return true
			}
			continue
		}
		if ip.String() == item {
			return true
		}
	}
	return false
}
