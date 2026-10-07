package admin

// WebVPN 访问：统一远程会话与 HTTP 访问（请求聚合）的列表 / 详情 / 摘要 / 导出

import (
	"encoding/csv"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/geoip"
	"github.com/wsczx/remlink/webvpn/live"
)

const (
	visitKindSession = "session"
	visitKindHTTP    = "http"
	// HTTP 访问最后一次请求在该时长内视为「在线」
	httpVisitActiveWindow = 5 * time.Minute
	visitFileLogLimit     = 1000
	visitDefaultRange     = 24 * time.Hour
	visitMaxRange         = 366 * 24 * time.Hour
)

// 统一访问记录
type webVpnVisit struct {
	Id          string                 `json:"id"`
	Kind        string                 `json:"kind"`
	Sid         string                 `json:"sid,omitempty"`
	Username    string                 `json:"username"`
	Group       string                 `json:"group"`
	AppName     string                 `json:"app_name"`
	Protocol    string                 `json:"protocol"`
	Target      string                 `json:"target"`
	ClientIP    string                 `json:"client_ip"`
	CountryCode string                 `json:"country_code"`
	StartedAt   time.Time              `json:"started_at"`
	EndedAt     *time.Time             `json:"ended_at"`
	LastActive  *time.Time             `json:"last_active,omitempty"`
	Online      bool                   `json:"online"`
	Duration    int64                  `json:"duration"` // 秒
	BytesIn     int64                  `json:"bytes_in"` // 客户端 → 目标
	BytesOut    int64                  `json:"bytes_out"`
	FileOps     int                    `json:"file_ops"`
	Requests    int                    `json:"requests"`
	Errors      int                    `json:"errors"`
	Risk        int                    `json:"risk"`
	Terminals   int32                  `json:"terminals"`
	Recording   dbdata.WebVpnRecording `json:"recording"`
	CloseReason string                 `json:"close_reason"`
	CloseText   string                 `json:"close_reason_text"`

	session *dbdata.WebVpnSession
}

var webVpnCloseText = map[string]string{
	live.ReasonUserClose:    "用户主动断开",
	live.ReasonIdle:         "空闲超时",
	live.ReasonKicked:       "管理员断开",
	live.ReasonRevoked:      "用户权限被吊销",
	live.ReasonUnauthorized: "授权失效",
	live.ReasonAppChanged:   "应用配置变更",
	live.ReasonRemoteClosed: "目标主机断开",
	live.ReasonError:        "连接异常",
	live.ReasonShutdown:     "服务重启",
}

func closeReasonText(r string) string {
	if t, ok := webVpnCloseText[r]; ok {
		return t
	}
	return r
}

func sessionVisit(s *dbdata.WebVpnSession, now time.Time) webVpnVisit {
	v := webVpnVisit{
		Id: visitKindSession + ":" + s.Sid, Kind: visitKindSession, Sid: s.Sid, Username: s.Username, Group: s.GroupName,
		AppName: s.AppName, Protocol: s.Protocol, Target: s.Target, ClientIP: s.ClientIP, StartedAt: s.StartedAt,
		EndedAt: s.EndedAt, BytesIn: s.BytesIn, BytesOut: s.BytesOut, CloseReason: s.CloseReason, session: s,
	}
	if ls := live.Default().Get(s.Sid); ls != nil && !ls.Closed() {
		la := ls.LastActive()
		v.Online, v.EndedAt, v.LastActive = true, nil, &la
		v.BytesIn, v.BytesOut, v.Terminals = ls.BytesIn(), ls.BytesOut(), ls.Terminals()
		v.CloseReason = ""
	} else if v.EndedAt == nil {
		// 库中未结束但注册表已无此会话（结束记录尚未落库）
		v.EndedAt = &now
	}
	end := now
	if v.EndedAt != nil {
		end = *v.EndedAt
	}
	v.Duration = int64(end.Sub(v.StartedAt).Seconds())
	v.CloseText = closeReasonText(v.CloseReason)
	return v
}

func httpVisit(h *dbdata.WebVpnHTTPVisit, now time.Time) webVpnVisit {
	end := h.EndedAt
	v := webVpnVisit{
		Id: h.ID(), Kind: visitKindHTTP, Username: h.Username, Group: h.GroupName, AppName: h.AppName,
		Protocol: dbdata.WebVpnProtoHTTP, Target: h.Host, ClientIP: h.ClientIP, StartedAt: h.StartedAt, EndedAt: &end,
		Online: now.Sub(h.EndedAt) <= httpVisitActiveWindow, Duration: int64(h.EndedAt.Sub(h.StartedAt).Seconds()),
		BytesIn: h.BytesIn, BytesOut: h.BytesOut, Requests: h.Requests, Errors: h.Errors, Risk: int(h.Risk),
		Recording: dbdata.WebVpnRecording{Reason: dbdata.RecReasonHTTP},
	}
	v.LastActive = &end
	return v
}

