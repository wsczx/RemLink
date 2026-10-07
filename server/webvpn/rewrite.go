package webvpn

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/wsczx/remlink/base"
	"golang.org/x/net/html"
)

// 路径模式 HTTP 应用的链接改写器：把后端的绝对路径 / 后端主机地址映射到 /web/http/<应用>/ 前缀下。
//
// 覆盖：Location / Content-Location / Refresh 响应头、Set-Cookie（Path 加前缀、去掉 Domain、拒绝 RemLink 自有 cookie）、
// text/html（URL 类属性、srcset、<base>、<meta refresh>、style 属性与 <style> 内的 CSS）、text/css（url() 与 @import）。
// 不覆盖：JS 运行时拼接的绝对路径（fetch('/api')、location='/x' 等），这些请求会落到前缀之外并被 CSP 拦截。
type PathRewriter struct {
	prefix       string   // /web/http/<应用>（不含结尾斜杠）
	publicHost   string   // VPN 主域名 Host（含端口）
	scheme       string   // 对外协议 https / http
	backendHosts []string // 后端主机（含 Host 改写值）
}

func NewPathRewriter(prefix, publicHost string, tls bool, backendHost, hostRewrite string) *PathRewriter {
	scheme := "http"
	if tls {
		scheme = "https"
	}
	hosts := []string{backendHost}
	if hostRewrite != "" {
		hosts = append(hosts, hostRewrite)
	}
	return &PathRewriter{prefix: prefix, publicHost: publicHost, scheme: scheme, backendHosts: hosts}
}

// CSS / HTML 缓冲上限：超出后原样透传剩余内容
const rewriteMaxCSS = 8 << 20

// ---------------- URL ----------------

// 是否为「本应用自身」主机：后端主机（含子域）或 VPN 主域名本身
func (p *PathRewriter) ownHost(h string) bool {
	if h == "" {
		return false
	}
	for _, bh := range p.backendHosts {
		if HostMatchesBackend(h, bh) {
			return true
		}
	}
	return strings.EqualFold(stripPort(h), stripPort(p.publicHost))
}

// 给站内绝对路径加前缀（已在前缀下则不变，保证幂等）
func (p *PathRewriter) prefixPath(path string) string {
	if path == "" {
		return p.prefix + "/"
	}
	if path == p.prefix || strings.HasPrefix(path, p.prefix+"/") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return p.prefix + path
}

// 规范化以便判断：去掉浏览器解析时会忽略的 TAB/换行，首尾空白，以及开头的反斜杠（浏览器视 \ 同 /）
func normalizeURLRef(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] == '/' || s[i] == '\\') {
		i++
	}
	if i > 0 {
		s = strings.Repeat("/", i) + s[i:]
	}
	return s
}

// 改写单个 URL 引用。absolute=true 时改写结果为对外绝对地址（用于 Location），否则为站内绝对路径
// strict=true（响应头）：无法解析的协议相对地址一律收敛到应用根，防止借畸形 URL 跳出前缀
func (p *PathRewriter) rewriteRef(raw string, absolute, strict bool) string {
	s := normalizeURLRef(raw)
	if s == "" {
		return raw
	}
	switch {
	case strings.HasPrefix(s, "//"):
		u, err := url.Parse(s)
		if err != nil {
			if strict {
				return p.prefix + "/"
			}
			return raw
		}
		if !p.ownHost(u.Host) {
			return raw
		}
		return p.fromURL(u, absolute)
	case strings.HasPrefix(s, "/"):
		u, err := url.Parse(s)
		if err != nil {
			if strict {
				return p.prefix + "/"
			}
			return raw
		}
		return p.fromURL(u, false)
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return raw
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ws", "wss":
	default:
		return raw
	}
	if !p.ownHost(u.Host) {
		return raw
	}
	return p.fromURL(u, absolute)
}

func (p *PathRewriter) fromURL(u *url.URL, absolute bool) string {
	out := &url.URL{
		Path:     p.prefixPath(u.Path),
		RawQuery: u.RawQuery,
		Fragment: u.Fragment,
	}
	if u.RawPath != "" {
		out.RawPath = p.prefixPath(u.RawPath)
	}
	if u.ForceQuery {
		out.ForceQuery = true
	}
	if absolute {
		out.Scheme = p.scheme
		if s := strings.ToLower(u.Scheme); s == "ws" || s == "wss" {
			out.Scheme = map[string]string{"https": "wss", "http": "ws"}[p.scheme]
		}
		out.Host = p.publicHost
	}
	return out.String()
}

