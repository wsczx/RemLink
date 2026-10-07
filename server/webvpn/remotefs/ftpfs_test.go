package remotefs

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math/big"
	"net"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// ---------------- 测试用内存 FTP 服务端（fclairamb/ftpserverlib + afero 内存文件系统） ----------------

type testFTPDriver struct {
	fs       afero.Fs
	settings *ftpserver.Settings
	tlsc     *tls.Config
}

func (d *testFTPDriver) GetSettings() (*ftpserver.Settings, error) { return d.settings, nil }
func (d *testFTPDriver) ClientConnected(ftpserver.ClientContext) (string, error) {
	return "remlink test ftp", nil
}
func (d *testFTPDriver) ClientDisconnected(ftpserver.ClientContext) {}
func (d *testFTPDriver) AuthUser(_ ftpserver.ClientContext, user, pass string) (ftpserver.ClientDriver, error) {
	if user == "u" && pass == "p" {
		return d.fs, nil
	}
	return nil, errors.New("bad credentials")
}
func (d *testFTPDriver) GetTLSConfig() (*tls.Config, error) {
	if d.tlsc == nil {
		return nil, errors.New("tls not configured")
	}
	return d.tlsc, nil
}

func selfSignedTLS(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	require.NoError(t, err)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
}

func startFTP(t *testing.T, mode ftpserver.TLSRequirement, tlsc *tls.Config) (string, afero.Fs) {
	t.Helper()
	mfs := afero.NewMemMapFs()
	d := &testFTPDriver{
		fs:   mfs,
		tlsc: tlsc,
		settings: &ftpserver.Settings{
			ListenAddr:  "127.0.0.1:0",
			PublicHost:  "127.0.0.1",
			IdleTimeout: 60,
			TLSRequired: mode,
		},
	}
	srv := ftpserver.NewFtpServer(d)
	srv.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, srv.Listen())
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() { _ = srv.Stop() })
	return srv.Addr(), mfs
}

func dialTestFTP(t *testing.T, cfg FTPConfig) *FTPFS {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f, err := DialFTP(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func writeAll(t *testing.T, r RemoteFS, p string, data []byte, offset int64) {
	t.Helper()
	w, err := r.Create(p, offset)
	require.NoError(t, err)
	_, err = io.Copy(w, bytes.NewReader(data))
	require.NoError(t, err)
	require.NoError(t, w.Close())
}

func readAll(t *testing.T, r RemoteFS, p string) []byte {
	t.Helper()
	f, err := r.Open(p)
	require.NoError(t, err)
	defer f.Close()
	b, err := io.ReadAll(f)
	require.NoError(t, err)
	return b
}

func payload(n int, seed byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7) + seed
	}
	return b
}

// ---------------- 测试 ----------------

func TestFTPAuthFailed(t *testing.T) {
	addr, _ := startFTP(t, ftpserver.ClearOrEncrypted, nil)
	_, err := DialFTP(context.Background(), FTPConfig{Addr: addr, User: "u", Password: "wrong"})
	require.ErrorIs(t, err, ErrAuthFailed)
}

