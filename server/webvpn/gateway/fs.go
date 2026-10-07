package gateway

import (
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn/remotefs"
)

const (
	zipMaxFiles       = 20000
	zipDefaultMaxSize = 10 << 30 // 未配置 max_file_size 时目录打包的原始总大小上限
	maxRelPathLen     = 4096
)

// 文件条目
type fileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"` // 相对根目录，"/" 开头
	Size    int64  `json:"size"`
	Mode    string `json:"mode"` // 如 drwxr-xr-x
	ModTime int64  `json:"mod_time"`
	IsDir   bool   `json:"is_dir"`
	IsLink  bool   `json:"is_link"`
}

func toEntry(dir string, fi fs.FileInfo) fileEntry {
	return fileEntry{
		Name:    fi.Name(),
		Path:    path.Join(dir, fi.Name()),
		Size:    fi.Size(),
		Mode:    fi.Mode().String(),
		ModTime: fi.ModTime().Unix(),
		IsDir:   fi.IsDir(),
		IsLink:  fi.Mode()&fs.ModeSymlink != 0,
	}
}

// 文件 API 公共前置：鉴权 + 会话归属 + 文件后端就绪
type fsCtx struct {
	*reqCtx
	s    *rsession
	jail *remotefs.Jail
}

func (g *Gateway) fsGuard(w http.ResponseWriter, r *http.Request, strictOrigin bool) (*fsCtx, bool) {
	c, ok := g.guard(w, r, strictOrigin)
	if !ok {
		return nil, false
	}
	s, ok := g.ownedSession(w, c, mux.Vars(r)["sid"])
	if !ok {
		return nil, false
	}
	j, err := s.fs()
	if err != nil {
		if errors.Is(err, errNoFileAccess) {
			writeErr(w, http.StatusBadRequest, "unsupported_protocol", errNoFileAccess.Error())
			return nil, false
		}
		base.Warn("WebVPN 文件服务初始化失败:", s.ls.AppName, s.ls.Protocol, err)
		writeErr(w, http.StatusBadGateway, fsInitErrCode(s.ls.Protocol), "文件服务不可用")
		return nil, false
	}
	s.ls.Touch()
	return &fsCtx{reqCtx: c, s: s, jail: j}, true
}

// 记录文件操作日志（异步）
func (f *fsCtx) log(op, p string, size int64, err error) {
	res := "ok"
	if err != nil {
		res = err.Error()
		if len(res) > 250 {
			res = res[:250]
		}
	}
	f.s.g.files.log(dbdata.WebVpnFileLog{
		Sid: f.s.ls.Sid, Username: f.s.ls.Username, AppName: f.s.ls.AppName,
		Op: op, Path: p, Size: size, Result: res,
	})
}

// 将内部错误映射为对用户安全的响应
func fsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, remotefs.ErrEscape), errors.Is(err, remotefs.ErrBadPath):
		writeErr(w, http.StatusForbidden, "path_denied", "路径不允许访问")
	case errors.Is(err, fs.ErrNotExist):
		writeErr(w, http.StatusNotFound, "not_found", "文件或目录不存在")
	case errors.Is(err, fs.ErrPermission):
		writeErr(w, http.StatusForbidden, "permission_denied", "目标主机拒绝访问")
	case errors.Is(err, fs.ErrExist):
		writeErr(w, http.StatusConflict, "exists", "目标已存在")
	case errors.Is(err, remotefs.ErrTooLarge):
		writeErr(w, http.StatusRequestEntityTooLarge, "too_large", "超出大小限制")
	case errors.Is(err, remotefs.ErrTooMany):
		writeErr(w, http.StatusRequestEntityTooLarge, "too_many_files", "文件数量超出限制")
	default:
		writeErr(w, http.StatusBadGateway, "remote_error", "操作失败: "+err.Error())
	}
}

func noDeadline(w http.ResponseWriter, read bool) {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	if read {
		_ = rc.SetReadDeadline(time.Time{})
	}
}