// 站内 URL（HTML/CSS 中的引用）
func (p *PathRewriter) URL(raw string) string { return p.rewriteRef(raw, false, false) }

// 响应头 Location / Content-Location
func (p *PathRewriter) Location(raw string) string {
	s := normalizeURLRef(raw)
	if strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//") {
		return p.rewriteRef(raw, false, true)
	}
	return p.rewriteRef(raw, true, true)
}

var refreshURLRe = regexp.MustCompile(`(?i)^(\s*[0-9.]*\s*[;,]\s*(?:url\s*=\s*)?)(['"]?)([^'"]*)(['"]?)(.*)$`)

// Refresh 头 / <meta http-equiv=refresh> 的 content：5; url=/x
func (p *PathRewriter) Refresh(v string) string {
	m := refreshURLRe.FindStringSubmatch(v)
	if m == nil || strings.TrimSpace(m[3]) == "" {
		return v
	}
	return m[1] + m[2] + p.rewriteRef(m[3], false, true) + m[4] + m[5]
}

// ---------------- Cookie ----------------

// 改写后端 Set-Cookie：拒绝 RemLink 自有 cookie 名；去掉 Domain（只落在 VPN 主域名本身）；
// Path 映射到应用前缀下（作用于 / 的 cookie 变为作用于 /web/http/<应用>/）。返回 false 表示丢弃
func (p *PathRewriter) SetCookie(raw string) (string, bool) {
	parts := strings.Split(raw, ";")
	name, _, ok := strings.Cut(parts[0], "=")
	name = strings.TrimSpace(name)
	if !ok || name == "" || isRemLinkSessionCookie(name) {
		return "", false
	}
	kept := []string{strings.TrimSpace(parts[0])}
	for _, part := range parts[1:] {
		k, v, _ := strings.Cut(part, "=")
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "domain":
			continue
		case "path":
			v = strings.TrimSpace(v)
			if !strings.HasPrefix(v, "/") {
				v = "/"
			}
			kept = append(kept, "Path="+p.prefixPath(v))
		default:
			if t := strings.TrimSpace(part); t != "" {
				kept = append(kept, t)
			}
		}
	}
	// 未指定 Path 时浏览器取请求路径的目录，已天然位于前缀之下
	return strings.Join(kept, "; "), true
}

// ---------------- CSS ----------------

var (
	cssURLRe    = regexp.MustCompile(`(?i)url\(\s*(?:"([^"]*)"|'([^']*)'|([^'"\)\s]*))\s*\)`)
	cssImportRe = regexp.MustCompile(`(?i)@import\s*(?:"([^"]*)"|'([^']*)')`)
)

// 改写 CSS 中的 url(...) 与 @import "..."
func (p *PathRewriter) CSS(b []byte) []byte {
	b = cssURLRe.ReplaceAllFunc(b, func(m []byte) []byte {
		sm := cssURLRe.FindSubmatch(m)
		switch {
		case sm[1] != nil && len(sm[1]) > 0:
			return []byte(`url("` + p.URL(string(sm[1])) + `")`)
		case sm[2] != nil && len(sm[2]) > 0:
			return []byte(`url('` + p.URL(string(sm[2])) + `')`)
		case len(sm[3]) > 0:
			return []byte(`url(` + p.URL(string(sm[3])) + `)`)
		}
		return m
	})
	return cssImportRe.ReplaceAllFunc(b, func(m []byte) []byte {
		sm := cssImportRe.FindSubmatch(m)
		if len(sm[1]) > 0 {
			return []byte(`@import "` + p.URL(string(sm[1])) + `"`)
		}
		if len(sm[2]) > 0 {
			return []byte(`@import '` + p.URL(string(sm[2])) + `'`)
		}
		return m
	})
}

func (p *PathRewriter) rewriteCSSStream(dst io.Writer, src io.Reader) error {
	b, err := io.ReadAll(io.LimitReader(src, rewriteMaxCSS+1))
	if err != nil {
		return err
	}
	if len(b) > rewriteMaxCSS {
		if _, err := dst.Write(b); err != nil {
			return err
		}
		_, err = io.Copy(dst, src)
		return err
	}
	_, err = dst.Write(p.CSS(b))
	return err
}

// ---------------- HTML ----------------

// 取值为单个 URL 的属性
var htmlURLAttrs = map[string]bool{
	"href": true, "src": true, "action": true, "formaction": true, "data": true, "poster": true,
	"background": true, "cite": true, "longdesc": true, "usemap": true, "manifest": true,
	"codebase": true, "icon": true, "xlink:href": true, "lowsrc": true, "dynsrc": true,
}

