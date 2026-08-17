package admin

import (
	"net/http"
	"strconv"

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

func SetAuditExport(w http.ResponseWriter, r *http.Request) {
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
	gocsv.Marshal(datas, w)

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
