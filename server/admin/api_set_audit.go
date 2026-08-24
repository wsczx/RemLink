package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/wsczx/remlink/dbdata"
)

func SetAuditList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	pageS := r.FormValue("page")
	page, _ := strconv.Atoi(pageS)
	if page < 1 {
		page = 1
	}
	var datas []dbdata.AccessAudit
	session := dbdata.GetAuditSession(r.Form)
	count, err := dbdata.FindAndCount(session, &datas, dbdata.PageSize, page)
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}
	// 确保空结果返回 [] 而非 null
	if datas == nil {
		datas = []dbdata.AccessAudit{}
	}

	names := make([]string, 0, len(datas))
	for _, d := range datas {
		names = append(names, d.Username)
	}
	nickMap := dbdata.NicknameMap(names)
	for i := range datas {
		datas[i].Nickname = nickMap[datas[i].Username]
	}

	data := map[string]any{
		"count":     count,
		"page_size": dbdata.PageSize,
		"datas":     datas,
	}

	RespSucess(w, data)
}

type accessAuditExportRow struct {
	ID             int       `csv:"ID"`
	Username       string    `csv:"用户名"`
	GroupName      string    `csv:"用户组"`
	SourceIP       string    `csv:"源IP地址"`
	SourcePort     uint16    `csv:"源端口"`
	TargetIP       string    `csv:"目的IP地址"`
	TargetPort     uint16    `csv:"目的端口"`
	IPProtocol     string    `csv:"IP协议"`
	AccessProtocol string    `csv:"访问协议"`
	Info           string    `csv:"访问详情"`
	CreatedAt      time.Time `csv:"创建时间"`
}

func SetAuditExport(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	var datas []dbdata.AccessAudit
	maxNum := 1000000
	session := dbdata.GetAuditSession(r.Form)
	count, err := dbdata.FindAndCount(session, &datas, maxNum, 0)
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}
	if count == 0 {
		RespError(w, RespParamErr, "你导出的总数量为0条，请调整搜索条件，再导出")
		return
	}
	if count > int64(maxNum) {
		RespError(w, RespParamErr, "你导出的数据量超过100万条，请调整搜索条件，再导出")
		return
	}
	dbdata.AdminLog("系统设置", "审计日志导出", "导出了访问审计日志("+r.FormValue("search")+")", r.RemoteAddr)
	rows := make([]accessAuditExportRow, 0, len(datas))
	for _, audit := range datas {
		rows = append(rows, accessAuditExportRow{
			ID:             audit.Id,
			Username:       audit.Username,
			GroupName:      audit.GroupName,
			SourceIP:       audit.Src,
			SourcePort:     audit.SrcPort,
			TargetIP:       audit.Dst,
			TargetPort:     audit.DstPort,
			IPProtocol:     auditIPProtocolName(audit.Protocol),
			AccessProtocol: auditAccessProtocolName(audit.Protocol, audit.AccessProto),
			Info:           audit.Info,
			CreatedAt:      audit.CreatedAt,
		})
	}
	if err := gocsv.Marshal(rows, w); err != nil {
		return
	}

}

func auditIPProtocolName(protocol uint8) string {
	switch protocol {
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	default:
		return strconv.Itoa(int(protocol))
	}
}

func auditAccessProtocolName(protocol, accessProto uint8) string {
	if accessProto == 0 {
		switch protocol {
		case 6:
			return "TCP"
		case 17:
			return "UDP"
		}
	}
	protocolNames := [...]string{"", "UDP", "TCP", "HTTPS", "HTTP", "DNS", "SSH", "FTP", "SMTP", "IMAP", "POP3"}
	if int(accessProto) < len(protocolNames) {
		return protocolNames[accessProto]
	}
	return strconv.Itoa(int(accessProto))
}

func UserActLogList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	pageS := r.FormValue("page")
	page, _ := strconv.Atoi(pageS)
	if page < 1 {
		page = 1
	}
	var datas []dbdata.UserActLog
	session := dbdata.UserActLogIns.GetSession(r.Form)
	count, err := dbdata.FindAndCount(session, &datas, dbdata.PageSize, page)
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}
	// 确保空结果返回 [] 而非 null
	if datas == nil {
		datas = []dbdata.UserActLog{}
	}

	names := make([]string, 0, len(datas))
	for _, d := range datas {
		names = append(names, d.Username)
	}
	nickMap := dbdata.NicknameMap(names)
	for i := range datas {
		datas[i].Nickname = nickMap[datas[i].Username]
	}

	data := map[string]any{
		"count":     count,
		"page_size": dbdata.PageSize,
		"datas":     datas,
		"statusOps": dbdata.UserActLogIns.GetStatusOpsWithTag(),
		"osOps":     dbdata.UserActLogIns.OsOps,
		"clientOps": dbdata.UserActLogIns.ClientOps,
	}

	RespSucess(w, data)
}

// 管理员操作日志分页查询
func AdminOpLogList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	pageS := r.FormValue("page")
	page, _ := strconv.Atoi(pageS)
	if page < 1 {
		page = 1
	}
	var datas []dbdata.AdminOpLog
	session := dbdata.GetAdminOpLogSession(r.Form)
	count, err := dbdata.FindAndCount(session, &datas, dbdata.PageSize, page)
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}
	// 确保空结果返回 [] 而非 null
	if datas == nil {
		datas = []dbdata.AdminOpLog{}
	}
	data := map[string]any{
		"count":     count,
		"page_size": dbdata.PageSize,
		"datas":     datas,
		"opTypes":   dbdata.AdminOpLogTypes,
	}

	RespSucess(w, data)
}
