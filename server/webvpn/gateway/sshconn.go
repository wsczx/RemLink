package gateway

import (
	"context"
	"crypto/subtle"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"golang.org/x/crypto/ssh"
)

const (
	sshDialTimeout      = 10 * time.Second
	sshHandshakeTimeout = 15 * time.Second
)

var (
	errHostKeyMismatch = errors.New("目标主机指纹与固定值不一致，已拒绝连接（可能存在中间人攻击）")
	errAuthFailed      = errors.New("目标主机认证失败")
)

// 规范化指纹：允许管理员粘贴不带 SHA256: 前缀的值；多个指纹以逗号分隔
func normalizeFps(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !strings.HasPrefix(f, "SHA256:") {
			f = "SHA256:" + f
		}
		out = append(out, strings.TrimRight(f, "="))
	}
	return out
}

// 主机指纹校验：已固定则严格比对；未固定则首次连接写入（TOFU，条件更新保证并发下只记录一次）
func hostKeyCallback(app *dbdata.WebVpnApp, seen *string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		fp := strings.TrimRight(ssh.FingerprintSHA256(key), "=")
		*seen = fp
		pinned := app.HostKeyFp
		if strings.TrimSpace(pinned) == "" {
			got, err := dbdata.WebVpnAppPinHostKey(app.Id, fp)
			if err != nil {
				return err
			}
			if got == fp {
				base.Info("WebVPN SSH 首次连接记录主机指纹(TOFU):", app.Name, app.TargetAddr(), fp)
			}
			pinned = got
		}
		for _, p := range normalizeFps(pinned) {
			if subtle.ConstantTimeCompare([]byte(p), []byte(fp)) == 1 {
				return nil
			}
		}
		base.Warn("WebVPN SSH 主机指纹不匹配:", app.Name, app.TargetAddr(), "got", fp)
		return errHostKeyMismatch
	}
}

// 连接目标 SSH（地址只来自管理员配置的 TargetHost/TargetPort）
// 不请求 agent 转发；未注册任何入站通道处理器，目标发起的 forwarded-tcpip / auth-agent 通道会被 x/crypto/ssh 自动拒绝
func dialSSH(ctx context.Context, app *dbdata.WebVpnApp, a *targetAuth) (*ssh.Client, string, error) {
	var methods []ssh.AuthMethod
	if a.Signer != nil {
		methods = append(methods, ssh.PublicKeys(a.Signer))
	}
	if a.Password != "" {
		pw := a.Password
		methods = append(methods, ssh.Password(pw))
		methods = append(methods, ssh.KeyboardInteractive(func(user, instr string, qs []string, echos []bool) ([]string, error) {
			ans := make([]string, len(qs))
			for i := range qs {
				ans[i] = pw
			}
			return ans, nil
		}))
	}
	var seenFp string
	cfg := &ssh.ClientConfig{
		User:            a.User,
		Auth:            methods,
		HostKeyCallback: hostKeyCallback(app, &seenFp),
		Timeout:         sshHandshakeTimeout,
	}
	addr := app.TargetAddr()
	d := &net.Dialer{Timeout: sshDialTimeout, KeepAlive: 30 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, "", err
	}
	_ = conn.SetDeadline(time.Now().Add(sshHandshakeTimeout))
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		if errors.Is(err, errHostKeyMismatch) {
			return nil, seenFp, errHostKeyMismatch
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, seenFp, errAuthFailed
		}
		return nil, seenFp, err
	}
	_ = conn.SetDeadline(time.Time{})
	client := ssh.NewClient(c, chans, reqs)
	go keepAlive(client)
	return client, seenFp, nil
}

// 定期发送 keepalive，探测目标断连
func keepAlive(c *ssh.Client) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		done := make(chan error, 1)
		go func() {
			_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				c.Close()
				return
			}
		case <-time.After(20 * time.Second):
			c.Close()
			return
		}
	}
}
