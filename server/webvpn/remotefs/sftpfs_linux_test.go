//go:build linux

package remotefs

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/sftp"
)

// 用 pkg/sftp 的进程内服务端（直接操作本机临时目录）验证 SFTPFS + Jail 的真实行为
func newSFTPPair(t *testing.T) *SFTPFS {
	t.Helper()
	c1, c2 := net.Pipe()
	srv, err := sftp.NewServer(c1)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	cl, err := sftp.NewClientPipe(c2, c2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close(); srv.Close() })
	return NewSFTP(cl)
}

func TestSFTPJailReal(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "d/sub"), 0755))
	must(os.MkdirAll(outside, 0755))
	must(os.WriteFile(filepath.Join(root, "d/a.txt"), []byte("hello"), 0644))
	must(os.WriteFile(filepath.Join(root, "d/sub/b.txt"), []byte("world"), 0644))
	must(os.WriteFile(filepath.Join(outside, "secret"), []byte("top"), 0644))
	must(os.Symlink(outside, filepath.Join(root, "out")))
	must(os.Symlink(filepath.Join(outside, "new"), filepath.Join(root, "dangling")))
	must(os.Symlink(outside, filepath.Join(root, "d/link-out")))

	fs := newSFTPPair(t)
	j, err := NewJail(fs, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../outside/secret", "out/secret", "out", "dangling", "d/link-out/secret"} {
		if _, err := j.Resolve(bad); err == nil {
			t.Errorf("Resolve(%q) 应拒绝", bad)
		}
	}
	abs, err := j.Resolve("d/new/dir/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	must(fs.MkdirAll(filepath.Dir(abs)))
	w, err := fs.Create(abs, 0)
	must(err)
	w.Write([]byte("abc"))
	must(w.Close())
	w, err = fs.Create(abs, 3)
	must(err)
	w.Write([]byte("def"))
	must(w.Close())
	b, _ := os.ReadFile(filepath.Join(root, "d/new/dir/file.txt"))
	if string(b) != "abcdef" {
		t.Fatalf("续传写入错误: %q", b)
	}

	// zip 不跟随 d/link-out
	var buf bytes.Buffer
	if _, err := WriteZip(&buf, fs, filepath.Join(root, "d"), "d", ZipLimits{MaxFiles: 100}); err != nil {
		t.Fatal(err)
	}
	files := readZip(t, buf.Bytes())
	if _, ok := files["d/link-out/secret"]; ok {
		t.Fatal("zip 跟随了符号链接")
	}
	if files["d/a.txt"] != "hello" || files["d/new/dir/file.txt"] != "abcdef" {
		t.Fatalf("zip 内容错误: %v", files)
	}

	// 递归删除不跟随链接
	must(fs.RemoveAll(filepath.Join(root, "d")))
	if _, err := os.Stat(filepath.Join(outside, "secret")); err != nil {
		t.Fatal("根外文件被删除")
	}
	if _, err := fs.Lstat(filepath.Join(root, "d")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("目录应已删除: %v", err)
	}
	f, err := fs.Open(filepath.Join(outside, "secret")) // RemoteFS 本身不设限，限制由 Jail 负责
	must(err)
	io.Copy(io.Discard, f)
	f.Close()
}
