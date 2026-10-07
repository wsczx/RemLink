package remotefs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/hirochachacha/go-smb2"
)

// 认证失败（各协议实现统一返回该错误，网关据此计入防爆破计数）
var ErrAuthFailed = errors.New("目标主机认证失败")

// SMB 连接参数
type SMBConfig struct {
	Addr     string // host:port
	User     string // 可为 DOMAIN\user 或 user@domain
	Password string
	Domain   string // 为空时从 DOMAIN\user 中拆分
	Share    string
	Timeout  time.Duration
}

// 基于 github.com/hirochachacha/go-smb2 的 RemoteFS 实现（SMB 2.0.2 ~ 3.1.1，NTLM 认证）
//
// 符号链接/重解析点（含目录联接 junction）一律不跟随：Stat 等同 Lstat，ReadLink 返回 ErrUnsupported，
// 因此 Jail 解析经过任何重解析点的路径都会被拒绝；打包下载跳过重解析点；删除只删除链接本身。
type SMBFS struct {
	conn  net.Conn
	sess  *smb2.Session
	share *smb2.Share
}

// SMB 认证失败相关 NTSTATUS
var smbAuthStatus = map[uint32]bool{
	0xC000006D: true, // STATUS_LOGON_FAILURE
	0xC000006A: true, // STATUS_WRONG_PASSWORD
	0xC000006E: true, // STATUS_ACCOUNT_RESTRICTION
	0xC000006F: true, // STATUS_INVALID_LOGON_HOURS
	0xC0000070: true, // STATUS_INVALID_WORKSTATION
	0xC0000071: true, // STATUS_PASSWORD_EXPIRED
	0xC0000072: true, // STATUS_ACCOUNT_DISABLED
	0xC0000193: true, // STATUS_ACCOUNT_EXPIRED
	0xC0000224: true, // STATUS_PASSWORD_MUST_CHANGE
	0xC0000234: true, // STATUS_ACCOUNT_LOCKED_OUT
}

// 共享不存在
var ErrShareNotFound = errors.New("SMB 共享不存在或无权访问")

// 拆分 DOMAIN\user；domain 已配置时以配置为准
func SplitSMBUser(user, domain string) (string, string) {
	if domain == "" {
		if i := strings.IndexByte(user, '\\'); i > 0 && i < len(user)-1 {
			return user[i+1:], user[:i]
		}
	}
	return user, domain
}

// 建立 SMB 会话并挂载共享
func DialSMB(ctx context.Context, cfg SMBConfig) (*SMBFS, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	d := &net.Dialer{Timeout: cfg.Timeout, KeepAlive: 30 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", cfg.Addr)
	if err != nil {
		return nil, err
	}
	user, domain := SplitSMBUser(cfg.User, cfg.Domain)
	sd := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: user, Password: cfg.Password, Domain: domain}}
	hctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	sess, err := sd.DialContext(hctx, conn)
	if err != nil {
		conn.Close()
		var re *smb2.ResponseError
		if errors.As(err, &re) && smbAuthStatus[re.Code] {
			return nil, ErrAuthFailed
		}
		return nil, err
	}
	share, err := sess.WithContext(hctx).Mount(cfg.Share)
	if err != nil {
		_ = sess.Logoff()
		conn.Close()
		var re *smb2.ResponseError
		if errors.As(err, &re) && (re.Code == 0xC00000CC || re.Code == 0xC0000022) { // BAD_NETWORK_NAME / ACCESS_DENIED
			return nil, ErrShareNotFound
		}
		if errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrNotExist) {
			return nil, ErrShareNotFound
		}
		return nil, err
	}
	return &SMBFS{conn: conn, sess: sess, share: share}, nil
}

