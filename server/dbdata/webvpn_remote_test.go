package dbdata

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/security"
)

func setJwtSecret(t *testing.T, s string) {
	t.Helper()
	old := base.GetCfg().JwtSecret
	base.UpdateCfg(func(c *base.ServerConfig) { c.JwtSecret = s })
	t.Cleanup(func() { base.UpdateCfg(func(c *base.ServerConfig) { c.JwtSecret = old }) })
}

// 启用「数据加密」主密钥（写入临时密钥文件）
func enableMasterKey(t *testing.T) {
	t.Helper()
	t.Setenv("REMLINK_ENCRYPTION_KEY", filepath.Join(t.TempDir(), ".encryption_key"))
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	require.NoError(t, ImportEncryptionKey(hex.EncodeToString(k)))
	require.True(t, security.IsEnabled())
	t.Cleanup(func() { _ = security.DeleteKey() })
}

func TestWebVpnCredKEKRoundtrip(t *testing.T) {
	require.False(t, security.IsEnabled())
	setJwtSecret(t, "jwt-secret-A")
	ct, err := EncryptWebVpnCred("p@ss 密码")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(ct, webVpnCredKEKPrefix), ct)
	assert.NotContains(t, ct, "p@ss")
	ct2, _ := EncryptWebVpnCred("p@ss 密码")
	assert.NotEqual(t, ct, ct2, "GCM 随机 nonce，密文不应相同")
	pt, err := DecryptWebVpnCred(ct)
	require.NoError(t, err)
	assert.Equal(t, "p@ss 密码", pt)
	assert.True(t, IsWebVpnCredCipher(ct))

	// jwt_secret 变更后无法解密（不得静默返回密文）
	setJwtSecret(t, "jwt-secret-B")
	_, err = DecryptWebVpnCred(ct)
	assert.Error(t, err)

	// 空串与历史明文
	v, err := EncryptWebVpnCred("")
	assert.NoError(t, err)
	assert.Equal(t, "", v)
	v, _ = DecryptWebVpnCred("legacy")
	assert.Equal(t, "legacy", v)
}

func TestWebVpnCredMasterKey(t *testing.T) {
	setJwtSecret(t, "jwt-secret-A")
	enableMasterKey(t)
	ct, err := EncryptWebVpnCred("secret")
	require.NoError(t, err)
	assert.True(t, security.IsEncrypted(ct))
	pt, err := DecryptWebVpnCred(ct)
	require.NoError(t, err)
	assert.Equal(t, "secret", pt)
}

func newSSHApp(name string) *WebVpnApp {
	return &WebVpnApp{
		Name: name, Protocol: "ssh", TargetHost: "10.0.0.5", TargetPort: 22,
		CredMode: "stored", CredUser: "root", CredSecret: "pw1", CredKey: "KEYDATA",
		Options: `{"root":"/srv//data/"}`, AllowDownload: true,
	}
}

func TestSetWebVpnAppProtocolAndCreds(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()
	setJwtSecret(t, "jwt-secret-A")

	// 旧数据兼容：未指定协议的 http 应用
	legacy := &WebVpnApp{Name: "web1", Backend: "http://10.0.0.1:8080"}
	ast.NoError(SetWebVpnApp(legacy))
	ast.Equal("http", legacy.Protocol)

	a := newSSHApp("ssh1")
	require.NoError(t, SetWebVpnApp(a))
	var row WebVpnApp
	require.NoError(t, One("Name", "ssh1", &row))
	ast.True(strings.HasPrefix(row.CredSecret, webVpnCredKEKPrefix), "密码须加密入库")
	ast.True(strings.HasPrefix(row.CredKey, webVpnCredKEKPrefix), "私钥须加密入库")
	ast.NotContains(row.CredSecret, "pw1")
	ast.Equal(`{"root":"/srv/data"}`, row.Options)
	ast.Equal("/srv/data", row.OptionsOf().Root)
	pt, _ := DecryptWebVpnCred(row.CredSecret)
	ast.Equal("pw1", pt)

	// 脱敏输出
	red := row.Redacted()
	ast.Empty(red.CredSecret)
	ast.Empty(red.CredKey)
	ast.True(red.HasSecret)
	ast.True(red.HasKey)

	// 空值 = 不修改
	upd := red
	upd.Note = "changed"
	require.NoError(t, SetWebVpnApp(&upd))
	var row2 WebVpnApp
	require.NoError(t, One("Name", "ssh1", &row2))
	ast.Equal(row.CredSecret, row2.CredSecret)
	ast.Equal(row.CredKey, row2.CredKey)
	ast.Equal("changed", row2.Note)

	// 回传密文本身不会被二次加密
	upd2 := row2
	require.NoError(t, SetWebVpnApp(&upd2))
	var row3 WebVpnApp
	require.NoError(t, One("Name", "ssh1", &row3))
	ast.Equal(row2.CredSecret, row3.CredSecret)

	// 显式清除私钥
	upd3 := row3.Redacted()
	upd3.ClearKey = true
	require.NoError(t, SetWebVpnApp(&upd3))
	var row4 WebVpnApp
	require.NoError(t, One("Name", "ssh1", &row4))
	ast.Empty(row4.CredKey)
	ast.NotEmpty(row4.CredSecret)
}

