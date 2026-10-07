package gateway

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn/remotefs"
	"golang.org/x/crypto/ssh"
)

const fileDialTimeout = 20 * time.Second

// 建会话时与目标建立的连接
type targetConn struct {
	ssh  *ssh.Client
	fp   string // SSH 主机指纹
	rfs  remotefs.RemoteFS
	home string
}

func (t *targetConn) close() {
	if t.ssh != nil {
		_ = t.ssh.Close()
	}
	if t.rfs != nil {
		_ = t.rfs.Close()
	}
}

// 按协议连接目标并完成认证；失败时已写回响应
func (g *Gateway) connectTarget(w http.ResponseWriter, r *http.Request, c *reqCtx, auth *targetAuth, failKey string) (*targetConn, bool) {
	app := c.app
	authFailed := func() {
		g.fails.fail(failKey)
		writeErr(w, http.StatusUnauthorized, "auth_failed", errAuthFailed.Error())
	}
	unreachable := func(err error) {
		base.Warn("WebVPN 连接目标失败:", c.appName, app.ProtocolOf(), app.TargetAddr(), err)
		writeErr(w, http.StatusBadGateway, "target_unreachable", "连接目标主机失败")
	}
	switch app.ProtocolOf() {
	case dbdata.WebVpnProtoSSH, dbdata.WebVpnProtoSFTP:
		ctx, cancel := context.WithTimeout(r.Context(), sshDialTimeout+sshHandshakeTimeout)
		defer cancel()
		client, fp, err := dialSSH(ctx, app, auth)
		if err != nil {
			switch {
			case errors.Is(err, errAuthFailed):
				authFailed()
			case errors.Is(err, errHostKeyMismatch):
				writeErrData(w, http.StatusBadGateway, "host_key_mismatch", errHostKeyMismatch.Error(), map[string]any{"fingerprint": fp})
			default:
				unreachable(err)
			}
			return nil, false
		}
		g.fails.reset(failKey)
		return &targetConn{ssh: client, fp: fp}, true

	case dbdata.WebVpnProtoSMB, dbdata.WebVpnProtoFTP:
		ctx, cancel := context.WithTimeout(r.Context(), fileDialTimeout)
		defer cancel()
		rfs, home, err := dialFileBackend(ctx, app, auth)
		if err != nil {
			switch {
			case errors.Is(err, remotefs.ErrAuthFailed):
				authFailed()
			case errors.Is(err, remotefs.ErrShareNotFound):
				base.Warn("WebVPN SMB 共享挂载失败:", c.appName, app.TargetAddr(), err)
				writeErr(w, http.StatusBadGateway, "share_not_found", remotefs.ErrShareNotFound.Error())
			case errors.Is(err, remotefs.ErrBadPath):
				writeErr(w, http.StatusBadRequest, "bad_credential", "账号或密码包含目标端编码不支持的字符")
			default:
				unreachable(err)
			}
			return nil, false
		}
		g.fails.reset(failKey)
		return &targetConn{rfs: rfs, home: home}, true

	case dbdata.WebVpnProtoRDP, dbdata.WebVpnProtoVNC:
		// 图形协议的目标认证在附着桌面时由 guacd 完成；此处仅确认 guacd 可用
		ctx, cancel := context.WithTimeout(r.Context(), guacdDialTimeout)
		defer cancel()
		if err := checkGuacd(ctx); err != nil {
			base.Warn("WebVPN guacd 不可用:", guacdAddr(), err)
			writeErr(w, http.StatusServiceUnavailable, "guacd_unavailable", errGuacdUnavailable.Error())
			return nil, false
		}
		return &targetConn{}, true
	}
	writeErr(w, http.StatusBadRequest, "unsupported_protocol", "该应用协议暂不支持网页访问")
	return nil, false
}

// 建立 SMB/FTP 文件后端，返回未配置根目录时的默认根
func dialFileBackend(ctx context.Context, app *dbdata.WebVpnApp, auth *targetAuth) (remotefs.RemoteFS, string, error) {
	o := app.OptionsOf()
	switch app.ProtocolOf() {
	case dbdata.WebVpnProtoSMB:
		f, err := remotefs.DialSMB(ctx, remotefs.SMBConfig{
			Addr: app.TargetAddr(), User: auth.User, Password: auth.Password,
			Domain: o.Domain, Share: o.Share, Timeout: fileDialTimeout,
		})
		if err != nil {
			return nil, "", err
		}
		return f, "/", nil
	case dbdata.WebVpnProtoFTP:
		f, err := remotefs.DialFTP(ctx, remotefs.FTPConfig{
			Addr: app.TargetAddr(), User: auth.User, Password: auth.Password,
			TLS: o.TLS, ServerName: app.TargetHost, SkipVerify: o.SkipVerify, Encoding: o.Encoding,
			Timeout: fileDialTimeout,
		})
		if err != nil {
			return nil, "", err
		}
		return f, f.Home(), nil
	}
	return nil, "", errors.New("unsupported protocol")
}
