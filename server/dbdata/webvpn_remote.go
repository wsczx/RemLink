package dbdata

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/security"
	"golang.org/x/crypto/hkdf"
	"xorm.io/xorm"
)

// WebVPN 接入协议
const (
	WebVpnProtoHTTP = "http"
	WebVpnProtoSSH  = "ssh"
	WebVpnProtoSFTP = "sftp"
	WebVpnProtoSMB  = "smb"
	WebVpnProtoFTP  = "ftp"
	WebVpnProtoRDP  = "rdp"
	WebVpnProtoVNC  = "vnc"
)

// 凭据模式
const (
	WebVpnCredPrompt      = "prompt"      // 用户每次输入
	WebVpnCredStored      = "stored"      // 管理员托管（加密入库）
	WebVpnCredPassthrough = "passthrough" // 使用门户登录账号密码
)

// FTP TLS 模式与文件名编码
const (
	WebVpnFTPTLSNone     = "none"
	WebVpnFTPTLSExplicit = "explicit"
	WebVpnFTPTLSImplicit = "implicit"
	WebVpnEncUTF8        = "utf-8"
	WebVpnEncGBK         = "gbk"
)

var webVpnDefaultPorts = map[string]int{
	WebVpnProtoSSH:  22,
	WebVpnProtoSFTP: 22,
	WebVpnProtoSMB:  445,
	WebVpnProtoFTP:  21,
	WebVpnProtoRDP:  3389,
	WebVpnProtoVNC:  5900,
}

var (
	errWebVpnInvalidProtocol = errors.New("不支持的协议类型")
	errWebVpnInvalidTarget   = errors.New("目标主机必须是合法的域名或 IP（不含协议、端口与路径）")
	errWebVpnInvalidPort     = errors.New("目标端口必须在 1-65535 之间")
	errWebVpnInvalidCredMode = errors.New("凭据模式必须是 prompt / stored / passthrough")
	errWebVpnInvalidOptions  = errors.New("扩展参数必须是合法的 JSON 对象")
	errWebVpnInvalidRoot     = errors.New("根目录必须是以 / 开头的绝对路径且不能包含 ..")
	errWebVpnInvalidLimit    = errors.New("空闲超时/会话上限/文件大小不能为负数")
	errWebVpnStoredNeedUser  = errors.New("托管凭据模式必须配置目标账号")
	errWebVpnCredDecrypt     = errors.New("凭据解密失败（加密密钥或 jwt_secret 可能已变更，请重新录入凭据）")
	errWebVpnSMBNeedShare    = errors.New("SMB 应用必须配置共享名 share（不能包含 \\ / : * ? \" < > | 等字符）")
	errWebVpnSMBBadDomain    = errors.New("SMB 域名 domain 含非法字符")
	errWebVpnFTPBadTLS       = errors.New("FTP tls 必须是 none / explicit / implicit")
	errWebVpnFTPBadEncoding  = errors.New("FTP encoding 必须是 utf-8 / gbk")
	errWebVpnFTPActive       = errors.New("FTP 仅支持被动模式（passive 必须为 true）")
	errWebVpnRDPBadSecurity  = errors.New("RDP security 必须是 any / nla / nla-ext / tls / vmconnect / rdp")
	errWebVpnDBNotReady      = errors.New("数据库未初始化")
)

// 返回规范化协议，空视为 http
func (a *WebVpnApp) ProtocolOf() string {
	p := strings.ToLower(strings.TrimSpace(a.Protocol))
	if p == "" {
		return WebVpnProtoHTTP
	}
	return p
}

// 是否为 HTTP 反向代理类应用
func (a *WebVpnApp) IsHTTP() bool { return a.ProtocolOf() == WebVpnProtoHTTP }

// 返回目标端口（0 时取协议默认端口；FTP 隐式 TLS 默认 990）
func (a *WebVpnApp) TargetPortOf() int {
	if a.TargetPort > 0 {
		return a.TargetPort
	}
	p := a.ProtocolOf()
	if p == WebVpnProtoFTP && a.OptionsOf().TLS == WebVpnFTPTLSImplicit {
		return 990
	}
	return webVpnDefaultPorts[p]
}

