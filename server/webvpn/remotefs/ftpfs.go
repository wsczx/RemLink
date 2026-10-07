package remotefs

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/textproto"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jlaffaye/ftp"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// FTP/FTPS 连接参数
type FTPConfig struct {
	Addr       string // host:port
	User       string
	Password   string
	TLS        string // none|explicit|implicit
	ServerName string // TLS 校验用主机名（目标主机）
	SkipVerify bool
	Encoding   string        // utf-8|gbk
	MaxConns   int           // 单会话最大并发控制连接数；<=0 取 4
	Timeout    time.Duration // 建连/登录超时；<=0 取 15s
	// 每个操作（命令往返/数据传输空闲）的读写超时；<=0 取 60s
	IOTimeout time.Duration
}

const (
	ftpDefaultConns   = 4
	ftpAcquireTimeout = 60 * time.Second
	ftpIdleProbe      = 30 * time.Second // 空闲超过该时长的连接复用前先 NOOP 探活
	ftpListCacheTTL   = 3 * time.Second  // 目录列表短缓存（Jail 逐级 Lstat 会反复列父目录）
)

var errFTPBusy = errors.New("FTP 连接繁忙，请稍后重试")

// 基于 github.com/jlaffaye/ftp 的 RemoteFS 实现
//
// ftp.ServerConn 非并发安全，且数据传输期间控制连接被占用：
// 本实现维护一个按会话的连接池（容量 MaxConns），每个操作独占一条控制连接；
// 下载流（Open）在整个读取期间独占一条连接，读完/关闭后归还；中途中断的连接直接丢弃，避免协议状态错乱。
// 仅支持被动模式（EPSV/PASV），数据连接地址固定使用控制连接的对端 IP（忽略 PASV 返回的 IP，防 FTP bounce）。
type FTPFS struct {
	cfg    FTPConfig
	tlsc   *tls.Config
	gbk    bool
	slots  chan *ftpConn // 令牌：nil 表示可新建连接的空位
	closed atomic.Bool

	cmu   sync.Mutex
	cache map[string]ftpCacheEntry

	home string
}

type ftpConn struct {
	c    *ftp.ServerConn
	last time.Time
}

type ftpCacheEntry struct {
	at      time.Time
	entries []fs.FileInfo
}