// ---------------- 查询参数 ----------------

type visitQuery struct {
	start, end time.Time
	endIsNow   bool
	q          string
	protocol   string // 空=全部；http；remote=全部远程协议；或具体协议
	online     bool
	hasRec     bool // 仅有录像的会话
	hasFileOps bool // 仅有文件操作的会话
	hasErrors  bool // 仅有错误/可疑请求的 HTTP 访问
}

func formBool(r *http.Request, k string) bool {
	v := r.FormValue(k)
	return v == "1" || v == "true"
}

func parseVisitTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1e12 { // 毫秒
			return time.UnixMilli(n), true
		}
		return time.Unix(n, 0), true
	}
	for _, l := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, true
		}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.In(time.Local), true
	}
	return time.Time{}, false
}

func parseVisitQuery(r *http.Request) visitQuery {
	_ = r.ParseForm()
	now := time.Now()
	vq := visitQuery{
		q:          strings.TrimSpace(r.FormValue("q")),
		protocol:   strings.ToLower(strings.TrimSpace(r.FormValue("protocol"))),
		online:     formBool(r, "online"),
		hasRec:     formBool(r, "has_recording"),
		hasFileOps: formBool(r, "has_file_ops"),
		hasErrors:  formBool(r, "has_errors"),
	}
	var ok bool
	if vq.end, ok = parseVisitTime(r.FormValue("end")); !ok || vq.end.After(now) {
		vq.end, vq.endIsNow = now, true
	}
	if vq.start, ok = parseVisitTime(r.FormValue("start")); !ok || !vq.start.Before(vq.end) {
		vq.start = vq.end.Add(-visitDefaultRange)
	}
	if vq.end.Sub(vq.start) > visitMaxRange {
		vq.start = vq.end.Add(-visitMaxRange)
	}
	return vq
}

// ---------------- HTTP 聚合缓存（翻页时避免重复扫描） ----------------

type httpVisitCacheEntry struct {
	at        time.Time
	visits    []dbdata.WebVpnHTTPVisit
	truncated bool
}

var httpVisitCache = struct {
	sync.Mutex
	m map[string]httpVisitCacheEntry
}{m: map[string]httpVisitCacheEntry{}}

const httpVisitCacheTTL = 10 * time.Second

func scanHTTPVisits(vq visitQuery) ([]dbdata.WebVpnHTTPVisit, bool, error) {
	end := strconv.FormatInt(vq.end.Unix(), 10)
	if vq.endIsNow {
		end = "now"
	}
	key := strconv.FormatInt(vq.start.Unix(), 10) + "|" + end + "|" + vq.q
	now := time.Now()
	httpVisitCache.Lock()
	if e, ok := httpVisitCache.m[key]; ok && now.Sub(e.at) < httpVisitCacheTTL {
		httpVisitCache.Unlock()
		return e.visits, e.truncated, nil
	}
	httpVisitCache.Unlock()
	visits, truncated, err := dbdata.ScanWebVpnHTTPVisits(vq.start, vq.end, vq.q, 0)
	if err != nil {
		return nil, false, err
	}
	httpVisitCache.Lock()
	for k, e := range httpVisitCache.m {
		if now.Sub(e.at) >= httpVisitCacheTTL {
			delete(httpVisitCache.m, k)
		}
	}
	if len(httpVisitCache.m) < 32 {
		httpVisitCache.m[key] = httpVisitCacheEntry{at: now, visits: visits, truncated: truncated}
	}
	httpVisitCache.Unlock()
	return visits, truncated, nil
}