func TestValidateWebVpnTarget(t *testing.T) {
	bad := []*WebVpnApp{
		{Name: "x", Protocol: "telnet", TargetHost: "h"},
		{Name: "x", Protocol: "ssh", TargetHost: ""},
		{Name: "x", Protocol: "ssh", TargetHost: "ssh://h"},
		{Name: "x", Protocol: "ssh", TargetHost: "h:22"},
		{Name: "x", Protocol: "ssh", TargetHost: "h/path"},
		{Name: "x", Protocol: "ssh", TargetHost: "user@h"},
		{Name: "x", Protocol: "ssh", TargetHost: "h", TargetPort: 70000},
		{Name: "x", Protocol: "ssh", TargetHost: "h", CredMode: "magic"},
		{Name: "x", Protocol: "ssh", TargetHost: "h", CredMode: "stored"},
		{Name: "x", Protocol: "ssh", TargetHost: "h", Options: "[1]"},
		{Name: "x", Protocol: "ssh", TargetHost: "h", Options: `{"root":"../etc"}`},
		{Name: "x", Protocol: "ssh", TargetHost: "h", Options: `{"root":"/a/../etc"}`},
		{Name: "x", Protocol: "ssh", TargetHost: "h", IdleTimeout: -1},
		{Name: "x", Protocol: "http", Backend: ""},
		{Name: "x", Protocol: "http", Backend: "ftp://h"},
	}
	for i, a := range bad {
		assert.Error(t, validateWebVpnTarget(a), "case %d %+v", i, a)
	}
	good := []*WebVpnApp{
		{Name: "x", Protocol: "SSH", TargetHost: "host-1.internal"},
		{Name: "x", Protocol: "sftp", TargetHost: "10.1.2.3", TargetPort: 2222, CredMode: "passthrough"},
		{Name: "x", Protocol: "ssh", TargetHost: "fd00::1"},
		{Name: "x", Protocol: "rdp", TargetHost: "win.local"},
		{Name: "x", Backend: "https://10.0.0.1"},
	}
	for i, a := range good {
		assert.NoError(t, validateWebVpnTarget(a), "case %d", i)
	}
	a := &WebVpnApp{Protocol: "ssh", TargetHost: "fd00::1"}
	assert.Equal(t, "[fd00::1]:22", a.TargetAddr())
	assert.Equal(t, "prompt", (&WebVpnApp{}).CredModeOf())
}

func TestWebVpnHostKeyTOFU(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()
	setJwtSecret(t, "jwt-secret-A")

	a := newSSHApp("tofu")
	require.NoError(t, SetWebVpnApp(a))
	got, err := WebVpnAppPinHostKey(a.Id, "SHA256:first")
	ast.NoError(err)
	ast.Equal("SHA256:first", got)
	// 已固定后再次 TOFU 不得覆盖
	got, err = WebVpnAppPinHostKey(a.Id, "SHA256:attacker")
	ast.NoError(err)
	ast.Equal("SHA256:first", got)

	// 目标变更且未手工修改指纹 → 重置
	var cur WebVpnApp
	require.NoError(t, One("Id", a.Id, &cur))
	upd := cur.Redacted()
	upd.TargetHost = "10.0.0.6"
	require.NoError(t, SetWebVpnApp(&upd))
	var after WebVpnApp // xorm Get 会把非零字段当条件，须用新变量
	require.NoError(t, One("Id", a.Id, &after))
	ast.Empty(after.HostKeyFp)
}