// 连接并登录（首条连接同时用于校验凭据与获取初始目录）
func DialFTP(ctx context.Context, cfg FTPConfig) (*FTPFS, error) {
	if cfg.MaxConns <= 0 {
		cfg.MaxConns = ftpDefaultConns
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.IOTimeout <= 0 {
		cfg.IOTimeout = 60 * time.Second
	}
	f := &FTPFS{
		cfg:   cfg,
		gbk:   strings.EqualFold(cfg.Encoding, "gbk"),
		slots: make(chan *ftpConn, cfg.MaxConns),
		cache: make(map[string]ftpCacheEntry),
	}
	if cfg.TLS == "explicit" || cfg.TLS == "implicit" {
		f.tlsc = &tls.Config{
			ServerName:         cfg.ServerName,
			InsecureSkipVerify: cfg.SkipVerify,
			// 多数 FTPS 服务端（如 vsftpd require_ssl_reuse）要求数据连接复用控制连接的 TLS 会话
			ClientSessionCache: tls.NewLRUClientSessionCache(32),
		}
	}
	c, err := f.dial(ctx)
	if err != nil {
		return nil, err
	}
	if wd, err := c.CurrentDir(); err == nil && strings.HasPrefix(wd, "/") {
		f.home = f.dec(wd)
	} else {
		f.home = "/"
	}
	f.slots <- &ftpConn{c: c, last: time.Now()}
	for i := 1; i < cfg.MaxConns; i++ {
		f.slots <- nil
	}
	return f, nil
}

// 登录后的初始工作目录（未配置根目录时作为 Jail 根）
func (f *FTPFS) Home() string { return f.home }

func (f *FTPFS) dial(ctx context.Context) (*ftp.ServerConn, error) {
	// 自定义拨号：控制/数据连接均包一层逐次读写超时（库本身没有命令级超时），并自行处理 TLS
	// （设置 DialFunc 后库不再为隐式 TLS 控制连接与数据连接做 TLS 包装）
	first := true
	var dmu sync.Mutex
	dialFn := func(network, addr string) (net.Conn, error) {
		dmu.Lock()
		isCtrl := first
		first = false
		dmu.Unlock()
		d := &net.Dialer{Timeout: f.cfg.Timeout, KeepAlive: 30 * time.Second}
		dctx := ctx
		if !isCtrl {
			var cancel context.CancelFunc
			dctx, cancel = context.WithTimeout(context.Background(), f.cfg.Timeout)
			defer cancel()
		}
		raw, err := d.DialContext(dctx, network, addr)
		if err != nil {
			return nil, err
		}
		var conn net.Conn = &deadlineConn{Conn: raw, d: f.cfg.IOTimeout}
		if f.tlsc != nil && (!isCtrl || f.cfg.TLS == "implicit") {
			conn = tls.Client(conn, f.tlsc)
		}
		return conn, nil
	}
	opts := []ftp.DialOption{
		ftp.DialWithDialFunc(dialFn),
		ftp.DialWithDisabledUTF8(f.gbk),
	}
	switch f.cfg.TLS {
	case "explicit":
		opts = append(opts, ftp.DialWithExplicitTLS(f.tlsc))
	case "implicit":
		// 仅用于让库在登录后发送 PBSZ/PROT P；TLS 包装由 dialFn 完成
		opts = append(opts, ftp.DialWithTLS(f.tlsc))
	}
	c, err := ftp.Dial(f.cfg.Addr, opts...)
	if err != nil {
		return nil, err
	}
	user, err := f.enc(f.cfg.User)
	if err != nil {
		_ = c.Quit()
		return nil, ErrAuthFailed
	}
	pass, err := f.enc(f.cfg.Password)
	if err != nil {
		_ = c.Quit()
		return nil, ErrAuthFailed
	}
	if err := c.Login(user, pass); err != nil {
		_ = c.Quit()
		var te *textproto.Error
		if errors.As(err, &te) && (te.Code == 530 || te.Code == 430 || te.Code == 332) {
			return nil, ErrAuthFailed
		}
		return nil, err
	}
	return c, nil
}

// 取一条独占连接
func (f *FTPFS) acquire() (*ftpConn, error) {
	if f.closed.Load() {
		return nil, errors.New("会话已结束")
	}
	t := time.NewTimer(ftpAcquireTimeout)
	defer t.Stop()
	var pc *ftpConn
	select {
	case pc = <-f.slots:
	case <-t.C:
		return nil, errFTPBusy
	}
	if f.closed.Load() {
		if pc != nil {
			_ = pc.c.Quit()
		}
		f.slots <- nil
		return nil, errors.New("会话已结束")
	}
	if pc != nil && time.Since(pc.last) > ftpIdleProbe {
		if err := pc.c.NoOp(); err != nil {
			_ = pc.c.Quit()
			pc = nil
		}
	}
	if pc == nil {
		ctx, cancel := context.WithTimeout(context.Background(), f.cfg.Timeout)
		c, err := f.dial(ctx)
		cancel()
		if err != nil {
			f.slots <- nil
			return nil, err
		}
		pc = &ftpConn{c: c}
	}
	return pc, nil
}

// 归还连接；ok=false（协议状态不确定）时直接丢弃
func (f *FTPFS) release(pc *ftpConn, ok bool) {
	if pc == nil {
		return
	}
	if !ok || f.closed.Load() {
		_ = pc.c.Quit()
		f.slots <- nil
		return
	}
	pc.last = time.Now()
	f.slots <- pc
}

// 以独占连接执行一次控制连接操作；网络/协议层错误时丢弃连接
func (f *FTPFS) do(fn func(c *ftp.ServerConn) error) error {
	pc, err := f.acquire()
	if err != nil {
		return err
	}
	err = fn(pc.c)
	f.release(pc, err == nil || isFTPReply(err))
	return err
}

// 服务端正常应答的错误（4xx/5xx），连接仍可复用
func isFTPReply(err error) bool {
	var te *textproto.Error
	return errors.As(err, &te)
}

func ftpCode(err error) int {
	var te *textproto.Error
	if errors.As(err, &te) {
		return te.Code
	}
	return 0
}

// ---------------- 编码 ----------------

// UTF-8 → 服务端编码（GBK 时返回 GBK 字节构成的 string）
func (f *FTPFS) enc(s string) (string, error) {
	if !f.gbk {
		return s, nil
	}
	b, err := simplifiedchinese.GBK.NewEncoder().String(s)
	if err != nil {
		return "", ErrBadPath
	}
	return b, nil
}

// 服务端编码 → UTF-8
func (f *FTPFS) dec(s string) string {
	if !f.gbk {
		return s
	}
	b, err := simplifiedchinese.GBK.NewDecoder().String(s)
	if err != nil {
		return s
	}
	return b
}

func (f *FTPFS) encPath(p string) (string, error) {
	if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\r\n\x00") {
		return "", ErrBadPath
	}
	return f.enc(path.Clean(p))
}