// 是否为图形桌面类协议（经 guacd 接入）
func (a *WebVpnApp) IsDesktop() bool {
	p := a.ProtocolOf()
	return p == WebVpnProtoRDP || p == WebVpnProtoVNC
}

// 是否提供文件管理
func (a *WebVpnApp) HasFileAccess() bool {
	switch a.ProtocolOf() {
	case WebVpnProtoSSH, WebVpnProtoSFTP, WebVpnProtoSMB, WebVpnProtoFTP:
		return true
	}
	return false
}

// 返回 host:port 形式的目标地址（非 http 协议）
func (a *WebVpnApp) TargetAddr() string {
	return net.JoinHostPort(a.TargetHost, strconv.Itoa(a.TargetPortOf()))
}

// 返回规范化凭据模式，空视为 prompt
func (a *WebVpnApp) CredModeOf() string {
	m := strings.ToLower(strings.TrimSpace(a.CredMode))
	if m == "" {
		return WebVpnCredPrompt
	}
	return m
}

// 协议扩展参数（未列出的字段原样保存在 Options 中）
type WebVpnAppOptions struct {
	Root string `json:"root,omitempty"` // ssh/sftp/smb/ftp：文件浏览根目录

	// smb
	Share  string `json:"share,omitempty"`  // 共享名（必填），如 "data"、"C$"
	Domain string `json:"domain,omitempty"` // NTLM 域；空则取用户名中的 DOMAIN\user 前缀

	// ftp
	TLS        string `json:"tls,omitempty"`         // none|explicit|implicit；空=none
	Passive    *bool  `json:"passive,omitempty"`     // 仅支持被动模式（true/省略）
	Encoding   string `json:"encoding,omitempty"`    // utf-8|gbk；空=utf-8
	SkipVerify bool   `json:"skip_verify,omitempty"` // FTPS 跳过证书校验

	// rdp/vnc
	Security   string `json:"security,omitempty"`    // rdp：any|nla|nla-ext|tls|vmconnect|rdp；空=any
	IgnoreCert bool   `json:"ignore_cert,omitempty"` // rdp：忽略目标证书
}

// 解析 Options JSON（解析失败返回空配置）
func (a *WebVpnApp) OptionsOf() WebVpnAppOptions {
	var o WebVpnAppOptions
	if strings.TrimSpace(a.Options) == "" {
		return o
	}
	_ = json.Unmarshal([]byte(a.Options), &o)
	return o
}

// 返回脱敏副本：清空凭据密文，仅暴露是否已配置
func (a WebVpnApp) Redacted() WebVpnApp {
	a.HasSecret = a.CredSecret != ""
	a.HasKey = a.CredKey != ""
	a.CredSecret = ""
	a.CredKey = ""
	a.ClearSecret = false
	a.ClearKey = false
	a.Protocol = a.ProtocolOf()
	if a.Users == nil {
		a.Users = []string{}
	}
	if a.Groups == nil {
		a.Groups = []string{}
	}
	if a.AllowPath == nil {
		a.AllowPath = []string{}
	}
	if a.IpAllowList == nil {
		a.IpAllowList = []string{}
	}
	if a.CorsAllowedOrigins == nil {
		a.CorsAllowedOrigins = []string{}
	}
	return a
}

// 主机名校验：仅允许 IP 或 RFC1123 主机名
func webVpnHostValid(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}
	if ip := net.ParseIP(strings.Trim(h, "[]")); ip != nil {
		return !strings.ContainsAny(h, "[]") // IPv6 不带方括号
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for i, c := range label {
			ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
			if !ok || ((i == 0 || i == len(label)-1) && c == '-') {
				return false
			}
		}
	}
	return true
}