func TestFTPBasicOps(t *testing.T) {
	addr, _ := startFTP(t, ftpserver.ClearOrEncrypted, nil)
	f := dialTestFTP(t, FTPConfig{Addr: addr, User: "u", Password: "p"})
	ast := assert.New(t)
	ast.Equal("/", f.Home())

	require.NoError(t, f.MkdirAll("/a/b c/-d"))
	fi, err := f.Stat("/a/b c/-d")
	require.NoError(t, err)
	ast.True(fi.IsDir())
	ast.Equal("-d", fi.Name())

	data := payload(300<<10, 1)
	writeAll(t, f, "/a/b c/-d/file one.bin", data, 0)
	ast.Equal(data, readAll(t, f, "/a/b c/-d/file one.bin"))

	// 目录名含空格/以 "-" 开头时列目录仍正确
	list, err := SortedList(f, "/a/b c/-d")
	require.NoError(t, err)
	require.Len(t, list, 1)
	ast.Equal("file one.bin", list[0].Name())
	ast.Equal(int64(len(data)), list[0].Size())

	_, err = f.Stat("/a/none")
	ast.ErrorIs(err, fs.ErrNotExist)
	_, err = f.List("/a/none")
	ast.ErrorIs(err, fs.ErrNotExist)

	// 随机定位读取（REST + RETR），模拟 HTTP Range
	rf, err := f.Open("/a/b c/-d/file one.bin")
	require.NoError(t, err)
	end, err := rf.Seek(0, io.SeekEnd)
	require.NoError(t, err)
	ast.Equal(int64(len(data)), end)
	_, err = rf.Seek(1000, io.SeekStart)
	require.NoError(t, err)
	buf := make([]byte, 100)
	_, err = io.ReadFull(rf, buf)
	require.NoError(t, err)
	ast.Equal(data[1000:1100], buf)
	// 未读完即跳转：丢弃旧数据流并在新位置重新 RETR
	_, err = rf.Seek(200000, io.SeekStart)
	require.NoError(t, err)
	_, err = io.ReadFull(rf, buf)
	require.NoError(t, err)
	ast.Equal(data[200000:200100], buf)
	require.NoError(t, rf.Close()) // 中途关闭：连接被丢弃，后续操作不受影响

	// 断点续传：先写一半，再从 offset 续写
	half := len(data) / 2
	writeAll(t, f, "/a/resume.bin", data[:half], 0)
	fi, err = f.Stat("/a/resume.bin")
	require.NoError(t, err)
	ast.Equal(int64(half), fi.Size())
	writeAll(t, f, "/a/resume.bin", data[half:], int64(half))
	ast.Equal(data, readAll(t, f, "/a/resume.bin"))

	// 覆盖写
	writeAll(t, f, "/a/resume.bin", []byte("short"), 0)
	ast.Equal([]byte("short"), readAll(t, f, "/a/resume.bin"))
	// 空文件
	writeAll(t, f, "/a/empty", nil, 0)
	ast.Equal([]byte{}, append([]byte{}, readAll(t, f, "/a/empty")...))

	// 重命名 / 删除
	require.NoError(t, f.Rename("/a/resume.bin", "/a/renamed.bin"))
	_, err = f.Stat("/a/resume.bin")
	ast.ErrorIs(err, fs.ErrNotExist)
	require.NoError(t, f.Remove("/a/renamed.bin"))
	require.NoError(t, f.RemoveAll("/a/b c"))
	_, err = f.Stat("/a/b c")
	ast.ErrorIs(err, fs.ErrNotExist)
	ast.ErrorIs(f.Remove("/"), ErrBadPath)

	// 命令注入防护
	_, err = f.Stat("/a/x\r\nDELE y")
	ast.Error(err)
}

func TestFTPJailAndZip(t *testing.T) {
	addr, mfs := startFTP(t, ftpserver.ClearOrEncrypted, nil)
	require.NoError(t, mfs.MkdirAll("/srv/data/sub", 0755))
	require.NoError(t, mfs.MkdirAll("/secret", 0755))
	require.NoError(t, afero.WriteFile(mfs, "/srv/data/a.txt", []byte("hello"), 0644))
	require.NoError(t, afero.WriteFile(mfs, "/srv/data/sub/b.txt", []byte("world"), 0644))
	require.NoError(t, afero.WriteFile(mfs, "/secret/key", []byte("x"), 0644))

	f := dialTestFTP(t, FTPConfig{Addr: addr, User: "u", Password: "p"})
	j, err := NewJail(f, "/srv/data")
	require.NoError(t, err)
	abs, err := j.Resolve("/sub/b.txt")
	require.NoError(t, err)
	assert.Equal(t, "/srv/data/sub/b.txt", abs)
	_, err = j.Resolve("/../secret/key")
	assert.ErrorIs(t, err, ErrBadPath)

	var buf bytes.Buffer
	n, err := WriteZip(&buf, f, "/srv/data", "data", ZipLimits{MaxFiles: 10, MaxBytes: 1 << 20})
	require.NoError(t, err)
	assert.Equal(t, int64(10), n)
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	var names []string
	for _, zf := range zr.File {
		names = append(names, zf.Name)
	}
	sort.Strings(names)
	assert.Equal(t, []string{"data/", "data/a.txt", "data/sub/", "data/sub/b.txt"}, names)
}

