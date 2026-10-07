// Package remotefs 抽象 WebVPN 文件访问后端（SFTP，后续 SMB/FTP 复用），
// 并提供根目录限定（Jail）、目录 zip 流式打包等与具体协议无关的能力。
package remotefs

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// 可读、可定位的远程文件（支持 HTTP Range）
type File interface {
	io.Reader
	io.Seeker
	io.Closer
	Stat() (fs.FileInfo, error)
}

// 远程文件系统。所有路径均为目标端绝对路径（"/" 分隔），调用方负责经 Jail 校验
type RemoteFS interface {
	// 列目录，返回条目不跟随符号链接（lstat 语义）
	List(p string) ([]fs.FileInfo, error)
	Stat(p string) (fs.FileInfo, error)
	Lstat(p string) (fs.FileInfo, error)
	// 读取符号链接目标（Jail 据此自行解析链接，不依赖服务端 realpath）
	ReadLink(p string) (string, error)
	Open(p string) (File, error)
	// offset==0 新建/截断写；offset>0 打开已有文件并从 offset 续写
	Create(p string, offset int64) (io.WriteCloser, error)
	Mkdir(p string) error
	MkdirAll(p string) error
	// 删除文件或空目录（不跟随符号链接）
	Remove(p string) error
	// 递归删除（不跟随符号链接）
	RemoveAll(p string) error
	Rename(from, to string) error
	Close() error
}

var (
	ErrEscape      = errors.New("路径超出允许的根目录")
	ErrBadPath     = errors.New("非法路径")
	ErrUnsupported = errors.New("操作不受支持")
	ErrTooLarge    = errors.New("超出大小限制")
	ErrTooMany     = errors.New("文件数量超出限制")
)

// 通用递归删除：符号链接只删除链接本身
func RemoveAllGeneric(r RemoteFS, p string) error {
	fi, err := r.Lstat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0 {
		entries, err := r.List(p)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !validEntryName(e.Name()) {
				continue
			}
			if err := RemoveAllGeneric(r, path.Join(p, e.Name())); err != nil {
				return err
			}
		}
	}
	return r.Remove(p)
}

// 通用逐级建目录
func MkdirAllGeneric(r RemoteFS, p string) error {
	p = path.Clean(p)
	if p == "/" || p == "." {
		return nil
	}
	if fi, err := r.Stat(p); err == nil {
		if fi.IsDir() {
			return nil
		}
		return errors.New("路径已存在且不是目录: " + p)
	}
	if err := MkdirAllGeneric(r, path.Dir(p)); err != nil {
		return err
	}
	if err := r.Mkdir(p); err != nil {
		if fi, e2 := r.Stat(p); e2 == nil && fi.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

// 远程返回的目录项名不得含分隔符或为 . / ..（防恶意服务器构造穿越）
func validEntryName(n string) bool {
	return n != "" && n != "." && n != ".." && !strings.ContainsAny(n, "/\x00")
}

// 有序列目录（目录在前，再按名称）
func SortedList(r RemoteFS, p string) ([]fs.FileInfo, error) {
	list, err := r.List(p)
	if err != nil {
		return nil, err
	}
	out := list[:0]
	for _, e := range list {
		if validEntryName(e.Name()) {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := out[i].IsDir(), out[j].IsDir()
		if di != dj {
			return di
		}
		return out[i].Name() < out[j].Name()
	})
	return out, nil
}
