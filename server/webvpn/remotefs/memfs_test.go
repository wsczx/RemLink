package remotefs

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"
	"time"
)

// 内存 RemoteFS（含符号链接语义），仅用于测试
type memNode struct {
	dir  bool
	data []byte
	link string
	mod  time.Time
}

type memFS struct {
	mu    sync.Mutex
	nodes map[string]*memNode
}

func newMemFS() *memFS {
	return &memFS{nodes: map[string]*memNode{"/": {dir: true}}}
}

func (m *memFS) mkdir(p string) { m.nodes[path.Clean(p)] = &memNode{dir: true, mod: time.Now()} }
func (m *memFS) file(p, data string) {
	m.nodes[path.Clean(p)] = &memNode{data: []byte(data), mod: time.Now()}
}
func (m *memFS) symlink(p, target string) { m.nodes[path.Clean(p)] = &memNode{link: target} }

type memInfo struct {
	name string
	n    *memNode
}

func (i memInfo) Name() string { return i.name }
func (i memInfo) Size() int64  { return int64(len(i.n.data)) }
func (i memInfo) Mode() fs.FileMode {
	switch {
	case i.n.link != "":
		return fs.ModeSymlink | 0777
	case i.n.dir:
		return fs.ModeDir | 0755
	}
	return 0644
}
func (i memInfo) ModTime() time.Time { return i.n.mod }
func (i memInfo) IsDir() bool        { return i.n.dir }
func (i memInfo) Sys() any           { return nil }

// 解析路径中的全部符号链接
func (m *memFS) resolve(p string, depth int) (string, error) {
	if depth > 16 {
		return "", errors.New("too many links")
	}
	p = path.Clean(p)
	cur := "/"
	for _, seg := range strings.Split(strings.TrimPrefix(p, "/"), "/") {
		if seg == "" {
			continue
		}
		next := path.Join(cur, seg)
		n := m.nodes[next]
		if n == nil {
			return "", fs.ErrNotExist
		}
		if n.link != "" {
			t := n.link
			if !strings.HasPrefix(t, "/") {
				t = path.Join(cur, t)
			}
			r, err := m.resolve(t, depth+1)
			if err != nil {
				return "", err
			}
			cur = r
			continue
		}
		cur = next
	}
	return cur, nil
}

// 解析父目录，返回 (父目录真实路径 + base)，不跟随最后一级
func (m *memFS) lpath(p string) (string, error) {
	p = path.Clean(p)
	if p == "/" {
		return "/", nil
	}
	parent, err := m.resolve(path.Dir(p), 0)
	if err != nil {
		return "", err
	}
	return path.Join(parent, path.Base(p)), nil
}

func (m *memFS) List(p string) ([]fs.FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rp, err := m.resolve(p, 0)
	if err != nil {
		return nil, err
	}
	if !m.nodes[rp].dir {
		return nil, errors.New("not a dir")
	}
	var out []fs.FileInfo
	for k, n := range m.nodes {
		if k != "/" && path.Dir(k) == rp {
			out = append(out, memInfo{path.Base(k), n})
		}
	}
	return out, nil
}

func (m *memFS) Stat(p string) (fs.FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rp, err := m.resolve(p, 0)
	if err != nil {
		return nil, err
	}
	return memInfo{path.Base(rp), m.nodes[rp]}, nil
}

func (m *memFS) Lstat(p string) (fs.FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lp, err := m.lpath(p)
	if err != nil {
		return nil, err
	}
	n := m.nodes[lp]
	if n == nil {
		return nil, fs.ErrNotExist
	}
	return memInfo{path.Base(lp), n}, nil
}

func (m *memFS) ReadLink(p string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lp, err := m.lpath(p)
	if err != nil {
		return "", err
	}
	n := m.nodes[lp]
	if n == nil {
		return "", fs.ErrNotExist
	}
	if n.link == "" {
		return "", errors.New("not a link")
	}
	return n.link, nil
}

type memFile struct {
	*bytes.Reader
	info memInfo
}

func (f *memFile) Close() error               { return nil }
func (f *memFile) Stat() (fs.FileInfo, error) { return f.info, nil }

func (m *memFS) Open(p string) (File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rp, err := m.resolve(p, 0)
	if err != nil {
		return nil, err
	}
	n := m.nodes[rp]
	if n.dir {
		return nil, errors.New("is a dir")
	}
	return &memFile{bytes.NewReader(n.data), memInfo{path.Base(rp), n}}, nil
}

type memWriter struct {
	m   *memFS
	key string
	buf []byte
}

func (w *memWriter) Write(b []byte) (int, error) { w.buf = append(w.buf, b...); return len(b), nil }
func (w *memWriter) Close() error {
	w.m.mu.Lock()
	defer w.m.mu.Unlock()
	w.m.nodes[w.key] = &memNode{data: w.buf, mod: time.Now()}
	return nil
}

// 与真实文件系统一致：打开时跟随最后一级符号链接（即使悬空）
func (m *memFS) Create(p string, offset int64) (io.WriteCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lp, err := m.lpath(p)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 16; i++ {
		n := m.nodes[lp]
		if n == nil || n.link == "" {
			break
		}
		t := n.link
		if !strings.HasPrefix(t, "/") {
			t = path.Join(path.Dir(lp), t)
		}
		if lp, err = m.lpath(t); err != nil {
			return nil, err
		}
	}
	var buf []byte
	if offset > 0 {
		n := m.nodes[lp]
		if n == nil {
			return nil, fs.ErrNotExist
		}
		buf = append([]byte{}, n.data[:offset]...)
	}
	return &memWriter{m: m, key: lp, buf: buf}, nil
}

func (m *memFS) Mkdir(p string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	lp, err := m.lpath(p)
	if err != nil {
		return err
	}
	if m.nodes[lp] != nil {
		return fs.ErrExist
	}
	m.nodes[lp] = &memNode{dir: true, mod: time.Now()}
	return nil
}

func (m *memFS) MkdirAll(p string) error { return MkdirAllGeneric(m, p) }

func (m *memFS) Remove(p string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	lp, err := m.lpath(p)
	if err != nil {
		return err
	}
	n := m.nodes[lp]
	if n == nil {
		return fs.ErrNotExist
	}
	if n.dir {
		for k := range m.nodes {
			if k != "/" && path.Dir(k) == lp {
				return errors.New("dir not empty")
			}
		}
	}
	delete(m.nodes, lp)
	return nil
}

func (m *memFS) RemoveAll(p string) error { return RemoveAllGeneric(m, p) }

func (m *memFS) Rename(from, to string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	lf, err := m.lpath(from)
	if err != nil {
		return err
	}
	lt, err := m.lpath(to)
	if err != nil {
		return err
	}
	if m.nodes[lf] == nil {
		return fs.ErrNotExist
	}
	if m.nodes[lt] != nil {
		return fs.ErrExist
	}
	moved := map[string]*memNode{}
	for k, n := range m.nodes {
		if k == lf || strings.HasPrefix(k, lf+"/") {
			moved[lt+strings.TrimPrefix(k, lf)] = n
			delete(m.nodes, k)
		}
	}
	for k, n := range moved {
		m.nodes[k] = n
	}
	return nil
}

func (m *memFS) Close() error { return nil }
