package remotefs

import (
	"archive/zip"
	"io"
	"io/fs"
	"path"
)

// 目录打包限制
type ZipLimits struct {
	MaxFiles int   // 最大文件数；<=0 不限
	MaxBytes int64 // 原始总字节数上限；<=0 不限
}

// 目录树统计结果
type TreeStat struct {
	Files int
	Dirs  int
	Bytes int64
}

// 预扫描目录树（不跟随符号链接，跳过非常规文件），超限立即返回错误
func ScanTree(r RemoteFS, dir string, lim ZipLimits) (TreeStat, error) {
	var st TreeStat
	var walk func(p string) error
	walk = func(p string) error {
		entries, err := r.List(p)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !validEntryName(e.Name()) {
				continue
			}
			m := e.Mode()
			switch {
			case m&fs.ModeSymlink != 0:
				continue
			case e.IsDir():
				st.Dirs++
				if err := walk(path.Join(p, e.Name())); err != nil {
					return err
				}
			case m.IsRegular():
				st.Files++
				st.Bytes += e.Size()
				if lim.MaxFiles > 0 && st.Files > lim.MaxFiles {
					return ErrTooMany
				}
				if lim.MaxBytes > 0 && st.Bytes > lim.MaxBytes {
					return ErrTooLarge
				}
			}
		}
		return nil
	}
	return st, walk(dir)
}

// 将 dir 以 zip 流式写入 w（不落盘），条目名以 prefix 为顶层目录
// 不跟随符号链接；写出过程中再次校验限制（文件在扫描后变大时中止）
// 返回写入的原始字节数
func WriteZip(w io.Writer, r RemoteFS, dir, prefix string, lim ZipLimits) (int64, error) {
	zw := zip.NewWriter(w)
	var total int64
	files := 0
	var walk func(p, name string) error
	walk = func(p, name string) error {
		entries, err := SortedList(r, p)
		if err != nil {
			return err
		}
		if name != "" {
			if _, err := zw.CreateHeader(&zip.FileHeader{Name: name + "/", Method: zip.Store}); err != nil {
				return err
			}
		}
		for _, e := range entries {
			child := path.Join(p, e.Name())
			cname := e.Name()
			if name != "" {
				cname = name + "/" + e.Name()
			}
			m := e.Mode()
			switch {
			case m&fs.ModeSymlink != 0:
				continue
			case e.IsDir():
				if err := walk(child, cname); err != nil {
					return err
				}
			case m.IsRegular():
				files++
				if lim.MaxFiles > 0 && files > lim.MaxFiles {
					return ErrTooMany
				}
				if err := func() error {
					// 打开前再 Lstat 一次，避免扫描后被替换为符号链接
					fi, err := r.Lstat(child)
					if err != nil || !fi.Mode().IsRegular() {
						return nil
					}
					f, err := r.Open(child)
					if err != nil {
						return nil // 无权限等单文件错误跳过，不中断整体打包
					}
					defer f.Close()
					hdr, herr := zip.FileInfoHeader(fi)
					if herr != nil {
						hdr = &zip.FileHeader{Modified: fi.ModTime()}
					}
					hdr.Name = cname
					hdr.Method = zip.Deflate
					zf, err := zw.CreateHeader(hdr)
					if err != nil {
						return err
					}
					var src io.Reader = f
					if lim.MaxBytes > 0 {
						src = io.LimitReader(f, lim.MaxBytes-total+1)
					}
					n, err := io.Copy(zf, src)
					total += n
					if lim.MaxBytes > 0 && total > lim.MaxBytes {
						return ErrTooLarge
					}
					return err
				}(); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(dir, prefix); err != nil {
		_ = zw.Close()
		return total, err
	}
	return total, zw.Close()
}