// 按协议分支校验目标配置
func validateWebVpnTarget(a *WebVpnApp) error {
	a.Protocol = a.ProtocolOf()
	if a.IdleTimeout < 0 || a.MaxSessionsPerUser < 0 || a.MaxFileSize < 0 {
		return errWebVpnInvalidLimit
	}
	if a.Protocol == WebVpnProtoHTTP {
		if a.Backend == "" {
			return errWebVpnEmptyBackend
		}
		_, err := ParseWebVpnBackendURL(a.Backend)
		return err
	}
	if _, ok := webVpnDefaultPorts[a.Protocol]; !ok {
		return errWebVpnInvalidProtocol
	}
	a.TargetHost = strings.TrimSpace(a.TargetHost)
	if !webVpnHostValid(a.TargetHost) {
		return errWebVpnInvalidTarget
	}
	if a.TargetPort < 0 || a.TargetPort > 65535 {
		return errWebVpnInvalidPort
	}
	a.CredMode = a.CredModeOf()
	switch a.CredMode {
	case WebVpnCredPrompt, WebVpnCredStored, WebVpnCredPassthrough:
	default:
		return errWebVpnInvalidCredMode
	}
	a.CredUser = strings.TrimSpace(a.CredUser)
	// VNC 通常仅密码认证，托管模式允许不配置账号
	if a.CredMode == WebVpnCredStored && a.CredUser == "" && a.Protocol != WebVpnProtoVNC {
		return errWebVpnStoredNeedUser
	}
	a.HostKeyFp = strings.TrimSpace(a.HostKeyFp)
	if s := strings.TrimSpace(a.Options); s != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil || m == nil {
			return errWebVpnInvalidOptions
		}
		if r, ok := m["root"]; ok {
			root, _ := r.(string)
			if root != "" {
				if !strings.HasPrefix(root, "/") || webVpnHasDotDot(root) {
					return errWebVpnInvalidRoot
				}
				m["root"] = path.Clean(root)
			}
		}
		if err := validateWebVpnProtoOptions(a.Protocol, m); err != nil {
			return err
		}
		b, _ := json.Marshal(m)
		a.Options = string(b)
	} else {
		if a.Protocol == WebVpnProtoSMB {
			return errWebVpnSMBNeedShare
		}
		a.Options = ""
	}
	return nil
}

// 按协议校验并规范化扩展参数（就地修改 m）
func validateWebVpnProtoOptions(proto string, m map[string]any) error {
	b, _ := json.Marshal(m)
	var o WebVpnAppOptions
	if err := json.Unmarshal(b, &o); err != nil {
		// 已知字段类型错误（如 ignore_cert 传字符串）
		return errWebVpnInvalidOptions
	}
	switch proto {
	case WebVpnProtoSMB:
		share := strings.Trim(strings.TrimSpace(o.Share), `\/`)
		if !webVpnSMBNameValid(share, 80) {
			return errWebVpnSMBNeedShare
		}
		m["share"] = share
		dom := strings.TrimSpace(o.Domain)
		if dom != "" && !webVpnSMBNameValid(dom, 255) {
			return errWebVpnSMBBadDomain
		}
		if dom == "" {
			delete(m, "domain")
		} else {
			m["domain"] = dom
		}
	case WebVpnProtoFTP:
		t := strings.ToLower(strings.TrimSpace(o.TLS))
		switch t {
		case "":
			t = WebVpnFTPTLSNone
		case WebVpnFTPTLSNone, WebVpnFTPTLSExplicit, WebVpnFTPTLSImplicit:
		default:
			return errWebVpnFTPBadTLS
		}
		m["tls"] = t
		enc := strings.ToLower(strings.TrimSpace(o.Encoding))
		switch enc {
		case "", "utf8", WebVpnEncUTF8:
			enc = WebVpnEncUTF8
		case WebVpnEncGBK, "gb2312", "gb18030":
			enc = WebVpnEncGBK
		default:
			return errWebVpnFTPBadEncoding
		}
		m["encoding"] = enc
		if o.Passive != nil && !*o.Passive {
			return errWebVpnFTPActive
		}
		m["passive"] = true
	case WebVpnProtoRDP:
		sec := strings.ToLower(strings.TrimSpace(o.Security))
		switch sec {
		case "":
			sec = "any"
		case "any", "nla", "nla-ext", "tls", "vmconnect", "rdp":
		default:
			return errWebVpnRDPBadSecurity
		}
		m["security"] = sec
	}
	return nil
}

