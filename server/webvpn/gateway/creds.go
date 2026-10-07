package gateway

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"golang.org/x/crypto/ssh"
)

// 目标认证材料
type targetAuth struct {
	User     string
	Password string
	Signer   ssh.Signer
}

// 用户建会话时提交的凭据
type credInput struct {
	App        string `json:"app"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	PrivateKey string `json:"private_key"`
	Passphrase string `json:"passphrase"`
}

type credError struct {
	code string
	msg  string
}

func (e *credError) Error() string { return e.msg }

var (
	errNeedUsername = &credError{"need_username", "请输入目标主机账号"}
	errNeedCred     = &credError{"need_credential", "请输入密码或私钥"}
	errNeedPassword = &credError{"need_password", "未缓存门户登录密码，请输入密码"}
	errBadKey       = &credError{"bad_private_key", "私钥格式错误或口令不正确"}
	errBadUsername  = &credError{"bad_username", "目标账号格式不合法"}
	errStoredBroken = &credError{"stored_credential_error", "托管凭据不可用，请联系管理员重新录入"}
	// 非 SSH 协议不支持私钥认证，必须提供密码
	errNeedPasswordOnly = &credError{"need_credential", "请输入密码"}
)

func validTargetUser(u string) bool {
	if u == "" || len(u) > 128 {
		return false
	}
	for _, c := range u {
		if unicode.IsControl(c) || unicode.IsSpace(c) {
			return false
		}
	}
	return true
}

// 按应用凭据模式解析目标认证材料
// 私钥仅 ssh/sftp 使用；VNC 账号与密码均可为空（多数 VNC 服务仅密码或无认证）
func resolveAuth(app *dbdata.WebVpnApp, portalUser *dbdata.User, in *credInput) (*targetAuth, error) {
	a := &targetAuth{}
	proto := app.ProtocolOf()
	useKey := proto == dbdata.WebVpnProtoSSH || proto == dbdata.WebVpnProtoSFTP
	isVNC := proto == dbdata.WebVpnProtoVNC
	switch app.CredModeOf() {
	case dbdata.WebVpnCredStored:
		a.User = app.CredUser
		secret, err := dbdata.DecryptWebVpnCred(app.CredSecret)
		if err != nil {
			base.Error("WebVPN 托管凭据解密失败:", app.Name, err)
			return nil, errStoredBroken
		}
		key := ""
		if useKey {
			if key, err = dbdata.DecryptWebVpnCred(app.CredKey); err != nil {
				base.Error("WebVPN 托管私钥解密失败:", app.Name, err)
				return nil, errStoredBroken
			}
		}
		if key != "" {
			// 私钥带口令时以托管密码作为口令
			s, err := parseSigner(key, secret)
			if err != nil {
				base.Error("WebVPN 托管私钥解析失败:", app.Name, err)
				return nil, errStoredBroken
			}
			a.Signer = s
		}
		a.Password = secret
	case dbdata.WebVpnCredPassthrough:
		// 使用门户登录账号；密码优先取登录时缓存，否则由用户输入
		a.User = portalUser.Username
		if pw, ok := passwords.get(portalUser.Username); ok {
			a.Password = pw
		} else if in.Password != "" {
			a.Password = in.Password
		} else {
			return nil, errNeedPassword
		}
	default: // prompt
		a.User = app.CredUser
		if a.User == "" {
			a.User = strings.TrimSpace(in.Username)
		}
		if a.User == "" && !isVNC {
			return nil, errNeedUsername
		}
		a.Password = in.Password
		if in.PrivateKey != "" && useKey {
			s, err := parseSigner(in.PrivateKey, in.Passphrase)
			if err != nil {
				return nil, errBadKey
			}
			a.Signer = s
		}
	}
	if isVNC {
		if a.User != "" && !validTargetUser(a.User) {
			return nil, errBadUsername
		}
		return a, nil
	}
	if !validTargetUser(a.User) {
		return nil, errBadUsername
	}
	if a.Password == "" && a.Signer == nil {
		if !useKey {
			return nil, errNeedPasswordOnly
		}
		return nil, errNeedCred
	}
	return a, nil
}

func parseSigner(pemKey, passphrase string) (ssh.Signer, error) {
	s, err := ssh.ParsePrivateKey([]byte(pemKey))
	if err == nil {
		return s, nil
	}
	var pm *ssh.PassphraseMissingError
	if errors.As(err, &pm) && passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase([]byte(pemKey), []byte(passphrase))
	}
	return nil, err
}

// ---------------- passthrough：门户登录密码缓存 ----------------
//
// 门户会话（JWT）中不含密码，passthrough 模式需在门户登录成功时调用 RememberLoginPassword 缓存。
// 缓存仅在进程内存中，以进程启动时随机生成的 AES-256-GCM 密钥加密，TTL 与 WebVPN 会话绝对寿命一致；
// 用户被吊销/登出时清除。进程重启后缓存失效，用户需重新登录或在连接时手动输入密码。

type pwEntry struct {
	ct  []byte
	exp time.Time
}

type pwCache struct {
	mu  sync.Mutex
	gcm cipher.AEAD
	m   map[string]pwEntry
}

var passwords = newPwCache()

func newPwCache() *pwCache {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	blk, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(blk)
	return &pwCache{gcm: gcm, m: make(map[string]pwEntry)}
}

func pwTTL() time.Duration {
	min := base.GetCfg().WebVpnSessionMaxLifetime
	if min <= 0 {
		min = 480
	}
	return time.Duration(min) * time.Minute
}

func (c *pwCache) put(user, pw string, ttl time.Duration) {
	if user == "" || pw == "" {
		return
	}
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return
	}
	ct := c.gcm.Seal(nonce, nonce, []byte(pw), []byte(user))
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, e := range c.m {
		if now.After(e.exp) {
			delete(c.m, k)
		}
	}
	c.m[user] = pwEntry{ct: ct, exp: now.Add(ttl)}
}

func (c *pwCache) get(user string) (string, bool) {
	c.mu.Lock()
	e, ok := c.m[user]
	if ok && time.Now().After(e.exp) {
		delete(c.m, user)
		ok = false
	}
	c.mu.Unlock()
	if !ok {
		return "", false
	}
	ns := c.gcm.NonceSize()
	pt, err := c.gcm.Open(nil, e.ct[:ns], e.ct[ns:], []byte(user))
	if err != nil {
		return "", false
	}
	return string(pt), true
}

func (c *pwCache) del(user string) {
	c.mu.Lock()
	delete(c.m, user)
	c.mu.Unlock()
}

// 门户登录成功后调用：缓存密码供 passthrough 模式使用（仅当存在 passthrough 应用时才有意义）
func RememberLoginPassword(username, password string) {
	passwords.put(username, password, pwTTL())
}

// 门户登出时调用：清除缓存密码
func ForgetLoginPassword(username string) { passwords.del(username) }

func init() {
	// 吊销（踢下线/登出/权限收窄等）时同时清除缓存密码
	dbdata.OnWebVpnUserRevoked(ForgetLoginPassword)
}

// ---------------- 失败限速 ----------------

// 每个 用户+应用 在窗口期内认证失败超过阈值即暂时锁定，防止借网关爆破目标主机
type failLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	m      map[string][]time.Time
}

func newFailLimiter(max int, window time.Duration) *failLimiter {
	return &failLimiter{max: max, window: window, m: make(map[string][]time.Time)}
}

func (f *failLimiter) prune(k string, now time.Time) []time.Time {
	ts := f.m[k]
	i := 0
	for _, t := range ts {
		if now.Sub(t) < f.window {
			ts[i] = t
			i++
		}
	}
	ts = ts[:i]
	if len(ts) == 0 {
		delete(f.m, k)
	} else {
		f.m[k] = ts
	}
	return ts
}

func (f *failLimiter) blocked(k string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.prune(k, time.Now())) >= f.max
}

func (f *failLimiter) fail(k string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	f.m[k] = append(f.prune(k, now), now)
}

func (f *failLimiter) reset(k string) {
	f.mu.Lock()
	delete(f.m, k)
	f.mu.Unlock()
}
