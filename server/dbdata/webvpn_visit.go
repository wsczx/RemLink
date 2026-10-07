package dbdata

// WebVPN「访问」：统一远程会话（webvpn_session）与 HTTP 请求聚合（webvpn_audit）
//   - 远程协议：一条 webvpn_session 即一次访问
//   - HTTP 应用：同一 (用户, 应用, 客户端 IP) 相邻请求间隔 ≤ WebVpnHTTPVisitGap 的请求聚合为一次访问
//
// 聚合在 Go 侧按时间窗口顺序扫描完成（不依赖窗口函数，兼容 sqlite/mysql/postgres），扫描量有上限。

import (
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"xorm.io/xorm"
)

const (
	WebVpnHTTPVisitGap      = 30 * time.Minute
	WebVpnVisitScanLimit    = 200000 // 单次聚合最多扫描的 HTTP 请求条数（从最新往前）
	WebVpnVisitSessionLimit = 20000  // 单次最多读取的远程会话条数
	webVpnVisitScanBatch    = 5000
	webVpnTimeLayout        = "2006-01-02 15:04:05"
)

// 会话开始时的录像状态（webvpn_session.record_mode）
const (
	WebVpnRecordOn  = "on"
	WebVpnRecordOff = "off"
	WebVpnRecordNA  = "na"
)

// 录像可用性原因
const (
	RecReasonRecorded     = "recorded"         // 有录像
	RecReasonDisabled     = "disabled"         // 会话开始时应用未开启录像
	RecReasonFileProtocol = "file_protocol"    // SFTP/SMB/FTP 只记录文件操作
	RecReasonNoTerminal   = "no_terminal"      // 已开启录像，但会话期间未打开终端/桌面
	RecReasonFileMissing  = "file_missing"     // 录像文件不存在（被清理或 guacd 录像目录未共享）
	RecReasonHTTP         = "http"             // HTTP 应用无屏幕录像
	RecReasonFailed       = "recording_failed" // 录像文件创建失败
	RecReasonUnknown      = "unknown"          // 旧数据：无法区分「未开启录像」与「未打开终端」
)

// 录像信息
type WebVpnRecording struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	Format    string `json:"format,omitempty"` // asciicast | guac
	Parts     int    `json:"parts"`
	Error     string `json:"error,omitempty"` // recording_failed 时的具体错误
}

// 是否为纯文件协议（无屏幕，不录像）
func WebVpnIsFileProtocol(p string) bool {
	switch p {
	case WebVpnProtoSFTP, WebVpnProtoSMB, WebVpnProtoFTP:
		return true
	}
	return false
}

// 会话开始时的录像状态
func WebVpnRecordModeOf(app *WebVpnApp) string {
	switch {
	case WebVpnIsFileProtocol(app.ProtocolOf()) || app.IsHTTP():
		return WebVpnRecordNA
	case app.Record:
		return WebVpnRecordOn
	}
	return WebVpnRecordOff
}

func webVpnRecordFormat(proto, rel string) string {
	if strings.HasSuffix(rel, ".guac") {
		return "guac"
	}
	if strings.HasSuffix(rel, ".cast") {
		return "asciicast"
	}
	switch proto {
	case WebVpnProtoSSH:
		return "asciicast"
	case WebVpnProtoRDP, WebVpnProtoVNC:
		return "guac"
	}
	return ""
}

