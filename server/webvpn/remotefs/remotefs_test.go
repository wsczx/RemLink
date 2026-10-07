package remotefs

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"
)

func sampleFS() *memFS {
	m := newMemFS()
	m.mkdir("/home")
	m.mkdir("/home/u")
	m.mkdir("/home/u/docs")
	m.file("/home/u/docs/a.txt", "hello")
	m.mkdir("/home/u/docs/sub")
	m.file("/home/u/docs/sub/b.txt", "world!")
	m.mkdir("/etc")
	m.file("/etc/passwd", "root:x:0:0")
	m.symlink("/home/u/out", "/etc")                        // 指向根外
	m.symlink("/home/u/docs/in", "/home/u/docs/sub")        // 根内链接
	m.symlink("/home/u/dangling", "/etc/newfile")           // 悬空且指向根外
	m.symlink("/home/u/docs/sub/escape.txt", "/etc/passwd") // 根内目录里指向根外文件
	return m
}

func TestCleanRel(t *testing.T) {
	ok := map[string]string{
		"":           "/",
		"/":          "/",
		"a/b":        "/a/b",
		"/a//b/./c/": "/a/b/c",
		"a b/中文.txt": "/a b/中文.txt",
		"a\\b":       "/a\\b",
		"...":        "/...",
		"..a/b..":    "/..a/b..",
	}
	for in, want := range ok {
		got, err := CleanRel(in)
		if err != nil || got != want {
			t.Errorf("CleanRel(%q)=%q,%v want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"..", "../etc", "a/../../etc", "/a/..", "a/b/../c", "a\x00b"} {
		if _, err := CleanRel(bad); err == nil {
			t.Errorf("CleanRel(%q) 应拒绝", bad)
		}
	}
}

func TestJailResolve(t *testing.T) {
	m := sampleFS()
	j, err := NewJail(m, "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		rel     string
		wantErr bool
		want    string
	}{
		{"docs/a.txt", false, "/home/u/docs/a.txt"},
		{"/docs/sub", false, "/home/u/docs/sub"},
		{"docs/in/b.txt", false, "/home/u/docs/in/b.txt"},   // 根内符号链接允许
		{"newdir/x/y.txt", false, "/home/u/newdir/x/y.txt"}, // 不存在的路径校验祖先
		{"../etc/passwd", true, ""},
		{"docs/../../etc", true, ""},
		{"out", true, ""},
		{"out/passwd", true, ""},
		{"out/newfile", true, ""},
		{"dangling", true, ""}, // 悬空链接指向根外：禁止（否则 Create 会在根外建文件）
		{"docs/sub/escape.txt", true, ""},
	}
	for _, c := range cases {
		got, err := j.Resolve(c.rel)
		if c.wantErr {
			if err == nil {
				t.Errorf("Resolve(%q) 应失败，得到 %q", c.rel, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("Resolve(%q)=%q,%v want %q", c.rel, got, err, c.want)
		}
	}
	if j.Rel("/home/u/docs/a.txt") != "/docs/a.txt" || j.Rel("/home/u") != "/" {
		t.Error("Rel 转换错误")
	}
}

func TestRealPathOf(t *testing.T) {
	m := sampleFS()
	m.symlink("/home/u/rel", "docs/../docs/sub") // 相对链接
	m.symlink("/home/u/loop1", "/home/u/loop2")
	m.symlink("/home/u/loop2", "/home/u/loop1")
	cases := map[string]string{
		"/home/u/docs/in/b.txt":    "/home/u/docs/sub/b.txt",
		"/home/u/out/passwd":       "/etc/passwd",
		"/home/u/dangling":         "/etc/newfile",
		"/home/u/rel/b.txt":        "/home/u/docs/sub/b.txt",
		"/home/u/nonexist/a/b":     "/home/u/nonexist/a/b",
		"/home/u/docs/in/../a.txt": "/home/u/docs/a.txt", // 物理语义：.. 相对链接目标
	}
	for in, want := range cases {
		got, err := RealPathOf(m, in)
		if err != nil || got != want {
			t.Errorf("RealPathOf(%q)=%q,%v want %q", in, got, err, want)
		}
	}
	if _, err := RealPathOf(m, "/home/u/loop1/x"); err == nil {
		t.Error("循环链接应报错")
	}
}

func TestJailRootSymlink(t *testing.T) {
	m := sampleFS()
	// 根目录本身是符号链接：以真实路径为界
	m.symlink("/home/link", "/home/u")
	j, err := NewJail(m, "/home/link")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Resolve("docs/a.txt"); err != nil {
		t.Errorf("根为链接时根内路径应允许: %v", err)
	}
	if _, err := j.Resolve("out"); !errors.Is(err, ErrEscape) {
		t.Errorf("应拒绝逃逸: %v", err)
	}
	if _, err := NewJail(m, "/home/../etc"); err == nil {
		t.Error("根目录含 .. 应拒绝")
	}
	if _, err := NewJail(m, "relative"); err == nil {
		t.Error("相对根目录应拒绝")
	}
}

func TestRemoveAllDoesNotFollowSymlink(t *testing.T) {
	m := sampleFS()
	m.mkdir("/home/u/tmp")
	m.symlink("/home/u/tmp/etc", "/etc")
	m.file("/home/u/tmp/f", "x")
	if err := m.RemoveAll("/home/u/tmp"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Lstat("/home/u/tmp"); err == nil {
		t.Error("目录应被删除")
	}
	if _, err := m.Stat("/etc/passwd"); err != nil {
		t.Error("不得跟随符号链接删除根外文件")
	}
}

func TestMkdirAllGeneric(t *testing.T) {
	m := sampleFS()
	if err := m.MkdirAll("/home/u/x/y/z"); err != nil {
		t.Fatal(err)
	}
	if fi, err := m.Stat("/home/u/x/y/z"); err != nil || !fi.IsDir() {
		t.Fatal("MkdirAll 未建出目录")
	}
	if err := m.MkdirAll("/home/u/docs/a.txt/q"); err == nil {
		t.Error("文件路径上建目录应失败")
	}
}

func readZip(t *testing.T, b []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(data)
	}
	return out
}

func TestWriteZipStream(t *testing.T) {
	m := sampleFS()
	var buf bytes.Buffer
	n, err := WriteZip(&buf, m, "/home/u/docs", "docs", ZipLimits{MaxFiles: 10, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len("hello")+len("world!")) {
		t.Errorf("字节数错误: %d", n)
	}
	files := readZip(t, buf.Bytes())
	var names []string
	for k := range files {
		names = append(names, k)
	}
	sort.Strings(names)
	want := []string{"docs/", "docs/a.txt", "docs/sub/", "docs/sub/b.txt"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("zip 条目=%v want %v（符号链接不得被跟随打包）", names, want)
	}
	if files["docs/a.txt"] != "hello" || files["docs/sub/b.txt"] != "world!" {
		t.Error("zip 内容错误")
	}
}

func TestZipLimits(t *testing.T) {
	m := sampleFS()
	if _, err := ScanTree(m, "/home/u/docs", ZipLimits{MaxFiles: 1}); !errors.Is(err, ErrTooMany) {
		t.Errorf("ScanTree 应超出文件数: %v", err)
	}
	if _, err := ScanTree(m, "/home/u/docs", ZipLimits{MaxBytes: 6}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("ScanTree 应超出大小: %v", err)
	}
	st, err := ScanTree(m, "/home/u/docs", ZipLimits{})
	if err != nil || st.Files != 2 || st.Bytes != 11 || st.Dirs != 1 {
		t.Errorf("ScanTree 统计错误: %+v %v", st, err)
	}
	var buf bytes.Buffer
	if _, err := WriteZip(&buf, m, "/home/u/docs", "docs", ZipLimits{MaxBytes: 7}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("WriteZip 流式过程中应检测超限: %v", err)
	}
	buf.Reset()
	if _, err := WriteZip(&buf, m, "/home/u/docs", "docs", ZipLimits{MaxFiles: 1}); !errors.Is(err, ErrTooMany) {
		t.Errorf("WriteZip 应检测文件数超限: %v", err)
	}
}

func TestCreateResume(t *testing.T) {
	m := sampleFS()
	w, _ := m.Create("/home/u/f.bin", 0)
	w.Write([]byte("abc"))
	w.Close()
	w, err := m.Create("/home/u/f.bin", 3)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("def"))
	w.Close()
	f, _ := m.Open("/home/u/f.bin")
	b, _ := io.ReadAll(f)
	if string(b) != "abcdef" {
		t.Errorf("续传结果错误: %q", b)
	}
}