// 汇总时间窗口内的全部访问（按开始时间倒序）；truncated 表示 HTTP 请求扫描达到上限
func collectVisits(vq visitQuery) ([]webVpnVisit, bool, error) {
	now := time.Now()
	var out []webVpnVisit
	truncated := false
	wantHTTP := (vq.protocol == "" || vq.protocol == dbdata.WebVpnProtoHTTP) && !vq.hasRec && !vq.hasFileOps
	wantSession := vq.protocol != dbdata.WebVpnProtoHTTP && !vq.hasErrors
	if wantHTTP {
		hv, tr, err := scanHTTPVisits(vq)
		if err != nil {
			return nil, false, err
		}
		truncated = tr
		for i := range hv {
			out = append(out, httpVisit(&hv[i], now))
		}
	}
	if wantSession {
		proto := vq.protocol
		if proto == "remote" {
			proto = ""
		}
		ss, err := dbdata.ListWebVpnSessionsForVisits(vq.start, vq.end, vq.q, proto, 0)
		if err != nil {
			return nil, false, err
		}
		var counts map[string]int
		if vq.hasFileOps {
			sids := make([]string, len(ss))
			for i := range ss {
				sids[i] = ss[i].Sid
			}
			if counts, err = dbdata.WebVpnFileOpCounts(sids); err != nil {
				return nil, false, err
			}
		}
		for i := range ss {
			if vq.hasRec && ss[i].RecordingPath == "" {
				continue
			}
			if vq.hasFileOps && counts[ss[i].Sid] == 0 {
				continue
			}
			out = append(out, sessionVisit(&ss[i], now))
		}
	}
	kept := out[:0]
	for _, v := range out {
		if vq.online && !v.Online {
			continue
		}
		if vq.hasErrors && v.Errors == 0 && v.Risk == 0 {
			continue
		}
		kept = append(kept, v)
	}
	out = kept
	sort.SliceStable(out, func(i, j int) bool {
		// 在线优先，其余按开始时间倒序
		if out[i].Online != out[j].Online {
			return out[i].Online
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, truncated, nil
}

// 补全展示字段：国家、文件操作次数、录像状态（只对当前页执行，涉及文件 stat）
func enrichVisits(vs []webVpnVisit, withParts bool) error {
	var sids []string
	for i := range vs {
		vs[i].CountryCode = geoip.Lookup(vs[i].ClientIP)
		if vs[i].Kind == visitKindSession {
			sids = append(sids, vs[i].Sid)
		}
	}
	if len(sids) == 0 {
		return nil
	}
	counts, err := dbdata.WebVpnFileOpCounts(sids)
	if err != nil {
		return err
	}
	for i := range vs {
		v := &vs[i]
		if v.Kind != visitKindSession || v.session == nil {
			continue
		}
		v.FileOps = counts[v.Sid]
		v.Recording = dbdata.WebVpnRecordingOf(v.session, dbdata.WebVpnRecordExists)
		if withParts && v.Recording.Available {
			v.Recording.Parts = dbdata.WebVpnRecordingParts(v.session.RecordingPath, dbdata.WebVpnRecordExists)
		}
	}
	return nil
}

// ---------------- 接口 ----------------

// GET /api/v1/webvpn/visits?page&page_size&q&online&protocol&start&end
func WebVpnVisitList(w http.ResponseWriter, r *http.Request) {
	vq := parseVisitQuery(r)
	page, pageSize := webVpnPageArgs(r)
	all, truncated, err := collectVisits(vq)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	lo := (page - 1) * pageSize
	if lo > len(all) {
		lo = len(all)
	}
	hi := lo + pageSize
	if hi > len(all) {
		hi = len(all)
	}
	rows := append([]webVpnVisit{}, all[lo:hi]...)
	if err := enrichVisits(rows, false); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	RespSucess(w, map[string]any{
		"count": len(all), "page_size": pageSize, "datas": rows, "truncated": truncated,
		"scan_limit": dbdata.WebVpnVisitScanLimit,
		"start":      vq.start, "end": vq.end,
	})
}

// GET /api/v1/webvpn/visits/summary：当前在线 / 今日访问 / 今日文件操作
func WebVpnVisitSummary(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	visits, truncated, err := collectVisits(visitQuery{start: today, end: now, endIsNow: true})
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	online, todayN := 0, 0
	for _, v := range visits {
		if v.Online {
			online++
		}
		if !v.StartedAt.Before(today) {
			todayN++
		}
	}
	// 跨天仍活跃的 HTTP 访问不在今日窗口内：补扫最近活跃窗口
	if hv, _, err := dbdata.ScanWebVpnHTTPVisits(now.Add(-httpVisitActiveWindow), now, "", 0); err == nil {
		seen := map[string]bool{}
		for _, v := range visits {
			if v.Kind == visitKindHTTP && v.Online {
				seen[v.Username+"\x00"+v.AppName+"\x00"+v.ClientIP] = true
			}
		}
		for _, h := range hv {
			if h.StartedAt.Before(today) && !seen[h.Username+"\x00"+h.AppName+"\x00"+h.ClientIP] {
				online++
			}
		}
	}
	fileOps, err := dbdata.WebVpnFileOpCountSince(today)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	RespSucess(w, map[string]any{
		"online":         online,
		"online_remote":  live.Default().Count(),
		"today_visits":   todayN,
		"today_file_ops": fileOps,
		"truncated":      truncated,
	})
}

// GET /api/v1/webvpn/visits/detail?id=session:<sid> | http:<first>-<last> [&page&page_size]（HTTP 请求分页）
func WebVpnVisitDetail(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	kind, ref, _ := strings.Cut(r.FormValue("id"), ":")
	now := time.Now()
	switch kind {
	case visitKindSession:
		s, err := dbdata.GetWebVpnSession(ref)
		if err != nil {
			RespError(w, RespParamErr, "会话不存在")
			return
		}
		v := []webVpnVisit{sessionVisit(s, now)}
		if err := enrichVisits(v, true); err != nil {
			RespError(w, RespInternalErr, err)
			return
		}
		logs, total, err := dbdata.WebVpnSessionFileLogs(s.Sid, visitFileLogLimit)
		if err != nil {
			RespError(w, RespInternalErr, err)
			return
		}
		if logs == nil {
			logs = []dbdata.WebVpnFileLog{}
		}
		RespSucess(w, map[string]any{"visit": v[0], "file_logs": logs, "file_log_total": total, "file_log_limit": visitFileLogLimit})
	case visitKindHTTP:
		a, b, ok := strings.Cut(ref, "-")
		first, err1 := strconv.ParseInt(a, 10, 64)
		last, err2 := strconv.ParseInt(b, 10, 64)
		if !ok || err1 != nil || err2 != nil {
			RespError(w, RespParamErr, "参数错误")
			return
		}
		h, err := dbdata.GetWebVpnHTTPVisit(first, last)
		if err != nil {
			RespError(w, RespParamErr, "访问记录不存在或已被清理")
			return
		}
		page, pageSize := webVpnPageArgs(r)
		reqs, err := dbdata.WebVpnHTTPVisitRequests(h, page, pageSize)
		if err != nil {
			RespError(w, RespInternalErr, err)
			return
		}
		if reqs == nil {
			reqs = []dbdata.WebVpnAudit{}
		}
		v := []webVpnVisit{httpVisit(h, now)}
		_ = enrichVisits(v, false)
		RespSucess(w, map[string]any{"visit": v[0], "requests": reqs, "count": h.Requests, "page_size": pageSize})
	default:
		RespError(w, RespParamErr, "参数错误")
	}
}

var recordingReasonText = map[string]string{
	dbdata.RecReasonRecorded:     "有录像",
	dbdata.RecReasonDisabled:     "未开启录像",
	dbdata.RecReasonFileProtocol: "文件类协议无屏幕录像",
	dbdata.RecReasonNoTerminal:   "未打开终端/桌面",
	dbdata.RecReasonFileMissing:  "录像文件不存在",
	dbdata.RecReasonHTTP:         "HTTP 应用无录像",
	dbdata.RecReasonFailed:       "录像失败",
	dbdata.RecReasonUnknown:      "未开启录像或未打开终端",
}

// GET /api/v1/webvpn/visits/export：导出当前筛选的访问记录（CSV）
func WebVpnVisitExport(w http.ResponseWriter, r *http.Request) {
	vq := parseVisitQuery(r)
	all, truncated, err := collectVisits(vq)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if len(all) == 0 {
		RespError(w, RespParamErr, "当前筛选条件下没有访问记录")
		return
	}
	if err := enrichVisits(all, false); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("WebVPN应用管理", "访问记录", "导出了WebVPN访问记录 "+strconv.Itoa(len(all))+" 条", r.RemoteAddr)
	name := "webvpn_visits_" + time.Now().Format("20060102_150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if truncated {
		w.Header().Set("X-Truncated", "1")
	}
	_, _ = w.Write([]byte("\xEF\xBB\xBF")) // Excel 识别 UTF-8
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"类型", "用户", "用户组", "应用", "协议", "目标", "客户端IP", "国家/地区", "开始时间", "结束时间",
		"时长(秒)", "在线", "上行字节", "下行字节", "请求数", "错误请求数", "文件操作数", "录像", "断开原因"})
	const tl = "2006-01-02 15:04:05"
	for _, v := range all {
		kind, end, online := "远程会话", "", "否"
		if v.Kind == visitKindHTTP {
			kind = "HTTP 访问"
		}
		if v.EndedAt != nil && !v.Online {
			end = v.EndedAt.Format(tl)
		}
		if v.Online {
			online = "是"
		}
		_ = cw.Write([]string{kind, v.Username, v.Group, v.AppName, strings.ToUpper(v.Protocol), v.Target, v.ClientIP,
			v.CountryCode, v.StartedAt.Format(tl), end, strconv.FormatInt(v.Duration, 10), online,
			strconv.FormatInt(v.BytesIn, 10), strconv.FormatInt(v.BytesOut, 10), strconv.Itoa(v.Requests),
			strconv.Itoa(v.Errors), strconv.Itoa(v.FileOps), recordingReasonText[v.Recording.Reason], v.CloseText})
	}
	cw.Flush()
}