// 改写逗号分隔的 srcset：url [描述符], url [描述符]
func (p *PathRewriter) srcset(v string) string {
	items := strings.Split(v, ",")
	for i, it := range items {
		t := strings.TrimSpace(it)
		if t == "" {
			continue
		}
		u, desc, _ := strings.Cut(t, " ")
		nu := p.URL(u)
		if desc != "" {
			nu += " " + strings.TrimSpace(desc)
		}
		items[i] = nu
	}
	return strings.Join(items, ", ")
}

// 流式改写 HTML：未改动的 token 原样输出（Raw），仅重写含站内绝对路径的标签
func (p *PathRewriter) RewriteHTML(dst io.Writer, src io.Reader) error {
	z := html.NewTokenizer(src)
	inStyle := false
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if raw := z.Raw(); len(raw) > 0 {
				if _, err := dst.Write(raw); err != nil {
					return err
				}
			}
			if z.Err() == io.EOF {
				return nil
			}
			return z.Err()
		case html.StartTagToken, html.SelfClosingTagToken:
			raw := append([]byte(nil), z.Raw()...)
			name, hasAttr := z.TagName()
			tag := string(name)
			inStyle = tt == html.StartTagToken && tag == "style"
			if !hasAttr {
				if _, err := dst.Write(raw); err != nil {
					return err
				}
				continue
			}
			out, changed := p.rewriteTag(z, tag, tt == html.SelfClosingTagToken)
			if !changed {
				out = raw
			}
			if _, err := dst.Write(out); err != nil {
				return err
			}
		case html.TextToken:
			raw := z.Raw()
			if inStyle {
				raw = p.CSS(raw)
			}
			if _, err := dst.Write(raw); err != nil {
				return err
			}
		case html.EndTagToken:
			inStyle = false
			if _, err := dst.Write(z.Raw()); err != nil {
				return err
			}
		default:
			if _, err := dst.Write(z.Raw()); err != nil {
				return err
			}
		}
	}
}

type htmlAttr struct{ key, val string }

func (p *PathRewriter) rewriteTag(z *html.Tokenizer, tag string, selfClosing bool) ([]byte, bool) {
	var attrs []htmlAttr
	for {
		k, v, more := z.TagAttr()
		attrs = append(attrs, htmlAttr{string(k), string(v)})
		if !more {
			break
		}
	}
	refresh := false
	if tag == "meta" {
		for _, a := range attrs {
			if a.key == "http-equiv" && strings.EqualFold(strings.TrimSpace(a.val), "refresh") {
				refresh = true
			}
		}
	}
	changed := false
	out := attrs[:0]
	for _, a := range attrs {
		nv := a.val
		switch {
		case htmlURLAttrs[a.key]:
			nv = p.URL(a.val)
		case a.key == "srcset" || a.key == "imagesrcset":
			nv = p.srcset(a.val)
		case a.key == "style":
			nv = string(p.CSS([]byte(a.val)))
		case a.key == "content" && refresh:
			nv = p.Refresh(a.val)
		case a.key == "integrity" && tag == "link":
			// 样式表内容经过改写，SRI 摘要必然失配
			changed = true
			continue
		}
		if nv != a.val {
			changed = true
		}
		out = append(out, htmlAttr{a.key, nv})
	}
	if !changed {
		return nil, false
	}
	var b bytes.Buffer
	b.WriteByte('<')
	b.WriteString(tag)
	for _, a := range out {
		b.WriteByte(' ')
		b.WriteString(a.key)
		if a.val != "" {
			b.WriteString(`="`)
			b.WriteString(html.EscapeString(a.val))
			b.WriteByte('"')
		}
	}
	if selfClosing {
		b.WriteString(" /")
	}
	b.WriteByte('>')
	return b.Bytes(), true
}

// ---------------- 请求 / 响应 ----------------

// 转发前：去掉路径前缀、Referer 映射回后端路径、仅协商可解码的 gzip
func (p *PathRewriter) rewriteRequest(req *http.Request) {
	rest := strings.TrimPrefix(req.URL.Path, p.prefix)
	if rest == "" || rest[0] != '/' {
		rest = "/" + rest
	}
	req.URL.Path = rest
	if req.URL.RawPath != "" {
		raw := strings.TrimPrefix(req.URL.RawPath, p.prefix)
		if raw == "" || raw[0] != '/' {
			raw = "/" + raw
		}
		req.URL.RawPath = raw
	}
	if ref := req.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && strings.EqualFold(u.Host, p.publicHost) && strings.HasPrefix(u.Path, p.prefix+"/") {
			u.Path = strings.TrimPrefix(u.Path, p.prefix)
			u.RawPath = ""
			req.Header.Set("Referer", u.String())
		}
	}
	if strings.Contains(strings.ToLower(req.Header.Get("Accept-Encoding")), "gzip") {
		req.Header.Set("Accept-Encoding", "gzip")
	} else {
		req.Header.Del("Accept-Encoding")
	}
	req.Header.Set("X-Forwarded-Prefix", p.prefix)
}