// ---------------- 错误映射 ----------------

// 读类操作：550 视为不存在
func readErr(op, p string, err error) error {
	switch ftpCode(err) {
	case 550, 450:
		return &fs.PathError{Op: op, Path: p, Err: fs.ErrNotExist}
	case 530, 532, 553:
		return &fs.PathError{Op: op, Path: p, Err: fs.ErrPermission}
	}
	return ftpWrap(op, p, err)
}

// 写类操作：550/553 视为拒绝
func writeErr(op, p string, err error) error {
	switch ftpCode(err) {
	case 550, 553, 530, 532, 450:
		return &fs.PathError{Op: op, Path: p, Err: fs.ErrPermission}
	}
	return ftpWrap(op, p, err)
}

func ftpWrap(op, p string, err error) error {
	if err == nil {
		return nil
	}
	return &fs.PathError{Op: op, Path: p, Err: err}
}

// ---------------- 目录与元数据 ----------------

type ftpInfo struct {
	name   string
	size   int64
	mode   fs.FileMode
	mtime  time.Time
	target string // 符号链接目标（LIST 的 "name -> target"）
}

func (i *ftpInfo) Name() string       { return i.name }
func (i *ftpInfo) Size() int64        { return i.size }
func (i *ftpInfo) Mode() fs.FileMode  { return i.mode }
func (i *ftpInfo) ModTime() time.Time { return i.mtime }
func (i *ftpInfo) IsDir() bool        { return i.mode.IsDir() }
func (i *ftpInfo) Sys() any           { return nil }

func rootInfo() *ftpInfo { return &ftpInfo{name: "/", mode: fs.ModeDir | 0755} }

func (f *FTPFS) toInfo(e *ftp.Entry) *ftpInfo {
	i := &ftpInfo{name: f.dec(e.Name), size: int64(e.Size), mtime: e.Time}
	switch e.Type {
	case ftp.EntryTypeFolder:
		i.mode = fs.ModeDir | 0755
		i.size = 0
	case ftp.EntryTypeLink:
		i.mode = fs.ModeSymlink | 0777
		i.target = f.dec(e.Target)
	default:
		i.mode = 0644
	}
	return i
}

func (f *FTPFS) invalidate() {
	f.cmu.Lock()
	clear(f.cache)
	f.cmu.Unlock()
}

func (f *FTPFS) list(p string, useCache bool) ([]fs.FileInfo, error) {
	p = path.Clean(p)
	if useCache {
		f.cmu.Lock()
		e, ok := f.cache[p]
		f.cmu.Unlock()
		if ok && time.Since(e.at) < ftpListCacheTTL {
			return e.entries, nil
		}
	}
	ep, err := f.encPath(p)
	if err != nil {
		return nil, err
	}
	var entries []*ftp.Entry
	err = f.do(func(c *ftp.ServerConn) error {
		// 先 CWD 再无参 LIST/MLSD：避免含空格或以 "-" 开头的路径被部分服务端当作 ls 参数
		if err := c.ChangeDir(ep); err != nil {
			return err
		}
		var err error
		entries, err = c.List("")
		return err
	})
	if err != nil {
		return nil, readErr("list", p, err)
	}
	out := make([]fs.FileInfo, 0, len(entries))
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." || e.Name == "" {
			continue
		}
		out = append(out, f.toInfo(e))
	}
	f.cmu.Lock()
	if len(f.cache) > 256 {
		clear(f.cache)
	}
	f.cache[p] = ftpCacheEntry{at: time.Now(), entries: out}
	f.cmu.Unlock()
	return out, nil
}

// 列目录（总是取最新内容并刷新缓存）
func (f *FTPFS) List(p string) ([]fs.FileInfo, error) { return f.list(p, false) }

// FTP 无单文件 lstat 的通用命令：通过列父目录查找（带短缓存）
func (f *FTPFS) Lstat(p string) (fs.FileInfo, error) {
	p = path.Clean(p)
	if p == "/" {
		return rootInfo(), nil
	}
	if !strings.HasPrefix(p, "/") {
		return nil, ErrBadPath
	}
	dir, name := path.Split(p)
	entries, err := f.list(dir, true)
	if err != nil {
		// 父目录不可列（如非 chroot 服务端的 /home）：能 CWD 进入则视为目录（用于根目录/家目录的逐级校验）
		if ep, eerr := f.encPath(p); eerr == nil {
			if f.do(func(c *ftp.ServerConn) error { return c.ChangeDir(ep) }) == nil {
				return &ftpInfo{name: name, mode: fs.ModeDir | 0755}, nil
			}
		}
		return nil, err
	}
	for _, e := range entries {
		if e.Name() == name {
			return e, nil
		}
	}
	return nil, &fs.PathError{Op: "lstat", Path: p, Err: fs.ErrNotExist}
}