// GET /webvpn/fs/{sid}/list?path=
func (g *Gateway) handleList(w http.ResponseWriter, r *http.Request) {
	f, ok := g.fsGuard(w, r, false)
	if !ok {
		return
	}
	rel := r.URL.Query().Get("path")
	abs, err := f.jail.Resolve(rel)
	if err == nil {
		var list []fs.FileInfo
		if list, err = remotefs.SortedList(f.jail.FS, abs); err == nil {
			dir := f.jail.Rel(abs)
			out := make([]fileEntry, 0, len(list))
			for _, fi := range list {
				out = append(out, toEntry(dir, fi))
			}
			f.log("list", dir, 0, nil)
			writeOK(w, map[string]any{"path": dir, "entries": out})
			return
		}
	}
	f.log("list", rel, 0, err)
	fsError(w, err)
}

// GET /webvpn/fs/{sid}/stat?path=：查询单个文件（断点续传前获取已上传大小）
func (g *Gateway) handleStat(w http.ResponseWriter, r *http.Request) {
	f, ok := g.fsGuard(w, r, false)
	if !ok {
		return
	}
	rel := r.URL.Query().Get("path")
	abs, fi, err := f.jail.ResolveExisting(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeOK(w, map[string]any{"exists": false})
			return
		}
		fsError(w, err)
		return
	}
	e := toEntry(path.Dir(f.jail.Rel(abs)), fi)
	writeOK(w, map[string]any{"exists": true, "entry": e})
}

func contentDisposition(name string) string {
	return mime.FormatMediaType("attachment", map[string]string{"filename": name})
}

