package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/webvpn/live"
)

// WebVPN 远程会话（SSH/SFTP/…）管理接口

func webVpnPageArgs(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.FormValue("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.FormValue("page_size"))
	if pageSize <= 0 || pageSize > 500 {
		pageSize = dbdata.PageSize
	}
	return page, pageSize
}

func webVpnDateArgs(r *http.Request) []string {
	d := r.FormValue("start_date")
	if d == "" {
		return nil
	}
	end := r.FormValue("end_date")
	if end == "" {
		end = time.Now().Format("2006-01-02 15:04:05")
	}
	return []string{d, end}
}

// 在线会话列表（内存注册表实时数据）
// GET ?username=&app_name=
func WebVpnLiveSessionList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	datas := live.Default().List(r.FormValue("username"), r.FormValue("app_name"))
	RespSucess(w, map[string]any{
		"count": len(datas),
		"datas": datas,
	})
}

// 踢会话：按 sid 踢单个；或按 username（+可选 app_name）批量踢
// POST {"sid":"..."} | {"username":"...","app_name":"..."}
func WebVpnLiveSessionKick(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sid      string `json:"sid"`
		Username string `json:"username"`
		AppName  string `json:"app_name"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil || json.Unmarshal(body, &req) != nil || (req.Sid == "" && req.Username == "" && req.AppName == "") {
		RespError(w, RespParamErr, "参数错误")
		return
	}
	reg := live.Default()
	n := 0
	target := req.Sid
	switch {
	case req.Sid != "":
		if s := reg.Get(req.Sid); s != nil {
			target = s.Username + "@" + s.AppName + " " + req.Sid
		}
		if reg.Kick(req.Sid, live.ReasonKicked) {
			n = 1
		}
	case req.AppName != "":
		target = req.Username + "@" + req.AppName
		n = reg.KickApp(req.AppName, req.Username, live.ReasonKicked)
	default:
		target = req.Username
		n = reg.KickUser(req.Username, live.ReasonKicked)
	}
	dbdata.AdminLog("WebVPN应用管理", target, "断开了WebVPN远程会话 "+strconv.Itoa(n)+" 个", r.RemoteAddr)
	RespSucess(w, map[string]any{"kicked": n})
}

// 历史会话列表（数据库）
// GET ?page=&page_size=&username=&app_name=&start_date=&end_date=
func WebVpnSessionHistoryList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	page, pageSize := webVpnPageArgs(r)
	datas, count, err := dbdata.WebVpnSessionList(pageSize, page, dbdata.WebVpnSessionSearch{
		Username: r.FormValue("username"),
		AppName:  r.FormValue("app_name"),
		Date:     webVpnDateArgs(r),
	})
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}
	if datas == nil {
		datas = []dbdata.WebVpnSession{}
	}
	RespSucess(w, map[string]any{"count": count, "page_size": pageSize, "datas": datas})
}

// 文件操作日志
// GET ?page=&page_size=&sid=&username=&app_name=&op=&start_date=&end_date=
func WebVpnFileLogList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	page, pageSize := webVpnPageArgs(r)
	datas, count, err := dbdata.WebVpnFileLogList(pageSize, page, dbdata.WebVpnFileLogSearch{
		Sid:      r.FormValue("sid"),
		Username: r.FormValue("username"),
		AppName:  r.FormValue("app_name"),
		Op:       r.FormValue("op"),
		Date:     webVpnDateArgs(r),
	})
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}
	if datas == nil {
		datas = []dbdata.WebVpnFileLog{}
	}
	RespSucess(w, map[string]any{"count": count, "page_size": pageSize, "datas": datas})
}

// 会话录像下载
//   - SSH：asciicast v2（.cast），可用 asciinema-player 直接回放
//   - RDP/VNC：Guacamole 会话录像（.guac），可用 guacamole-common-js 的 Guacamole.SessionRecording 回放，
//     或在 guacd 镜像内用 guacenc 转码为视频：guacenc -s 1280x720 -r 2000000 <sid>.guac（生成 .m4v）
//
// GET ?sid=[&part=N]  part：同一桌面会话第 N 次附着产生的录像（<sid>.N.guac，N>=2）
func WebVpnRecordingDownload(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	sid := r.FormValue("sid")
	if sid == "" {
		RespError(w, RespParamErr, "sid 不能为空")
		return
	}
	s, err := dbdata.GetWebVpnSession(sid)
	if err != nil || s.RecordingPath == "" {
		RespError(w, RespParamErr, "该会话没有录像")
		return
	}
	rel := s.RecordingPath
	ext := path.Ext(rel)
	if part := r.FormValue("part"); part != "" && ext == ".guac" {
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 || n > 10000 {
			RespError(w, RespParamErr, "part 参数错误")
			return
		}
		if n > 1 {
			rel = strings.TrimSuffix(rel, ".guac") + "." + strconv.Itoa(n) + ".guac"
		}
	}
	full, err := dbdata.WebVpnRecordFullPath(rel)
	if err != nil {
		RespError(w, RespParamErr, err)
		return
	}
	f, err := os.Open(full)
	if err != nil {
		RespError(w, RespInternalErr, "录像文件不存在或已被清理")
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		RespError(w, RespInternalErr, "录像文件不可读")
		return
	}
	dbdata.AdminLog("WebVPN应用管理", s.Username+"@"+s.AppName+" "+sid, "下载了WebVPN会话录像", r.RemoteAddr)
	name := path.Base(rel)
	switch ext {
	case ".guac":
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Recording-Format", "guac")
		// 转码提示：guacenc 位于 guacamole/guacd 镜像内
		w.Header().Set("X-Recording-Hint", "guacenc -s 1280x720 "+name)
	default:
		w.Header().Set("Content-Type", "application/x-asciicast")
		w.Header().Set("X-Recording-Format", "asciicast")
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, fi.ModTime(), f)
}