// 跟随符号链接（服务端在 RETR/CWD 时会跟随，Stat 与之保持一致）
func (f *FTPFS) Stat(p string) (fs.FileInfo, error) {
	cur := path.Clean(p)
	for hop := 0; hop <= maxLinkHops; hop++ {
		fi, err := f.Lstat(cur)
		if err != nil {
			return nil, err
		}
		li, ok := fi.(*ftpInfo)
		if !ok || fi.Mode()&fs.ModeSymlink == 0 {
			if cur != path.Clean(p) {
				// 以原名返回目标属性
				c := *li
				c.name = path.Base(p)
				return &c, nil
			}
			return fi, nil
		}
		if li.target == "" {
			return nil, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
		}
		t := li.target
		if !strings.HasPrefix(t, "/") {
			t = path.Join(path.Dir(cur), t)
		}
		cur = path.Clean(t)
	}
	return nil, &fs.PathError{Op: "stat", Path: p, Err: ErrEscape}
}

func (f *FTPFS) ReadLink(p string) (string, error) {
	fi, err := f.Lstat(p)
	if err != nil {
		return "", err
	}
	li, ok := fi.(*ftpInfo)
	if !ok || fi.Mode()&fs.ModeSymlink == 0 || li.target == "" {
		return "", &fs.PathError{Op: "readlink", Path: p, Err: ErrUnsupported}
	}
	return li.target, nil
}

// ---------------- 读 ----------------

type ftpFile struct {
	f    *FTPFS
	p    string
	ep   string
	info *ftpInfo
	pos  int64

	pc   *ftpConn
	resp *ftp.Response
	rpos int64 // 当前数据流位置
}

func (f *FTPFS) Open(p string) (File, error) {
	fi, err := f.Stat(p)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, &fs.PathError{Op: "open", Path: p, Err: errors.New("是目录")}
	}
	ep, err := f.encPath(p)
	if err != nil {
		return nil, err
	}
	info, _ := fi.(*ftpInfo)
	if info == nil {
		info = &ftpInfo{name: fi.Name(), size: fi.Size(), mode: fi.Mode(), mtime: fi.ModTime()}
	}
	return &ftpFile{f: f, p: p, ep: ep, info: info}, nil
}

func (r *ftpFile) Stat() (fs.FileInfo, error) { return r.info, nil }

// 关闭当前数据流；complete=false 表示中途中断，连接丢弃
func (r *ftpFile) closeStream(complete bool) error {
	if r.resp == nil {
		return nil
	}
	err := r.resp.Close()
	r.f.release(r.pc, complete && err == nil)
	r.resp, r.pc = nil, nil
	return err
}

func (r *ftpFile) Read(b []byte) (int, error) {
	if r.resp != nil && r.rpos != r.pos {
		_ = r.closeStream(false)
	}
	if r.resp == nil {
		if r.pos >= r.info.size && r.info.size > 0 {
			return 0, io.EOF
		}
		pc, err := r.f.acquire()
		if err != nil {
			return 0, err
		}
		resp, err := pc.c.RetrFrom(r.ep, uint64(r.pos))
		if err != nil {
			r.f.release(pc, isFTPReply(err))
			return 0, readErr("read", r.p, err)
		}
		r.pc, r.resp, r.rpos = pc, resp, r.pos
	}
	n, err := r.resp.Read(b)
	r.pos += int64(n)
	r.rpos = r.pos
	if err == io.EOF {
		if cerr := r.closeStream(true); cerr != nil {
			return n, cerr
		}
		return n, io.EOF
	}
	if err != nil {
		_ = r.closeStream(false)
	}
	return n, err
}