// GET /webvpn/fs/{sid}/download?path=：文件直接流（支持 Range），目录流式 zip
func (g *Gateway) handleDownload(w http.ResponseWriter, r *http.Request) {
	f, ok := g.fsGuard(w, r, false)
	if !ok {
		return
	}
	rel := r.URL.Query().Get("path")
	if !f.s.app.AllowDownload {
		f.log("download", rel, 0, errors.New("forbidden"))
		writeErr(w, http.StatusForbidden, "download_denied", "管理员未允许下载")
		return
	}
	abs, err := f.jail.Resolve(rel)
	if err != nil {
		f.log("download", rel, 0, err)
		fsError(w, err)
		return
	}
	fi, err := f.jail.FS.Stat(abs) // Resolve 已确认解析后的目标仍在根内
	if err != nil {
		f.log("download", rel, 0, err)
		fsError(w, err)
		return
	}
	relPath := f.jail.Rel(abs)
	name := path.Base(abs)
	if relPath == "/" {
		name = f.s.app.Name
	}
	noDeadline(w, false)
	if fi.IsDir() {
		lim := remotefs.ZipLimits{MaxFiles: zipMaxFiles, MaxBytes: zipDefaultMaxSize}
		if f.s.app.MaxFileSize > 0 {
			lim.MaxBytes = f.s.app.MaxFileSize
		}
		if _, err := remotefs.ScanTree(f.jail.FS, abs, lim); err != nil {
			f.log("download_dir", relPath, 0, err)
			fsError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", contentDisposition(name+".zip"))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		cw := &countWriter{w: w}
		n, err := remotefs.WriteZip(cw, f.jail.FS, abs, name, lim)
		f.s.ls.AddOut(cw.n)
		f.log("download_dir", relPath, n, err)
		if err != nil {
			// 响应头已发出，只能中断连接让浏览器判定下载失败
			panic(http.ErrAbortHandler)
		}
		return
	}
	if !fi.Mode().IsRegular() {
		f.log("download", relPath, 0, errors.New("not a regular file"))
		writeErr(w, http.StatusBadRequest, "not_regular", "仅支持下载普通文件或目录")
		return
	}
	if f.s.app.MaxFileSize > 0 && fi.Size() > f.s.app.MaxFileSize {
		f.log("download", relPath, fi.Size(), remotefs.ErrTooLarge)
		fsError(w, remotefs.ErrTooLarge)
		return
	}
	file, err := f.jail.FS.Open(abs)
	if err != nil {
		f.log("download", relPath, 0, err)
		fsError(w, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", contentDisposition(name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	cw := &countRW{ResponseWriter: w}
	http.ServeContent(cw, r, name, fi.ModTime(), file)
	f.s.ls.AddOut(cw.n)
	f.log("download", relPath, cw.n, nil)
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

type countRW struct {
	http.ResponseWriter
	n int64
}

func (c *countRW) Write(p []byte) (int, error) {
	n, err := c.ResponseWriter.Write(p)
	c.n += int64(n)
	return n, err
}

func (c *countRW) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// 上传结果
type uploadResult struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`   // 本次写入字节数
	Offset int64  `json:"offset"` // 起始偏移
	Total  int64  `json:"total"`  // 写入后文件大小
	Error  string `json:"error,omitempty"`
	Code   string `json:"code,omitempty"`
}

// POST /webvpn/fs/{sid}/upload?dir=/&overwrite=0
// multipart/form-data，按顺序：relpath（可选，相对 dir 的路径，可含子目录）、offset（可选，续传偏移）、
// size（可选，整个文件的大小：分片上传时只在最后一片写完后记一条文件日志）、file（文件内容）；
// 可重复多组以一次上传多个文件。relpath 含目录时自动 MkdirAll（支持文件夹上传）
func (g *Gateway) handleUpload(w http.ResponseWriter, r *http.Request) {
	f, ok := g.fsGuard(w, r, true)
	if !ok {
		return
	}
	if !f.s.app.AllowUpload {
		f.log("upload", r.URL.Query().Get("dir"), 0, errors.New("forbidden"))
		writeErr(w, http.StatusForbidden, "upload_denied", "管理员未允许上传")
		return
	}
	baseDir, err := remotefs.CleanRel(r.URL.Query().Get("dir"))
	if err != nil {
		fsError(w, err)
		return
	}
	overwrite := r.URL.Query().Get("overwrite") == "1" || r.URL.Query().Get("overwrite") == "true"
	noDeadline(w, true)
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "需要 multipart/form-data")
		return
	}
	var results []uploadResult
	relpath, offset, size := "", int64(0), int64(0)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", "上传数据格式错误")
			return
		}
		switch part.FormName() {
		case "relpath":
			b, _ := io.ReadAll(io.LimitReader(part, maxRelPathLen+1))
			if len(b) > maxRelPathLen {
				writeErr(w, http.StatusBadRequest, "bad_request", "路径过长")
				return
			}
			relpath = string(b)
		case "offset":
			b, _ := io.ReadAll(io.LimitReader(part, 32))
			offset, _ = strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
			if offset < 0 {
				offset = 0
			}
		case "size":
			b, _ := io.ReadAll(io.LimitReader(part, 32))
			size, _ = strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		case "file":
			rp := relpath
			if rp == "" {
				rp = part.FileName()
			}
			res := f.uploadOne(part, baseDir, rp, offset, size, overwrite)
			results = append(results, res)
			relpath, offset, size = "", 0, 0
		}
		part.Close()
	}
	if len(results) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "未包含文件")
		return
	}
	// 单文件失败时返回对应状态码，便于前端处理冲突/续传
	if len(results) == 1 && results[0].Code != "" {
		writeErrData(w, uploadStatus(results[0].Code), results[0].Code, results[0].Error, results[0])
		return
	}
	writeOK(w, map[string]any{"files": results})
}

func uploadStatus(code string) int {
	switch code {
	case "exists", "offset_mismatch":
		return http.StatusConflict
	case "too_large":
		return http.StatusRequestEntityTooLarge
	case "path_denied":
		return http.StatusForbidden
	case "bad_path":
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

// size 为整个文件大小（未知传 0）：分片上传只在最后一片写完时记录一条日志，大小记为文件总大小
func (f *fsCtx) uploadOne(src io.Reader, baseDir, relpath string, offset, size int64, overwrite bool) uploadResult {
	res := uploadResult{Offset: offset}
	failed := func(code string, err error) uploadResult {
		res.Code = code
		res.Error = err.Error()
		f.log("upload", res.Path, res.Size, err)
		return res
	}
	if relpath == "" || strings.HasSuffix(relpath, "/") {
		return failed("bad_path", remotefs.ErrBadPath)
	}
	rel, err := remotefs.CleanRel(path.Join(baseDir, relpath))
	if err != nil || rel == "/" {
		res.Path = relpath
		return failed("bad_path", remotefs.ErrBadPath)
	}
	res.Path = rel
	abs, err := f.jail.Resolve(rel)
	if err != nil {
		return failed("path_denied", err)
	}
	// 文件夹上传：自动创建父目录；创建后再次校验，防止中途被替换为根外链接
	if err := f.jail.FS.MkdirAll(path.Dir(abs)); err != nil {
		return failed("remote_error", err)
	}
	if abs, err = f.jail.Resolve(rel); err != nil {
		return failed("path_denied", err)
	}
	max := f.s.app.MaxFileSize
	if max > 0 && offset > max {
		return failed("too_large", remotefs.ErrTooLarge)
	}
	fi, statErr := f.jail.FS.Stat(abs)
	exists := statErr == nil
	if exists && fi.IsDir() {
		return failed("exists", errors.New("同名目录已存在"))
	}
	if offset > 0 {
		if !exists || fi.Size() != offset {
			cur := int64(0)
			if exists {
				cur = fi.Size()
			}
			res.Total = cur
			return failed("offset_mismatch", errors.New("续传偏移与已上传大小不一致，当前大小 "+strconv.FormatInt(cur, 10)))
		}
	} else if exists && !overwrite {
		res.Total = fi.Size()
		return failed("exists", errors.New("文件已存在"))
	}
	wc, err := f.jail.FS.Create(abs, offset)
	if err != nil {
		return failed("remote_error", err)
	}
	var rd io.Reader = src
	if max > 0 {
		rd = io.LimitReader(src, max-offset+1)
	}
	n, err := io.Copy(wc, rd)
	cerr := wc.Close()
	res.Size = n
	res.Total = offset + n
	f.s.ls.AddIn(n)
	if max > 0 && offset+n > max {
		if offset == 0 {
			_ = f.jail.FS.Remove(abs)
		}
		return failed("too_large", remotefs.ErrTooLarge)
	}
	if err == nil {
		err = cerr
	}
	if err != nil {
		// 保留已写入部分以便续传
		return failed("remote_error", err)
	}
	switch {
	case size <= 0:
		// 未提供总大小（旧客户端）：按片记录
		f.log("upload", rel, n, nil)
	case res.Total >= size:
		f.log("upload", rel, res.Total, nil)
	}
	return res
}

type pathReq struct {
	Path  string   `json:"path"`
	Paths []string `json:"paths"`
	From  string   `json:"from"`
	To    string   `json:"to"`
}

// POST /webvpn/fs/{sid}/mkdir {"path":"/a/b"}
func (g *Gateway) handleMkdir(w http.ResponseWriter, r *http.Request) {
	f, ok := g.fsGuard(w, r, true)
	if !ok {
		return
	}
	var req pathReq
	if !decodeJSON(w, r, &req, 16<<10) {
		return
	}
	if !f.s.app.AllowUpload {
		f.log("mkdir", req.Path, 0, errors.New("forbidden"))
		writeErr(w, http.StatusForbidden, "upload_denied", "管理员未允许写入")
		return
	}
	abs, err := f.jail.Resolve(req.Path)
	if err == nil {
		err = f.jail.FS.MkdirAll(abs)
	}
	rel := req.Path
	if err == nil {
		rel = f.jail.Rel(abs)
	}
	f.log("mkdir", rel, 0, err)
	if err != nil {
		fsError(w, err)
		return
	}
	writeOK(w, map[string]any{"path": rel})
}

// POST /webvpn/fs/{sid}/rename {"from":"/a","to":"/b"}
func (g *Gateway) handleRename(w http.ResponseWriter, r *http.Request) {
	f, ok := g.fsGuard(w, r, true)
	if !ok {
		return
	}
	var req pathReq
	if !decodeJSON(w, r, &req, 16<<10) {
		return
	}
	desc := req.From + " -> " + req.To
	if !f.s.app.AllowUpload {
		f.log("rename", desc, 0, errors.New("forbidden"))
		writeErr(w, http.StatusForbidden, "upload_denied", "管理员未允许写入")
		return
	}
	err := func() error {
		from, _, err := f.jail.ResolveExisting(req.From)
		if err != nil {
			return err
		}
		to, err := f.jail.Resolve(req.To)
		if err != nil {
			return err
		}
		if from == f.jail.Root || to == f.jail.Root {
			return remotefs.ErrBadPath
		}
		if _, err := f.jail.FS.Lstat(to); err == nil {
			return fs.ErrExist
		}
		desc = f.jail.Rel(from) + " -> " + f.jail.Rel(to)
		return f.jail.FS.Rename(from, to)
	}()
	f.log("rename", desc, 0, err)
	if err != nil {
		fsError(w, err)
		return
	}
	writeOK(w, nil)
}

// POST /webvpn/fs/{sid}/delete {"path":"/a"} 或 {"paths":["/a","/b"]}
func (g *Gateway) handleDelete(w http.ResponseWriter, r *http.Request) {
	f, ok := g.fsGuard(w, r, true)
	if !ok {
		return
	}
	var req pathReq
	if !decodeJSON(w, r, &req, 256<<10) {
		return
	}
	paths := req.Paths
	if req.Path != "" {
		paths = append(paths, req.Path)
	}
	if len(paths) == 0 || len(paths) > 1000 {
		writeErr(w, http.StatusBadRequest, "bad_request", "参数错误")
		return
	}
	if !f.s.app.AllowDelete {
		f.log("delete", strings.Join(paths, ","), 0, errors.New("forbidden"))
		writeErr(w, http.StatusForbidden, "delete_denied", "管理员未允许删除")
		return
	}
	for _, p := range paths {
		err := func() error {
			// 不跟随最后一级符号链接：Resolve 只校验父目录链，删除作用于链接本身
			abs, err := f.jail.Abs(p)
			if err != nil {
				return err
			}
			if abs == f.jail.Root {
				return remotefs.ErrBadPath
			}
			if _, err := f.jail.Resolve(path.Dir(f.jail.Rel(abs))); err != nil {
				return err
			}
			fi, err := f.jail.FS.Lstat(abs)
			if err != nil {
				return err
			}
			if fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0 {
				return f.jail.FS.RemoveAll(abs)
			}
			return f.jail.FS.Remove(abs)
		}()
		f.log("delete", p, 0, err)
		if err != nil {
			fsError(w, err)
			return
		}
	}
	writeOK(w, nil)
}

// ---------------- 文件日志异步写入 ----------------

type fileLogger struct {
	ch       chan dbdata.WebVpnFileLog
	quit     chan struct{}
	done     chan struct{}
	once     sync.Once
	stopOnce sync.Once
}

func newFileLogger() *fileLogger {
	return &fileLogger{ch: make(chan dbdata.WebVpnFileLog, 2000), quit: make(chan struct{}), done: make(chan struct{})}
}

func (l *fileLogger) start() {
	l.once.Do(func() {
		go func() {
			defer close(l.done)
			for {
				select {
				case rec := <-l.ch:
					l.write(rec)
				case <-l.quit:
					for {
						select {
						case rec := <-l.ch:
							l.write(rec)
						default:
							return
						}
					}
				}
			}
		}()
	})
}

func (l *fileLogger) write(rec dbdata.WebVpnFileLog) {
	if err := dbdata.AddWebVpnFileLog(&rec); err != nil {
		base.Warn("WebVPN 文件日志写入失败:", err)
	}
}

func (l *fileLogger) log(rec dbdata.WebVpnFileLog) {
	if len(rec.Path) > 1024 {
		rec.Path = rec.Path[:1024]
	}
	select {
	case l.ch <- rec:
	default:
		// 队列满时降级为本地日志，不阻塞文件传输
		base.Warn("WebVPN 文件日志队列已满:", rec.Username, rec.AppName, rec.Op, rec.Path, rec.Result)
	}
}

func (l *fileLogger) stop() {
	l.stopOnce.Do(func() {
		close(l.quit)
	})
}