func TestWebVpnHooksFire(t *testing.T) {
	preIpData(t)
	defer closeIpdata()
	setJwtSecret(t, "jwt-secret-A")

	revoked := make(chan string, 4)
	changed := make(chan string, 4)
	OnWebVpnUserRevoked(func(u string) { revoked <- u })
	OnWebVpnAppChanged(func(n string) { changed <- n })

	require.NoError(t, WebVpnRevokeUser("hook-user"))
	select {
	case u := <-revoked:
		assert.Equal(t, "hook-user", u)
	case <-time.After(2 * time.Second):
		t.Fatal("吊销钩子未触发")
	}
	a := newSSHApp("hookapp")
	require.NoError(t, SetWebVpnApp(a))
	select {
	case n := <-changed:
		assert.Equal(t, "hookapp", n)
	case <-time.After(2 * time.Second):
		t.Fatal("应用变更钩子未触发")
	}
	require.NoError(t, DelWebVpnApp(a.Id))
	select {
	case n := <-changed:
		assert.Equal(t, "hookapp", n)
	case <-time.After(2 * time.Second):
		t.Fatal("应用删除钩子未触发")
	}
}

func TestMigrateWebVpnCreds(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()
	setJwtSecret(t, "jwt-secret-A")

	a := newSSHApp("mig")
	require.NoError(t, SetWebVpnApp(a))
	enableMasterKey(t)
	n, _, err := migrateWebVpnCreds(true)
	require.NoError(t, err)
	ast.Equal(1, n)
	var row WebVpnApp
	require.NoError(t, One("Name", "mig", &row))
	ast.True(security.IsEncrypted(row.CredSecret))
	pt, _ := DecryptWebVpnCred(row.CredSecret)
	ast.Equal("pw1", pt)

	n, _, err = migrateWebVpnCreds(false)
	require.NoError(t, err)
	ast.Equal(1, n)
	var row2 WebVpnApp
	require.NoError(t, One("Name", "mig", &row2))
	ast.True(strings.HasPrefix(row2.CredSecret, webVpnCredKEKPrefix), "关闭加密后仍不得为明文")
	pt, _ = DecryptWebVpnCred(row2.CredKey)
	ast.Equal("KEYDATA", pt)
}

func TestWebVpnRecordFullPath(t *testing.T) {
	for _, bad := range []string{"", "/etc/passwd", "../x.cast", "a/../../x", "a\x00b"} {
		_, err := WebVpnRecordFullPath(bad)
		assert.Error(t, err, bad)
	}
	p, err := WebVpnRecordFullPath("20260101/abc.cast")
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(webVpnRecordDir, "20260101", "abc.cast"), p)
}

func TestWebVpnSessionAndFileLog(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()
	require.NoError(t, AddWebVpnSession(&WebVpnSession{Sid: "s1", Username: "u", AppName: "a", Protocol: "ssh", Target: "h:22", ClientIP: "1.1.1.1", StartedAt: time.Now()}))
	require.NoError(t, EndWebVpnSession("s1", 10, 20, "user_close", "20260101/s1.cast"))
	s, err := GetWebVpnSession("s1")
	require.NoError(t, err)
	ast.NotNil(s.EndedAt)
	ast.Equal(int64(20), s.BytesOut)
	ast.Equal("1.1.1.1", s.ClientIP)
	list, count, err := WebVpnSessionList(10, 1, WebVpnSessionSearch{Username: "u"})
	ast.NoError(err)
	ast.Equal(1, count)
	ast.Len(list, 1)

	require.NoError(t, AddWebVpnFileLog(&WebVpnFileLog{Sid: "s1", Username: "u", AppName: "a", Op: "upload", Path: "/x", Size: 3, Result: "ok"}))
	logs, n, err := WebVpnFileLogList(10, 1, WebVpnFileLogSearch{Sid: "s1", Op: "upload"})
	ast.NoError(err)
	ast.Equal(1, n)
	ast.Equal("/x", logs[0].Path)

	require.NoError(t, AddWebVpnSession(&WebVpnSession{Sid: "s2", Username: "u", AppName: "a", Protocol: "ssh", Target: "h:22", StartedAt: time.Now()}))
	CloseDanglingWebVpnSessions()
	s2, _ := GetWebVpnSession("s2")
	ast.Equal("shutdown", s2.CloseReason)
}