// 推断会话录像状态。exists 判断录像相对路径对应的文件是否存在（nil 表示不检查）
func WebVpnRecordingOf(s *WebVpnSession, exists func(rel string) bool) WebVpnRecording {
	proto := strings.ToLower(s.Protocol)
	out := WebVpnRecording{Format: webVpnRecordFormat(proto, s.RecordingPath)}
	switch {
	case proto == "" || proto == WebVpnProtoHTTP:
		out.Reason, out.Format = RecReasonHTTP, ""
	case s.RecordingPath != "":
		if exists != nil && !exists(s.RecordingPath) {
			out.Reason = RecReasonFileMissing
		} else {
			out.Available, out.Reason, out.Parts = true, RecReasonRecorded, 1
		}
	case WebVpnIsFileProtocol(proto) || s.RecordMode == WebVpnRecordNA:
		out.Reason, out.Format = RecReasonFileProtocol, ""
	case s.RecordMode == WebVpnRecordOff:
		out.Reason = RecReasonDisabled
	case s.RecordMode == WebVpnRecordOn && s.RecordErr != "":
		out.Reason, out.Error = RecReasonFailed, s.RecordErr
	case s.RecordMode == WebVpnRecordOn:
		out.Reason = RecReasonNoTerminal
	default:
		out.Reason = RecReasonUnknown
	}
	return out
}

// 录像文件是否存在（相对录像目录）
func WebVpnRecordExists(rel string) bool {
	full, err := WebVpnRecordFullPath(rel)
	if err != nil {
		return false
	}
	fi, err := os.Stat(full)
	return err == nil && fi.Mode().IsRegular()
}

// guac 录像分段数：<sid>.guac、<sid>.2.guac、<sid>.3.guac…（连续存在的段数）
func WebVpnRecordingParts(rel string, exists func(rel string) bool) int {
	if rel == "" || !exists(rel) {
		return 0
	}
	if path.Ext(rel) != ".guac" {
		return 1
	}
	stem := strings.TrimSuffix(rel, ".guac")
	n := 1
	for n < 1000 && exists(stem+"."+strconv.Itoa(n+1)+".guac") {
		n++
	}
	return n
}

// ---------------- HTTP 访问聚合 ----------------

// 一次 HTTP 访问（同一用户/应用/IP 的连续请求）
type WebVpnHTTPVisit struct {
	FirstId   int64
	LastId    int64
	Username  string
	GroupName string
	AppName   string
	Host      string
	ClientIP  string
	StartedAt time.Time
	EndedAt   time.Time // 最后一次请求时间
	Requests  int
	Errors    int // 状态码 >= 400 的请求数
	BytesIn   int64
	BytesOut  int64
	Risk      int8
}

// 访问 ID：http:<首条请求 id>-<末条请求 id>
func (v *WebVpnHTTPVisit) ID() string {
	return "http:" + strconv.FormatInt(v.FirstId, 10) + "-" + strconv.FormatInt(v.LastId, 10)
}

func (v *WebVpnHTTPVisit) add(r *WebVpnAudit) {
	if v.Requests == 0 || r.CreatedAt.Before(v.StartedAt) {
		v.StartedAt = r.CreatedAt
	}
	if v.Requests == 0 || r.CreatedAt.After(v.EndedAt) {
		v.EndedAt = r.CreatedAt
	}
	if v.FirstId == 0 || r.Id < v.FirstId {
		v.FirstId = r.Id
	}
	if r.Id > v.LastId {
		v.LastId = r.Id
	}
	if v.GroupName == "" {
		v.GroupName = r.GroupName
	}
	if v.Host == "" {
		v.Host = r.Host
	}
	v.Requests++
	if r.StatusCode >= 400 {
		v.Errors++
	}
	v.BytesIn += r.BytesRecv
	v.BytesOut += r.BytesSent
	if r.RiskLevel > v.Risk {
		v.Risk = r.RiskLevel
	}
}

func webVpnVisitKey(r *WebVpnAudit) string {
	return r.Username + "\x00" + r.AppName + "\x00" + r.ClientIP
}