// 将 Jail 使用的 "/" 绝对路径映射为共享内相对路径（"\" 分隔，根为空串）
// 拒绝：非绝对路径、. / .. 段、含 "\"（防止在 SMB 侧被当作分隔符绕过 Jail）、含 ":"（NTFS 备用数据流）及控制字符
func SMBPath(p string) (string, error) {
	if !strings.HasPrefix(p, "/") {
		return "", ErrBadPath
	}
	var segs []string
	for _, seg := range strings.Split(p, "/") {
		if seg == "" {
			continue
		}
		if seg == "." || seg == ".." || strings.ContainsAny(seg, `\:`) {
			return "", ErrBadPath
		}
		for _, c := range seg {
			if c < 0x20 {
				return "", ErrBadPath
			}
		}
		segs = append(segs, seg)
	}
	return strings.Join(segs, `\`), nil
}

// 统一以请求路径的末段作为文件名（go-smb2 的 FileStat 名称可能带路径）
type namedInfo struct {
	fs.FileInfo
	name string
}

func (n namedInfo) Name() string { return n.name }

func baseName(p string) string {
	b := path.Base(p)
	if b == "." || b == "" {
		return "/"
	}
	return b
}

func (s *SMBFS) List(p string) ([]fs.FileInfo, error) {
	sp, err := SMBPath(p)
	if err != nil {
		return nil, err
	}
	list, err := s.share.ReadDir(sp)
	if err != nil {
		return nil, err
	}
	out := make([]fs.FileInfo, 0, len(list))
	for _, fi := range list {
		out = append(out, fi)
	}
	return out, nil
}

// 不跟随重解析点：Stat 与 Lstat 相同
func (s *SMBFS) Stat(p string) (fs.FileInfo, error) { return s.Lstat(p) }

func (s *SMBFS) Lstat(p string) (fs.FileInfo, error) {
	sp, err := SMBPath(p)
	if err != nil {
		return nil, err
	}
	fi, err := s.share.Lstat(sp)
	if err != nil {
		return nil, err
	}
	return namedInfo{FileInfo: fi, name: baseName(p)}, nil
}

// 不解析重解析点目标：Jail 遇到任何链接都会拒绝该路径
func (s *SMBFS) ReadLink(p string) (string, error) {
	return "", &fs.PathError{Op: "readlink", Path: p, Err: ErrUnsupported}
}

type smbFile struct {
	*smb2.File
	name string
}

func (f smbFile) Stat() (fs.FileInfo, error) {
	fi, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	return namedInfo{FileInfo: fi, name: f.name}, nil
}

func (s *SMBFS) Open(p string) (File, error) {
	sp, err := SMBPath(p)
	if err != nil {
		return nil, err
	}
	f, err := s.share.Open(sp)
	if err != nil {
		return nil, err
	}
	return smbFile{File: f, name: baseName(p)}, nil
}

func (s *SMBFS) Create(p string, offset int64) (io.WriteCloser, error) {
	sp, err := SMBPath(p)
	if err != nil {
		return nil, err
	}
	if offset <= 0 {
		return s.share.OpenFile(sp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	}
	f, err := s.share.OpenFile(sp, os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (s *SMBFS) Mkdir(p string) error {
	sp, err := SMBPath(p)
	if err != nil {
		return err
	}
	if sp == "" {
		return fs.ErrExist
	}
	return s.share.Mkdir(sp, 0755)
}

func (s *SMBFS) MkdirAll(p string) error { return MkdirAllGeneric(s, p) }

// go-smb2 的 Remove 以 FILE_OPEN_REPARSE_POINT 打开，作用于链接本身；可删除文件与空目录
func (s *SMBFS) Remove(p string) error {
	sp, err := SMBPath(p)
	if err != nil {
		return err
	}
	if sp == "" {
		return ErrBadPath
	}
	return s.share.Remove(sp)
}

func (s *SMBFS) RemoveAll(p string) error { return RemoveAllGeneric(s, p) }

func (s *SMBFS) Rename(from, to string) error {
	f, err := SMBPath(from)
	if err != nil {
		return err
	}
	t, err := SMBPath(to)
	if err != nil {
		return err
	}
	if f == "" || t == "" {
		return ErrBadPath
	}
	return s.share.Rename(f, t)
}

// 保活/探活：查询共享根属性
func (s *SMBFS) Ping() error {
	_, err := s.share.Lstat("")
	return err
}

func (s *SMBFS) Close() error {
	_ = s.share.Umount()
	_ = s.sess.Logoff()
	return s.conn.Close()
}