// Seek 只记录位置，下次 Read 时按需以 REST 重新打开数据流（http.ServeContent 会先 Seek 到末尾求大小）
func (r *ftpFile) Seek(offset int64, whence int) (int64, error) {
	var np int64
	switch whence {
	case io.SeekStart:
		np = offset
	case io.SeekCurrent:
		np = r.pos + offset
	case io.SeekEnd:
		np = r.info.size + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if np < 0 {
		return 0, errors.New("negative position")
	}
	r.pos = np
	return np, nil
}

func (r *ftpFile) Close() error {
	if r.resp == nil {
		return nil
	}
	// 未读完即关闭：服务端会回 426/226，协议状态不确定，丢弃该连接
	_ = r.closeStream(false)
	return nil
}

// ---------------- 写 ----------------

type ftpWriter struct {
	f    *FTPFS
	pw   *io.PipeWriter
	done chan error
	once sync.Once
	err  error
}

type countReader struct {
	r io.Reader
	n atomic.Int64
}

func (c *countReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n.Add(int64(n))
	return n, err
}

// offset==0：STOR 新建/截断；offset>0：REST+STOR 续写，服务端不支持 REST STOR 时回退为 APPE
// （网关在续传前已校验远端大小 == offset，APPE 语义等价）
func (f *FTPFS) Create(p string, offset int64) (io.WriteCloser, error) {
	ep, err := f.encPath(p)
	if err != nil {
		return nil, err
	}
	pc, err := f.acquire()
	if err != nil {
		return nil, err
	}
	f.invalidate()
	pr, pw := io.Pipe()
	w := &ftpWriter{f: f, pw: pw, done: make(chan error, 1)}
	go func() {
		cr := &countReader{r: pr}
		var err error
		if offset <= 0 {
			err = pc.c.Stor(ep, cr)
		} else {
			err = pc.c.StorFrom(ep, cr, uint64(offset))
			if err != nil && cr.n.Load() == 0 && isFTPReply(err) {
				switch ftpCode(err) {
				case 500, 501, 502, 504, 350:
					err = pc.c.Append(ep, cr)
				}
			}
		}
		if err != nil {
			err = writeErr("upload", p, err)
		}
		// 上游已失败时让写端立即得到错误
		pr.CloseWithError(errOr(err, io.ErrClosedPipe))
		f.release(pc, err == nil)
		w.done <- err
	}()
	return w, nil
}

func errOr(err, def error) error {
	if err != nil {
		return err
	}
	return def
}

func (w *ftpWriter) Write(b []byte) (int, error) { return w.pw.Write(b) }

func (w *ftpWriter) Close() error {
	w.once.Do(func() {
		_ = w.pw.Close()
		w.err = <-w.done
		w.f.invalidate()
	})
	return w.err
}

// ---------------- 修改 ----------------

func (f *FTPFS) Mkdir(p string) error {
	ep, err := f.encPath(p)
	if err != nil {
		return err
	}
	defer f.invalidate()
	return writeErr("mkdir", p, f.do(func(c *ftp.ServerConn) error { return c.MakeDir(ep) }))
}

func (f *FTPFS) MkdirAll(p string) error { return MkdirAllGeneric(f, p) }

func (f *FTPFS) Remove(p string) error {
	if path.Clean(p) == "/" {
		return ErrBadPath
	}
	fi, err := f.Lstat(p)
	if err != nil {
		return err
	}
	ep, err := f.encPath(p)
	if err != nil {
		return err
	}
	defer f.invalidate()
	if fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0 {
		return writeErr("rmdir", p, f.do(func(c *ftp.ServerConn) error { return c.RemoveDir(ep) }))
	}
	return writeErr("delete", p, f.do(func(c *ftp.ServerConn) error { return c.Delete(ep) }))
}

func (f *FTPFS) RemoveAll(p string) error { return RemoveAllGeneric(f, p) }

func (f *FTPFS) Rename(from, to string) error {
	ef, err := f.encPath(from)
	if err != nil {
		return err
	}
	et, err := f.encPath(to)
	if err != nil {
		return err
	}
	defer f.invalidate()
	return writeErr("rename", from, f.do(func(c *ftp.ServerConn) error { return c.Rename(ef, et) }))
}

// 关闭全部空闲连接；使用中的连接在归还时关闭
func (f *FTPFS) Close() error {
	if !f.closed.CompareAndSwap(false, true) {
		return nil
	}
	for {
		select {
		case pc := <-f.slots:
			if pc != nil {
				_ = pc.c.Quit()
			}
			defer func() { f.slots <- nil }()
		default:
			return nil
		}
	}
}

// 每次 Read/Write 前设置超时，防止服务端无响应时永久阻塞（空闲期间不读写，不受影响）
type deadlineConn struct {
	net.Conn
	d time.Duration
}

func (c *deadlineConn) Read(b []byte) (int, error) {
	_ = c.Conn.SetReadDeadline(time.Now().Add(c.d))
	return c.Conn.Read(b)
}

func (c *deadlineConn) Write(b []byte) (int, error) {
	_ = c.Conn.SetWriteDeadline(time.Now().Add(c.d))
	return c.Conn.Write(b)
}

func (f *FTPFS) String() string { return fmt.Sprintf("ftp://%s", f.cfg.Addr) }