func TestValidateWebVpnProtoOptions(t *testing.T) {
	bad := []*WebVpnApp{
		{Name: "x", Protocol: "smb", TargetHost: "h"},                             // 缺 share
		{Name: "x", Protocol: "smb", TargetHost: "h", Options: `{"root":"/a"}`},   // 缺 share
		{Name: "x", Protocol: "smb", TargetHost: "h", Options: `{"share":"a/b"}`}, // 非法字符
		{Name: "x", Protocol: "smb", TargetHost: "h", Options: `{"share":"a","domain":"a\\b"}`},
		{Name: "x", Protocol: "smb", TargetHost: "h", Options: `{"share":1}`}, // 类型错误
		{Name: "x", Protocol: "ftp", TargetHost: "h", Options: `{"tls":"ssl"}`},
		{Name: "x", Protocol: "ftp", TargetHost: "h", Options: `{"encoding":"big5"}`},
		{Name: "x", Protocol: "ftp", TargetHost: "h", Options: `{"passive":false}`},
		{Name: "x", Protocol: "ftp", TargetHost: "h", Options: `{"skip_verify":"yes"}`},
		{Name: "x", Protocol: "rdp", TargetHost: "h", Options: `{"security":"none"}`},
		{Name: "x", Protocol: "rdp", TargetHost: "h", Options: `{"ignore_cert":"true"}`},
		{Name: "x", Protocol: "rdp", TargetHost: "h", CredMode: "stored"}, // 托管需账号
	}
	for i, a := range bad {
		assert.Error(t, validateWebVpnTarget(a), "case %d %+v", i, a)
	}

	smb := &WebVpnApp{Name: "x", Protocol: "smb", TargetHost: "fs", Options: `{"share":" \\data$ ","domain":" CORP ","extra":1}`}
	require.NoError(t, validateWebVpnTarget(smb))
	o := smb.OptionsOf()
	assert.Equal(t, "data$", o.Share)
	assert.Equal(t, "CORP", o.Domain)
	assert.Contains(t, smb.Options, `"extra":1`) // 未知字段原样保留
	assert.Equal(t, 445, smb.TargetPortOf())
	assert.True(t, smb.HasFileAccess())
	assert.False(t, smb.IsDesktop())

	ftp := &WebVpnApp{Name: "x", Protocol: "ftp", TargetHost: "f", Options: `{"tls":"IMPLICIT","encoding":"GB2312","root":"/pub/"}`}
	require.NoError(t, validateWebVpnTarget(ftp))
	fo := ftp.OptionsOf()
	assert.Equal(t, "implicit", fo.TLS)
	assert.Equal(t, "gbk", fo.Encoding)
	assert.Equal(t, "/pub", fo.Root)
	require.NotNil(t, fo.Passive)
	assert.True(t, *fo.Passive)
	assert.Equal(t, 990, ftp.TargetPortOf())
	ftp2 := &WebVpnApp{Name: "x", Protocol: "ftp", TargetHost: "f", Options: `{"tls":"explicit"}`}
	require.NoError(t, validateWebVpnTarget(ftp2))
	assert.Equal(t, 21, ftp2.TargetPortOf())
	assert.Equal(t, 21, (&WebVpnApp{Protocol: "ftp"}).TargetPortOf())
	ftp2.TargetPort = 2121
	assert.Equal(t, 2121, ftp2.TargetPortOf())

	rdp := &WebVpnApp{Name: "x", Protocol: "rdp", TargetHost: "w", Options: `{"ignore_cert":true}`}
	require.NoError(t, validateWebVpnTarget(rdp))
	assert.Equal(t, "any", rdp.OptionsOf().Security)
	assert.True(t, rdp.OptionsOf().IgnoreCert)
	assert.Equal(t, 3389, rdp.TargetPortOf())
	assert.True(t, rdp.IsDesktop())
	assert.False(t, rdp.HasFileAccess())

	// VNC 托管凭据可不配置账号
	vnc := &WebVpnApp{Name: "x", Protocol: "vnc", TargetHost: "v", CredMode: "stored"}
	require.NoError(t, validateWebVpnTarget(vnc))
	assert.Equal(t, 5900, vnc.TargetPortOf())
}
