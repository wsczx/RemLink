package remotefs

import (
	"context"
	"io/fs"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMBPath(t *testing.T) {
	good := map[string]string{
		"/":                 "",
		"/a":                "a",
		"/a/b/c.txt":        `a\b\c.txt`,
		"//a///b/":          `a\b`,
		"/中文/文件 名.docx":     `中文\文件 名.docx`,
		"/Program Files$/x": `Program Files$\x`,
	}
	for in, want := range good {
		got, err := SMBPath(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	bad := []string{
		"",            // 非绝对路径
		"a/b",         // 非绝对路径
		"/a/../b",     // ..
		"/a/./b",      // .
		`/a\..\b`,     // 反斜杠在 SMB 侧是分隔符，可绕过 Jail 的 "/" 语义
		`/a\b`,        //
		"/file:ads",   // NTFS 备用数据流
		"/c:/windows", // 盘符
		"/a\x01b",     // 控制字符
	}
	for _, in := range bad {
		_, err := SMBPath(in)
		assert.ErrorIs(t, err, ErrBadPath, "%q 应拒绝", in)
	}
}

func TestSplitSMBUser(t *testing.T) {
	cases := []struct{ user, domain, wantUser, wantDomain string }{
		{`CORP\alice`, "", "alice", "CORP"},
		{`CORP\alice`, "OTHER", `CORP\alice`, "OTHER"}, // 已配置域时不拆分
		{"alice@corp.local", "", "alice@corp.local", ""},
		{"alice", "", "alice", ""},
		{`\alice`, "", `\alice`, ""},
		{`CORP\`, "", `CORP\`, ""},
	}
	for _, c := range cases {
		u, d := SplitSMBUser(c.user, c.domain)
		assert.Equal(t, c.wantUser, u, c.user)
		assert.Equal(t, c.wantDomain, d, c.user)
	}
}

// 模拟 SMB 的「不跟随重解析点」语义：Stat==Lstat，ReadLink 不支持
type noFollowFS struct{ *memFS }

func (n noFollowFS) Stat(p string) (fs.FileInfo, error) { return n.memFS.Lstat(p) }
func (n noFollowFS) ReadLink(p string) (string, error) {
	return "", &fs.PathError{Op: "readlink", Path: p, Err: ErrUnsupported}
}

func TestNoFollowReparsePointsInJail(t *testing.T) {
	m := newMemFS()
	m.mkdir("/share")
	m.mkdir("/share/docs")
	m.file("/share/docs/a.txt", "a")
	m.mkdir("/other")
	m.file("/other/secret", "s")
	m.symlink("/share/junction", "/other")        // 目录联接
	m.symlink("/share/docs/inner", "/share/docs") // 根内链接也不跟随
	r := noFollowFS{m}
	j, err := NewJail(r, "/share")
	require.NoError(t, err)

	_, err = j.Resolve("/docs/a.txt")
	require.NoError(t, err)
	for _, p := range []string{"/junction/secret", "/junction", "/docs/inner/a.txt"} {
		_, err := j.Resolve(p)
		assert.ErrorIs(t, err, ErrEscape, p)
	}
	// 列目录时链接显示为符号链接，打包时跳过
	st, err := ScanTree(r, "/share", ZipLimits{})
	require.NoError(t, err)
	assert.Equal(t, 1, st.Files)
}

func TestDialSMBUnreachable(t *testing.T) {
	// 未监听端口：应快速失败且不是认证错误
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	ln.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = DialSMB(ctx, SMBConfig{Addr: addr, User: "u", Password: "p", Share: "s", Timeout: 2 * time.Second})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrAuthFailed)
}
