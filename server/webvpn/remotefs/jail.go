package remotefs

import (
	"errors"
	"io/fs"
	"path"
	"strings"
)

// 根目录限定：所有用户提供的相对路径都必须经 Jail 解析
//
//  1. 词法层：拒绝 NUL、拒绝任何 ".." 段，path.Clean 后拼接到 Root
//  2. 符号链接层：网关自行逐级 Lstat + ReadLink 解析符号链接（不依赖服务端 realpath 语义，
//     部分 SFTP 实现的 realpath 只做词法规整），解析结果必须仍位于 RealRoot 之内；
//     悬空链接按其指向解析，指向根外同样拒绝（防止经悬空链接在根外创建文件）
//
// 注：解析与后续操作之间存在 TOCTOU 窗口（目标端用户可并发替换为符号链接），
// 该残余风险受目标账号自身权限约束。
type Jail struct {
	FS       RemoteFS
	Root     string // 词法根（已 Clean 的绝对路径）
	RealRoot string // 解析符号链接后的根
}

const maxLinkHops = 40

// 创建 Jail；root 为空时取 "/"
func NewJail(r RemoteFS, root string) (*Jail, error) {
	if root == "" {
		root = "/"
	}
	if !strings.HasPrefix(root, "/") || hasDotDot(root) || strings.ContainsRune(root, 0) {
		return nil, ErrBadPath
	}
	root = path.Clean(root)
	fi, err := r.Stat(root)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errors.New("根目录不是目录: " + root)
	}
	real, err := RealPathOf(r, root)
	if err != nil {
		return nil, err
	}
	return &Jail{FS: r, Root: root, RealRoot: real}, nil
}

func hasDotDot(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// 规范化用户相对路径：返回以 "/" 开头、相对 Root 的干净路径
func CleanRel(rel string) (string, error) {
	if strings.ContainsRune(rel, 0) || hasDotDot(rel) {
		return "", ErrBadPath
	}
	return path.Clean("/" + rel), nil
}

func within(p, root string) bool {
	if root == "/" {
		return strings.HasPrefix(p, "/")
	}
	return p == root || strings.HasPrefix(p, root+"/")
}

// 逐级解析绝对路径中的全部符号链接（含最后一级）；不存在的尾部按词法保留
func RealPathOf(r RemoteFS, p string) (string, error) {
	hops := 0
	var resolve func(p string) (string, error)
	resolve = func(p string) (string, error) {
		p = path.Clean(p)
		segs := strings.Split(strings.TrimPrefix(p, "/"), "/")
		cur := "/"
		for i, seg := range segs {
			if seg == "" || seg == "." {
				continue
			}
			if seg == ".." {
				cur = path.Dir(cur)
				continue
			}
			next := path.Join(cur, seg)
			fi, err := r.Lstat(next)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					// 不存在：其后各级不可能是链接，词法拼接即可
					return path.Join(append([]string{next}, segs[i+1:]...)...), nil
				}
				return "", err
			}
			if fi.Mode()&fs.ModeSymlink == 0 {
				cur = next
				continue
			}
			hops++
			if hops > maxLinkHops {
				return "", ErrEscape
			}
			target, err := r.ReadLink(next)
			if err != nil {
				return "", err
			}
			if !strings.HasPrefix(target, "/") {
				target = path.Join(cur, target)
			}
			if cur, err = resolve(target); err != nil {
				return "", err
			}
		}
		return cur, nil
	}
	return resolve(p)
}

// 仅词法解析（不访问远端）
func (j *Jail) Abs(rel string) (string, error) {
	c, err := CleanRel(rel)
	if err != nil {
		return "", err
	}
	abs := path.Join(j.Root, c)
	if !within(abs, j.Root) {
		return "", ErrEscape
	}
	return abs, nil
}

// 将相对路径转为远端绝对路径并校验符号链接逃逸（目标可以不存在）
func (j *Jail) Resolve(rel string) (string, error) {
	abs, err := j.Abs(rel)
	if err != nil {
		return "", err
	}
	real, err := RealPathOf(j.FS, abs)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return "", err
		}
		return "", ErrEscape
	}
	if !within(real, j.RealRoot) {
		return "", ErrEscape
	}
	return abs, nil
}

// 解析并要求目标存在（返回 lstat 信息）
func (j *Jail) ResolveExisting(rel string) (string, fs.FileInfo, error) {
	abs, err := j.Resolve(rel)
	if err != nil {
		return "", nil, err
	}
	fi, err := j.FS.Lstat(abs)
	if err != nil {
		return "", nil, err
	}
	return abs, fi, nil
}

// 绝对路径转回相对 Root 的路径（"/" 开头）
func (j *Jail) Rel(abs string) string {
	if j.Root == "/" {
		return abs
	}
	r := strings.TrimPrefix(abs, j.Root)
	if r == "" {
		return "/"
	}
	return r
}