// 把请求聚合为访问：按 (用户, 应用, IP) 分组，相邻请求间隔 > gap 则切分。
// 输入顺序任意；输出按开始时间倒序
func AggregateWebVpnHTTPVisits(rows []WebVpnAudit, gap time.Duration) []WebVpnHTTPVisit {
	sorted := make([]*WebVpnAudit, len(rows))
	for i := range rows {
		sorted[i] = &rows[i]
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
		}
		return sorted[i].Id < sorted[j].Id
	})
	var out []WebVpnHTTPVisit
	open := map[string]int{} // key -> out 下标
	for _, r := range sorted {
		k := webVpnVisitKey(r)
		if i, ok := open[k]; ok && r.CreatedAt.Sub(out[i].EndedAt) <= gap {
			out[i].add(r)
			continue
		}
		out = append(out, WebVpnHTTPVisit{Username: r.Username, AppName: r.AppName, ClientIP: r.ClientIP})
		out[len(out)-1].add(r)
		open[k] = len(out) - 1
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

func webVpnLike(q string) string {
	return "%" + strings.NewReplacer("%", "", "_", "").Replace(q) + "%"
}

// 扫描时间窗口内的 HTTP 请求并聚合为访问。q 为用户/应用/IP 模糊匹配；
// 从最新请求往前扫描，超过 limit 条时截断（truncated=true，较早的请求不参与统计）
func ScanWebVpnHTTPVisits(start, end time.Time, q string, limit int) ([]WebVpnHTTPVisit, bool, error) {
	if limit <= 0 {
		limit = WebVpnVisitScanLimit
	}
	var all []WebVpnAudit
	var lastId int64
	truncated := false
	for {
		n := webVpnVisitScanBatch
		if rest := limit - len(all); rest < n {
			n = rest
		}
		if n <= 0 {
			truncated = true
			break
		}
		ipCol := webVpnAuditIPCol()
		s := xdb.Cols("id", "username", "group_name", "app_name", "host", ipCol, "status_code",
			"bytes_sent", "bytes_recv", "risk_level", "created_at").
			Where("created_at BETWEEN ? AND ?", start.Format(webVpnTimeLayout), end.Format(webVpnTimeLayout))
		if q != "" {
			like := webVpnLike(q)
			s.And("(username LIKE ? OR app_name LIKE ? OR "+ipCol+" LIKE ?)", like, like, like)
		}
		if lastId > 0 {
			s.And("id < ?", lastId)
		}
		var batch []WebVpnAudit
		if err := s.OrderBy("id desc").Limit(n).Find(&batch); err != nil {
			return nil, false, err
		}
		all = append(all, batch...)
		if len(batch) < n {
			break
		}
		lastId = batch[len(batch)-1].Id
	}
	return AggregateWebVpnHTTPVisits(all, WebVpnHTTPVisitGap), truncated, nil
}

// 按访问 ID 的请求区间还原一次 HTTP 访问（汇总在数据库侧完成）
func GetWebVpnHTTPVisit(firstId, lastId int64) (*WebVpnHTTPVisit, error) {
	var first WebVpnAudit
	has, err := xdb.ID(firstId).Get(&first)
	if err != nil {
		return nil, err
	}
	if !has || lastId < firstId {
		return nil, ErrNotFound
	}
	var last WebVpnAudit
	if has, err = webVpnVisitScope(&first, firstId, lastId).OrderBy("id desc").Limit(1).Get(&last); err != nil {
		return nil, err
	}
	if !has {
		last = first
	}
	rows, err := webVpnVisitScope(&first, firstId, lastId).Table("webvpn_audit").
		Select("COUNT(*) AS n, SUM(bytes_sent) AS bs, SUM(bytes_recv) AS br, MAX(risk_level) AS rk, " +
			"SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) AS errs").QueryString()
	if err != nil {
		return nil, err
	}
	v := &WebVpnHTTPVisit{
		FirstId: firstId, LastId: last.Id, Username: first.Username, GroupName: first.GroupName, AppName: first.AppName,
		Host: first.Host, ClientIP: first.ClientIP, StartedAt: first.CreatedAt, EndedAt: last.CreatedAt,
	}
	if len(rows) == 1 {
		num := func(k string) int64 {
			f, _ := strconv.ParseFloat(rows[0][k], 64)
			return int64(f)
		}
		v.Requests, v.BytesOut, v.BytesIn = int(num("n")), num("bs"), num("br")
		v.Risk, v.Errors = int8(num("rk")), int(num("errs"))
	}
	return v, nil
}

func webVpnVisitScope(first *WebVpnAudit, firstId, lastId int64) *xorm.Session {
	return xdb.Where("id BETWEEN ? AND ?", firstId, lastId).
		And("username = ? AND app_name = ? AND "+webVpnAuditIPCol()+" = ?", first.Username, first.AppName, first.ClientIP)
}

// webvpn_audit.ClientIP 未显式命名列，实际列名由 xorm 映射器决定（SnakeMapper 下为 client_i_p）
func webVpnAuditIPCol() string {
	return xdb.GetColumnMapper().Obj2Table("ClientIP")
}

// 一次 HTTP 访问内的请求（按时间正序分页）
func WebVpnHTTPVisitRequests(v *WebVpnHTTPVisit, page, pageSize int) ([]WebVpnAudit, error) {
	first := &WebVpnAudit{Username: v.Username, AppName: v.AppName, ClientIP: v.ClientIP}
	var datas []WebVpnAudit
	err := webVpnVisitScope(first, v.FirstId, v.LastId).OrderBy("id asc").Limit(pageSize, (page-1)*pageSize).Find(&datas)
	return datas, err
}

// ---------------- 远程会话 ----------------

// 时间窗口内开始的会话 + 仍未结束的会话（在线）
func ListWebVpnSessionsForVisits(start, end time.Time, q, protocol string, limit int) ([]WebVpnSession, error) {
	if limit <= 0 {
		limit = WebVpnVisitSessionLimit
	}
	s := xdb.Where("(started_at BETWEEN ? AND ? OR ended_at IS NULL)", start.Format(webVpnTimeLayout), end.Format(webVpnTimeLayout))
	if q != "" {
		like := webVpnLike(q)
		s.And("(username LIKE ? OR app_name LIKE ? OR client_ip LIKE ? OR target LIKE ?)", like, like, like, like)
	}
	if protocol != "" {
		s.And("protocol = ?", protocol)
	}
	var datas []WebVpnSession
	err := s.OrderBy("id desc").Limit(limit).Find(&datas)
	return datas, err
}

// 各会话的文件操作次数（不含目录浏览 list）
func WebVpnFileOpCounts(sids []string) (map[string]int, error) {
	out := make(map[string]int, len(sids))
	for i := 0; i < len(sids); i += 500 {
		j := i + 500
		if j > len(sids) {
			j = len(sids)
		}
		args := make([]any, 0, j-i)
		for _, s := range sids[i:j] {
			args = append(args, s)
		}
		rows, err := xdb.Table("webvpn_file_log").Select("sid, COUNT(*) AS n").In("sid", args...).
			And("op <> ?", "list").GroupBy("sid").QueryString()
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			n, _ := strconv.Atoi(r["n"])
			out[r["sid"]] = n
		}
	}
	return out, nil
}

// 某时刻以来的文件操作次数（不含目录浏览）
func WebVpnFileOpCountSince(t time.Time) (int64, error) {
	return xdb.Where("created_at >= ? AND op <> ?", t.Format(webVpnTimeLayout), "list").Count(&WebVpnFileLog{})
}

// 会话的文件操作时间线（正序，最多 limit 条）
func WebVpnSessionFileLogs(sid string, limit int) ([]WebVpnFileLog, int64, error) {
	total, err := xdb.Where("sid = ?", sid).Count(&WebVpnFileLog{})
	if err != nil {
		return nil, 0, err
	}
	var datas []WebVpnFileLog
	err = xdb.Where("sid = ?", sid).OrderBy("id asc").Limit(limit).Find(&datas)
	return datas, total, err
}
