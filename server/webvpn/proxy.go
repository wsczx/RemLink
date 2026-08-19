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

// RemLink 自有会话 cookie，反代转发给后端时必须剥离，避免网关令牌泄漏内网。
var remLinkSessionCookies = []string{
	sessionCookieName, // webvpn_session
	"portal_session",  // 门户会话（跨子域通配，同样须剥离）
	"auth-session-id", // WebAuth/OTP 认证会话
	"acSamlv2Token",   // SAML SSO 会话令牌
}

// 构造指向指定 WebVPN 应用的反向代理。originalHost 为原始子域，用于改写 Location/Set-Cookie。
func NewReverseProxy(app *dbdata.WebVpnApp, originalHost string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(app.Backend)
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
			if strings.HasPrefix(r.URL.Path, p) {
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

// 以 TCP 连接的 RemoteAddr 为准，不信任客户端伪造的 X-Forwarded-For：
// WebVPN 反代作为直接入口，审计与来源 IP 白名单必须基于真实连接
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
	drop := make(map[string]bool, len(remLinkSessionCookies))
	for _, n := range remLinkSessionCookies {
		drop[n] = true
	}
	var kept []*http.Cookie
	for _, c := range cookies {
		if drop[c.Name] {
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

// 清洗后端响应 Set-Cookie 头里的 Domain 属性（指向后端主机时剥离）
func scrubSetCookieDomain(resp *http.Response, backendHost string) {
	cookies := resp.Header.Values("Set-Cookie")
	if len(cookies) == 0 {
		return
	}
	resp.Header.Del("Set-Cookie")
	bh := strings.ToLower(stripPort(backendHost))
	for _, c := range cookies {
		idx := strings.Index(c, "Domain=")
		if idx < 0 {
			resp.Header.Add("Set-Cookie", c)
			continue
		}
		rest := c[idx+len("Domain="):]
		end := strings.IndexByte(rest, ';')
		dom := rest
		tail := ""
		if end >= 0 {
			dom = rest[:end]
			tail = rest[end:]
		}
		d := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(dom, ".")))
		if d != "" && (strings.EqualFold(d, bh) || strings.HasSuffix(bh, "."+d)) {
			c = strings.TrimRight(c[:idx], " ") + tail
			c = strings.TrimSpace(strings.TrimSuffix(c, ";"))
		}
		resp.Header.Add("Set-Cookie", c)
	}
}

// 判断 Location 主机是否应改写为 WebVPN 子域：仅当它等于后端主机或其后缀子域时
// 供测试直接验证改写规则
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