// SMB 共享名/域名：禁止路径分隔符、通配符与控制字符
func webVpnSMBNameValid(s string, max int) bool {
	if s == "" || len(s) > max {
		return false
	}
	for _, c := range s {
		if c < 0x20 || c == 0x7f || strings.ContainsRune(`\/:*?"<>|`, c) {
			return false
		}
	}
	return true
}

func webVpnHasDotDot(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// 处理凭据字段：明文→密文；空值保持；clear_* 清除；目标变更时重置未改动的主机指纹
func applyWebVpnCreds(a, old *WebVpnApp) error {
	enc := func(in, prev string, clear bool) (string, error) {
		if clear {
			return "", nil
		}
		if in == "" {
			return prev, nil
		}
		if in == prev && IsWebVpnCredCipher(in) {
			return prev, nil
		}
		return EncryptWebVpnCred(in)
	}
	var prevSecret, prevKey string
	if old != nil {
		prevSecret, prevKey = old.CredSecret, old.CredKey
	}
	var err error
	if a.CredSecret, err = enc(a.CredSecret, prevSecret, a.ClearSecret); err != nil {
		return err
	}
	if a.CredKey, err = enc(a.CredKey, prevKey, a.ClearKey); err != nil {
		return err
	}
	if old != nil && a.HostKeyFp == old.HostKeyFp &&
		(old.TargetHost != a.TargetHost || old.TargetPortOf() != a.TargetPortOf() || old.ProtocolOf() != a.ProtocolOf()) {
		// 目标已变更，旧指纹不再适用，下次连接重新 TOFU
		a.HostKeyFp = ""
	}
	a.ClearSecret, a.ClearKey = false, false
	return nil
}

// ---------------- 凭据加解密 ----------------
//
// 优先复用「数据加密」主密钥（security，前缀 $AES256$）；
// 未启用数据加密时，用 HKDF-SHA256(jwt_secret) 派生的 KEK 加密（前缀 $WVK1$），
// 保证凭据在任何情况下都不以明文入库。启用/关闭数据加密时由 secret_batch 迁移两种密文。

const webVpnCredKEKPrefix = "$WVK1$"

// 判断是否为本模块产生的凭据密文
func IsWebVpnCredCipher(s string) bool {
	return security.IsEncrypted(s) || strings.HasPrefix(s, webVpnCredKEKPrefix)
}

func webVpnFallbackCipher() (*security.Cipher, error) {
	secret := base.GetCfg().JwtSecret
	if secret == "" {
		return nil, errors.New("jwt_secret 为空，无法派生凭据加密密钥")
	}
	r := hkdf.New(sha256.New, []byte(secret), []byte("remlink-webvpn-cred"), []byte("webvpn-cred-kek-v1"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	return security.NewCipher(key)
}

// 加密凭据明文（空串返回空串）
func EncryptWebVpnCred(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if security.IsEnabled() {
		return security.Encrypt(plain)
	}
	return encryptWebVpnCredKEK(plain)
}

func encryptWebVpnCredKEK(plain string) (string, error) {
	c, err := webVpnFallbackCipher()
	if err != nil {
		return "", err
	}
	ct, err := c.Encrypt(plain)
	if err != nil {
		return "", err
	}
	return webVpnCredKEKPrefix + strings.TrimPrefix(ct, security.Prefix()), nil
}

// 解密凭据密文；非密文（历史明文）原样返回
func DecryptWebVpnCred(ct string) (string, error) {
	switch {
	case ct == "":
		return "", nil
	case security.IsEncrypted(ct):
		p, err := security.Decrypt(ct)
		if err != nil {
			return "", errWebVpnCredDecrypt
		}
		return p, nil
	case strings.HasPrefix(ct, webVpnCredKEKPrefix):
		c, err := webVpnFallbackCipher()
		if err != nil {
			return "", err
		}
		p, err := c.Decrypt(security.Prefix() + strings.TrimPrefix(ct, webVpnCredKEKPrefix))
		if err != nil {
			return "", errWebVpnCredDecrypt
		}
		return p, nil
	default:
		return ct, nil
	}
}

// 启用/关闭数据加密时迁移 WebVPN 凭据密文：toMaster=true 转为主密钥加密，否则转为 KEK 加密（永不落明文）
func migrateWebVpnCreds(toMaster bool) (int, []string, error) {
	var apps []WebVpnApp
	if err := xdb.Cols("id", "name", "cred_secret", "cred_key").Find(&apps); err != nil {
		return 0, nil, err
	}
	conv := func(v string) (string, bool, error) {
		if v == "" {
			return v, false, nil
		}
		if toMaster && security.IsEncrypted(v) {
			return v, false, nil
		}
		if !toMaster && strings.HasPrefix(v, webVpnCredKEKPrefix) {
			return v, false, nil
		}
		plain, err := DecryptWebVpnCred(v)
		if err != nil {
			return v, false, err
		}
		var out string
		if toMaster {
			out, err = security.Encrypt(plain)
		} else {
			out, err = encryptWebVpnCredKEK(plain)
		}
		return out, err == nil, err
	}
	n := 0
	var warnings []string
	for _, a := range apps {
		s, c1, err1 := conv(a.CredSecret)
		k, c2, err2 := conv(a.CredKey)
		if err1 != nil || err2 != nil {
			warnings = append(warnings, "WebVPN 应用 "+a.Name+" 凭据迁移失败")
			continue
		}
		if !c1 && !c2 {
			continue
		}
		if _, err := xdb.Table("webvpn_app").Where("id = ?", a.Id).Update(map[string]any{"cred_secret": s, "cred_key": k}); err != nil {
			return n, warnings, err
		}
		n++
	}
	if n > 0 {
		InvalidateWebVpnAppCache()
	}
	return n, warnings, nil
}

// TOFU：仅当指纹为空时写入首次连接的主机指纹；返回最终生效的指纹
func WebVpnAppPinHostKey(appId int, fp string) (string, error) {
	_, err := xdb.Table("webvpn_app").Where("id = ? AND (host_key_fp = '' OR host_key_fp IS NULL)", appId).
		Update(map[string]any{"host_key_fp": fp})
	if err != nil {
		return "", err
	}
	InvalidateWebVpnAppCache()
	var a WebVpnApp
	if err := One("Id", appId, &a); err != nil {
		return "", err
	}
	return a.HostKeyFp, nil
}

// ---------------- 变更钩子 ----------------
//
// dbdata 不依赖上层包，长连接注册表（webvpn/live）通过钩子感知吊销与应用变更

var webVpnHooks struct {
	mu         sync.RWMutex
	revoked    []func(username string)
	appChanged []func(appName string)
}

// 注册「用户 WebVPN 会话被吊销」回调（踢下线/权限收窄/删用户/改组等最终都会走到 WebVpnRevokeUser）
func OnWebVpnUserRevoked(fn func(username string)) {
	webVpnHooks.mu.Lock()
	webVpnHooks.revoked = append(webVpnHooks.revoked, fn)
	webVpnHooks.mu.Unlock()
}

// 注册「WebVPN 应用配置变更/删除」回调
func OnWebVpnAppChanged(fn func(appName string)) {
	webVpnHooks.mu.Lock()
	webVpnHooks.appChanged = append(webVpnHooks.appChanged, fn)
	webVpnHooks.mu.Unlock()
}

func fireWebVpnUserRevoked(username string) {
	webVpnHooks.mu.RLock()
	fns := append([]func(string){}, webVpnHooks.revoked...)
	webVpnHooks.mu.RUnlock()
	for _, fn := range fns {
		go fn(username)
	}
}

func fireWebVpnAppChanged(name string) {
	webVpnHooks.mu.RLock()
	fns := append([]func(string){}, webVpnHooks.appChanged...)
	webVpnHooks.mu.RUnlock()
	for _, fn := range fns {
		go fn(name)
	}
}

// ---------------- 会话与文件日志 ----------------

// 非 HTTP 协议会话记录（建立时写入，结束时更新）
type WebVpnSession struct {
	Id            int64      `json:"id" xorm:"pk autoincr not null"`
	Sid           string     `json:"sid" xorm:"varchar(64) not null unique"`
	Username      string     `json:"username" xorm:"varchar(60) not null index"`
	GroupName     string     `json:"group_name" xorm:"varchar(255) not null default ''"`
	AppName       string     `json:"app_name" xorm:"varchar(60) not null index"`
	Protocol      string     `json:"protocol" xorm:"varchar(16) not null"`
	Target        string     `json:"target" xorm:"varchar(255) not null"`
	ClientIP      string     `json:"client_ip" xorm:"'client_ip' varchar(60) not null default ''"`
	StartedAt     time.Time  `json:"started_at" xorm:"DateTime index"`
	EndedAt       *time.Time `json:"ended_at" xorm:"DateTime"`
	BytesIn       int64      `json:"bytes_in" xorm:"BigInt not null default 0"`  // 客户端 → 目标
	BytesOut      int64      `json:"bytes_out" xorm:"BigInt not null default 0"` // 目标 → 客户端
	CloseReason   string     `json:"close_reason" xorm:"varchar(64) not null default ''"`
	RecordingPath string     `json:"recording_path" xorm:"varchar(255) not null default ''"` // 相对录像目录
	// 会话开始时的录像状态：on=已开启 off=应用未开启 na=协议不支持（文件类）；空=旧数据（字段加入前）
	RecordMode string `json:"record_mode" xorm:"varchar(16) not null default ''"`
	RecordErr  string `json:"record_err" xorm:"varchar(255) not null default ''"` // 录像创建失败原因
}

// 文件操作日志
type WebVpnFileLog struct {
	Id        int64     `json:"id" xorm:"pk autoincr not null"`
	Sid       string    `json:"sid" xorm:"varchar(64) not null index"`
	Username  string    `json:"username" xorm:"varchar(60) not null index"`
	AppName   string    `json:"app_name" xorm:"varchar(60) not null index"`
	Op        string    `json:"op" xorm:"varchar(32) not null"` // list/download/download_dir/upload/mkdir/rename/delete
	Path      string    `json:"path" xorm:"varchar(1024) not null"`
	Size      int64     `json:"size" xorm:"BigInt not null default 0"`
	Result    string    `json:"result" xorm:"varchar(255) not null default ''"` // ok 或错误原因
	CreatedAt time.Time `json:"created_at" xorm:"DateTime created index"`
}

func (WebVpnSession) TableName() string { return "webvpn_session" }
func (WebVpnFileLog) TableName() string { return "webvpn_file_log" }

// 录像根目录（与备份目录同在 conf 下）
const webVpnRecordDir = "./conf/webvpn_record"

func WebVpnRecordDir() string { return webVpnRecordDir }

// 将录像相对路径解析为绝对路径，拒绝目录穿越
func WebVpnRecordFullPath(rel string) (string, error) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || strings.HasPrefix(rel, "/") || webVpnHasDotDot(rel) || strings.ContainsRune(rel, 0) {
		return "", errors.New("无效的录像路径")
	}
	return filepath.Join(webVpnRecordDir, filepath.FromSlash(path.Clean(rel))), nil
}

func AddWebVpnSession(s *WebVpnSession) error {
	_, err := xdb.InsertOne(s)
	return err
}

// 结束会话：写入结束时间、流量与原因（recording 为空时保留录像开始时写入的路径）
func EndWebVpnSession(sid string, bytesIn, bytesOut int64, reason, recording string) error {
	now := time.Now()
	cols := []string{"ended_at", "bytes_in", "bytes_out", "close_reason"}
	if recording != "" {
		cols = append(cols, "recording_path")
	}
	_, err := xdb.Where("sid = ?", sid).Cols(cols...).
		Update(&WebVpnSession{EndedAt: &now, BytesIn: bytesIn, BytesOut: bytesOut, CloseReason: reason, RecordingPath: recording})
	return err
}

// 录像开始：尽早写入录像路径（在线会话也能判断是否已在录像）；仅首段生效
func SetWebVpnSessionRecording(sid, rel string) error {
	if xdb == nil {
		return errWebVpnDBNotReady
	}
	_, err := xdb.Table("webvpn_session").Where("sid = ? AND (recording_path = '' OR recording_path IS NULL)", sid).
		Update(map[string]any{"recording_path": rel, "record_err": ""})
	return err
}

// 录像创建失败：记录原因（截断到字段长度）
func SetWebVpnSessionRecordErr(sid, msg string) error {
	if xdb == nil {
		return errWebVpnDBNotReady
	}
	if r := []rune(msg); len(r) > 200 {
		msg = string(r[:200])
	}
	_, err := xdb.Table("webvpn_session").Where("sid = ?", sid).Update(map[string]any{"record_err": msg})
	return err
}

// 进程重启后，遗留的未结束会话统一标记为 shutdown
func CloseDanglingWebVpnSessions() {
	now := time.Now()
	_, err := xdb.Where("ended_at IS NULL").Cols("ended_at", "close_reason").
		Update(&WebVpnSession{EndedAt: &now, CloseReason: "shutdown"})
	if err != nil {
		base.Warn("清理遗留 WebVPN 会话失败:", err)
	}
}

func GetWebVpnSession(sid string) (*WebVpnSession, error) {
	s := &WebVpnSession{}
	if err := One("sid", sid, s); err != nil {
		return nil, err
	}
	return s, nil
}

type WebVpnSessionSearch struct {
	Username string
	AppName  string
	Date     []string
}

func WebVpnSessionList(pageSize, page int, search WebVpnSessionSearch) ([]WebVpnSession, int, error) {
	build := func(s *xorm.Session) *xorm.Session {
		if search.Username != "" {
			s.And("username = ?", search.Username)
		}
		if search.AppName != "" {
			s.And("app_name = ?", search.AppName)
		}
		if len(search.Date) == 2 && search.Date[0] != "" {
			s.And("started_at BETWEEN ? AND ?", search.Date[0], search.Date[1])
		}
		return s
	}
	count, err := build(xdb.Where("1=1")).Count(&WebVpnSession{})
	if err != nil {
		return nil, 0, err
	}
	var datas []WebVpnSession
	err = build(xdb.Where("1=1")).OrderBy("id desc").Limit(pageSize, (page-1)*pageSize).Find(&datas)
	return datas, int(count), err
}

func AddWebVpnFileLog(l *WebVpnFileLog) error {
	_, err := xdb.InsertOne(l)
	return err
}

type WebVpnFileLogSearch struct {
	Sid      string
	Username string
	AppName  string
	Op       string
	Date     []string
}

func WebVpnFileLogList(pageSize, page int, search WebVpnFileLogSearch) ([]WebVpnFileLog, int, error) {
	build := func(s *xorm.Session) *xorm.Session {
		if search.Sid != "" {
			s.And("sid = ?", search.Sid)
		}
		if search.Username != "" {
			s.And("username = ?", search.Username)
		}
		if search.AppName != "" {
			s.And("app_name = ?", search.AppName)
		}
		if search.Op != "" {
			s.And("op = ?", search.Op)
		}
		if len(search.Date) == 2 && search.Date[0] != "" {
			s.And("created_at BETWEEN ? AND ?", search.Date[0], search.Date[1])
		}
		return s
	}
	count, err := build(xdb.Where("1=1")).Count(&WebVpnFileLog{})
	if err != nil {
		return nil, 0, err
	}
	var datas []WebVpnFileLog
	err = build(xdb.Where("1=1")).OrderBy("id desc").Limit(pageSize, (page-1)*pageSize).Find(&datas)
	return datas, int(count), err
}