// 同一会话并发 list / 下载 / 上传（连接池上限小于并发数，验证排队与控制连接互不干扰）
func TestFTPConcurrent(t *testing.T) {
	addr, _ := startFTP(t, ftpserver.ClearOrEncrypted, nil)
	f := dialTestFTP(t, FTPConfig{Addr: addr, User: "u", Password: "p", MaxConns: 3})
	require.NoError(t, f.MkdirAll("/c"))
	const n = 8
	files := make([][]byte, n)
	for i := 0; i < n; i++ {
		files[i] = payload(64<<10+i*1000, byte(i))
		writeAll(t, f, fmt.Sprintf("/c/f%d", i), files[i], 0)
	}
	var wg sync.WaitGroup
	errs := make(chan error, n*4)
	for i := 0; i < n; i++ {
		wg.Add(3)
		go func(i int) { // 下载（慢读，长时间占用连接）
			defer wg.Done()
			rf, err := f.Open(fmt.Sprintf("/c/f%d", i))
			if err != nil {
				errs <- err
				return
			}
			defer rf.Close()
			var out bytes.Buffer
			buf := make([]byte, 4096)
			for {
				k, err := rf.Read(buf)
				out.Write(buf[:k])
				if err == io.EOF {
					break
				}
				if err != nil {
					errs <- err
					return
				}
			}
			if !bytes.Equal(out.Bytes(), files[i]) {
				errs <- fmt.Errorf("download f%d mismatch", i)
			}
		}(i)
		go func(i int) { // 列目录
			defer wg.Done()
			if _, err := f.List("/c"); err != nil {
				errs <- err
			}
		}(i)
		go func(i int) { // 上传
			defer wg.Done()
			w, err := f.Create(fmt.Sprintf("/c/up%d", i), 0)
			if err != nil {
				errs <- err
				return
			}
			if _, err := w.Write(files[i]); err != nil {
				errs <- err
			}
			if err := w.Close(); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	for i := 0; i < n; i++ {
		assert.Equal(t, files[i], readAll(t, f, fmt.Sprintf("/c/up%d", i)))
	}
}

func TestFTPGBKNames(t *testing.T) {
	addr, mfs := startFTP(t, ftpserver.ClearOrEncrypted, nil)
	f := dialTestFTP(t, FTPConfig{Addr: addr, User: "u", Password: "p", Encoding: "gbk"})
	require.NoError(t, f.MkdirAll("/中文目录"))
	writeAll(t, f, "/中文目录/测试文件.txt", []byte("gbk"), 0)

	// 服务端实际存储的是 GBK 字节
	gbkDir, _ := simplifiedchinese.GBK.NewEncoder().String("中文目录")
	gbkFile, _ := simplifiedchinese.GBK.NewEncoder().String("测试文件.txt")
	ok, err := afero.Exists(mfs, "/"+gbkDir+"/"+gbkFile)
	require.NoError(t, err)
	assert.True(t, ok, "服务端应以 GBK 编码存储文件名")

	list, err := f.List("/中文目录")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "测试文件.txt", list[0].Name())
	assert.Equal(t, []byte("gbk"), readAll(t, f, "/中文目录/测试文件.txt"))

	// GBK 无法表示的字符
	_, err = f.Create("/😀.txt", 0)
	assert.ErrorIs(t, err, ErrBadPath)
}

func TestFTPExplicitAndImplicitTLS(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode ftpserver.TLSRequirement
		tls  string
	}{
		{"explicit", ftpserver.MandatoryEncryption, "explicit"},
		{"implicit", ftpserver.ImplicitEncryption, "implicit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr, _ := startFTP(t, tc.mode, selfSignedTLS(t))
			// 未跳过校验：自签证书应失败
			_, err := DialFTP(context.Background(), FTPConfig{Addr: addr, User: "u", Password: "p", TLS: tc.tls, ServerName: "127.0.0.1"})
			require.Error(t, err)
			f := dialTestFTP(t, FTPConfig{Addr: addr, User: "u", Password: "p", TLS: tc.tls, ServerName: "127.0.0.1", SkipVerify: true})
			data := payload(100<<10, 9)
			writeAll(t, f, "/tls.bin", data, 0)
			assert.Equal(t, data, readAll(t, f, "/tls.bin"))
			list, err := f.List("/")
			require.NoError(t, err)
			assert.Len(t, list, 1)
		})
	}
}

func TestFTPCloseReleasesPool(t *testing.T) {
	addr, _ := startFTP(t, ftpserver.ClearOrEncrypted, nil)
	f := dialTestFTP(t, FTPConfig{Addr: addr, User: "u", Password: "p", MaxConns: 2})
	writeAll(t, f, "/x", []byte(strings.Repeat("x", 1000)), 0)
	rf, err := f.Open("/x")
	require.NoError(t, err)
	_, err = rf.Read(make([]byte, 10)) // 占用一条连接
	require.NoError(t, err)
	require.NoError(t, f.Close())
	_ = rf.Close()
	_, err = f.List("/")
	assert.Error(t, err, "关闭后应拒绝操作")
}