// 路径模式的同源缓解 CSP：被代理页面的连接 / 表单 / 子框架 / Worker 只能指向本应用前缀，
// 防止其脚本以门户同源身份调用 /api/v1/portal/*、/web/<协议>/<其它应用>/ 等 RemLink 接口
func (p *PathRewriter) CSP() string {
	ws := "wss"
	if p.scheme == "http" {
		ws = "ws"
	}
	scope := p.scheme + "://" + p.publicHost + p.prefix + "/"
	wsScope := ws + "://" + p.publicHost + p.prefix + "/"
	return "connect-src " + scope + " " + wsScope + " blob: data:; " +
		"form-action " + scope + "; " +
		"frame-src " + scope + " blob: data:; " +
		"worker-src " + scope + " blob:; " +
		"manifest-src " + scope + "; " +
		"object-src 'none'; " +
		"base-uri " + scope + "; " +
		"frame-ancestors 'self'"
}

func (p *PathRewriter) rewriteResponse(resp *http.Response) error {
	h := resp.Header
	if loc := h.Get("Location"); loc != "" {
		h.Set("Location", p.Location(loc))
	}
	if cl := h.Get("Content-Location"); cl != "" {
		h.Set("Content-Location", p.Location(cl))
	}
	if rf := h.Get("Refresh"); rf != "" {
		h.Set("Refresh", p.Refresh(rf))
	}
	if cookies := h.Values("Set-Cookie"); len(cookies) > 0 {
		h.Del("Set-Cookie")
		for _, c := range cookies {
			if nc, ok := p.SetCookie(c); ok {
				h.Add("Set-Cookie", nc)
			}
		}
	}
	// 禁止后端扩大 Service Worker 作用域 / 清空整站存储（会波及门户）
	h.Del("Service-Worker-Allowed")
	h.Del("Clear-Site-Data")
	h.Set("Content-Security-Policy", p.CSP())
	// 与 RemLink 页面（COOP same-origin）取值不同，互相打开时切换浏览上下文组，切断 window.opener 脚本访问
	h.Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
	h.Set("X-RemLink-WebVpn-Mode", "path")

	if resp.StatusCode == http.StatusSwitchingProtocols || resp.StatusCode == http.StatusNoContent ||
		resp.StatusCode == http.StatusNotModified || (resp.Request != nil && resp.Request.Method == http.MethodHead) {
		return nil
	}
	kind := rewriteKind(h.Get("Content-Type"))
	if kind == "" {
		return nil
	}
	enc := strings.ToLower(strings.TrimSpace(h.Get("Content-Encoding")))
	if enc != "" && enc != "identity" && enc != "gzip" && enc != "x-gzip" {
		base.Debug("WebVPN 路径模式：未知压缩编码，跳过链接改写:", enc)
		return nil
	}
	orig := resp.Body
	pr, pw := io.Pipe()
	go func() {
		var src io.Reader = orig
		var err error
		if enc == "gzip" || enc == "x-gzip" {
			var gz *gzip.Reader
			if gz, err = gzip.NewReader(orig); err == nil {
				defer gz.Close()
				src = gz
			}
		}
		if err == nil {
			if kind == "html" {
				err = p.RewriteHTML(pw, src)
			} else {
				err = p.rewriteCSSStream(pw, src)
			}
		}
		orig.Close()
		pw.CloseWithError(err)
	}()
	resp.Body = &pipeBody{PipeReader: pr, orig: orig}
	resp.ContentLength = -1
	h.Del("Content-Length")
	h.Del("Content-Encoding")
	h.Del("Content-MD5")
	if et := h.Get("ETag"); et != "" && !strings.HasPrefix(et, "W/") {
		h.Set("ETag", "W/"+et)
	}
	return nil
}

// 关闭时一并关闭后端 body，令改写协程尽快退出
type pipeBody struct {
	*io.PipeReader
	orig io.Closer
}

func (b *pipeBody) Close() error {
	b.orig.Close()
	return b.PipeReader.Close()
}

func rewriteKind(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch ct {
	case "text/html", "application/xhtml+xml":
		return "html"
	case "text/css":
		return "css"
	}
	return ""
}
