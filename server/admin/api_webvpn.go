package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 返回 WebVPN 泛域名后缀（如 wv.example.com），供前端拼接访问地址。
func WebVpnDomain(w http.ResponseWriter, r *http.Request) {
	RespSucess(w, map[string]any{
		"domain": base.GetCfg().WebVpnDomain,
	})
}

// 返回 WebVPN 应用分页列表
func WebVpnAppList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	page, _ := strconv.Atoi(r.FormValue("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.FormValue("page_size"))
	if pageSize <= 0 {
		pageSize = dbdata.PageSize
	}

	datas, count, err := dbdata.WebVpnAppList(pageSize, page, r.FormValue("name"))
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}
	if datas == nil {
		datas = []dbdata.WebVpnApp{}
	}

	RespSucess(w, map[string]any{
		"count":     count,
		"page_size": pageSize,
		"datas":     datas,
	})
}

// 返回单个 WebVPN 应用详情
func WebVpnAppDetail(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id, _ := strconv.Atoi(r.FormValue("id"))
	if id < 1 {
		RespError(w, RespParamErr, "Id错误")
		return
	}

	var data dbdata.WebVpnApp
	if err := dbdata.One("Id", id, &data); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if data.Users == nil {
		data.Users = []string{}
	}
	if data.Groups == nil {
		data.Groups = []string{}
	}
	if data.AllowPath == nil {
		data.AllowPath = []string{}
	}
	if data.IpAllowList == nil {
		data.IpAllowList = []string{}
	}

	RespSucess(w, data)
}

// 新增或更新 WebVPN 应用
func WebVpnAppSet(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	v := &dbdata.WebVpnApp{}
	if err := json.Unmarshal(body, v); err != nil {
		RespError(w, RespParamErr, "参数错误")
		return
	}

	if err := dbdata.SetWebVpnApp(v); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	dbdata.AdminLog("WebVPN应用管理", v.Name, "创建/修改了WebVPN应用", r.RemoteAddr)
	RespSucess(w, nil)
}

// 删除 WebVPN 应用
func WebVpnAppDel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Id int `json:"id"`
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || json.Unmarshal(body, &req) != nil || req.Id < 1 {
		RespError(w, RespParamErr, "Id错误")
		return
	}
	id := req.Id

	a := &dbdata.WebVpnApp{}
	dbdata.One("Id", id, a)

	if err := dbdata.DelWebVpnApp(id); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	dbdata.AdminLog("WebVPN应用管理", a.Name, "删除了WebVPN应用", r.RemoteAddr)
	RespSucess(w, nil)
}

// 返回 WebVPN 访问审计分页列表（支持用户名/应用名/时间范围过滤）
func WebVpnAuditList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	page, _ := strconv.Atoi(r.FormValue("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.FormValue("page_size"))
	if pageSize <= 0 {
		pageSize = dbdata.PageSize
	}
	search := dbdata.WebVpnAuditSearch{
		Username: r.FormValue("username"),
		AppName:  r.FormValue("app_name"),
		Method:   r.FormValue("method"),
	}
	if d := r.FormValue("start_date"); d != "" {
		end := r.FormValue("end_date")
		if end == "" {
			end = time.Now().Format("2006-01-02 15:04:05")
		}
		search.Date = []string{d, end}
	}

	datas, count, err := dbdata.WebVpnAuditList(pageSize, page, search)
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}
	if datas == nil {
		datas = []dbdata.WebVpnAudit{}
	}

	RespSucess(w, map[string]any{
		"count":     count,
		"page_size": pageSize,
		"datas":     datas,
	})
}

// 踢出指定用户的全部 WebVPN 会话（整用户下线）
func WebVpnSessionKick(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || json.Unmarshal(body, &req) != nil || req.Username == "" {
		RespError(w, RespParamErr, "用户名不能为空")
		return
	}
	dbdata.WebVpnRevokeUser(req.Username)
	dbdata.AdminLog("WebVPN应用管理", req.Username, "踢出了该用户的所有WebVPN会话", r.RemoteAddr)
	RespSucess(w, nil)
}

// 导出 WebVPN 访问审计（CSV，上限 100 万条）
func WebVpnAuditExport(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	search := dbdata.WebVpnAuditSearch{
		Username: r.FormValue("username"),
		AppName:  r.FormValue("app_name"),
		Method:   r.FormValue("method"),
	}
	if d := r.FormValue("start_date"); d != "" {
		end := r.FormValue("end_date")
		if end == "" {
			end = time.Now().Format("2006-01-02 15:04:05")
		}
		search.Date = []string{d, end}
	}

	var datas []dbdata.WebVpnAudit
	count, err := dbdata.WebVpnAuditExportCount(search)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if count == 0 {
		RespError(w, RespParamErr, "你导出的总数量为0条，请调整搜索条件，再导出")
		return
	}
	if count > 1000000 {
		RespError(w, RespParamErr, "你导出的数据量超过100万条，请调整搜索条件，再导出")
		return
	}
	datas, err = dbdata.WebVpnAuditExportList(search)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("WebVPN应用管理", "审计日志", "导出了WebVPN访问审计日志", r.RemoteAddr)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=webvpn_audit.csv")
	gocsv.Marshal(datas, w)
}
