package handler

import (
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn"
	"net/http"
	"strings"
	"time"
)

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

// 包装 ModifyResponse，在响应返回后投递审计记录
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
			ClientIP:   webvpn.ClientIP(rw.req),
			DurationMs: time.Since(rec.start).Milliseconds(),
			RiskLevel:  webvpn.RiskOf(resp.StatusCode),
		})
		return nil
	}
}

// 给反代响应补 CORS 头：仅当入站带 Origin（浏览器跨域实际请求）时补，与预检一致
func withCORS(next func(*http.Response) error, r *http.Request, app *dbdata.WebVpnApp) func(*http.Response) error {
	origin := r.Header.Get("Origin")
	if origin == "" || (!webVpnSameOrigin(r) && !dbdata.WebVpn.CorsOriginAllowed(app, origin)) {
		return next
	}
	return func(resp *http.Response) error {
		if next != nil {
			if err := next(resp); err != nil {
				return err
			}
		}
		setCORSHeaders(resp.Header, origin)
		return nil
	}
}

// 透出底层 writer 并记录状态码与写出字节数，供访问审计使用
type webVpnRespWriter struct {
	http.ResponseWriter
	req          *http.Request
	statusCode   int
	bytesWritten int64
}
