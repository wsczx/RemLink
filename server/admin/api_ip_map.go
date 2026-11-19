package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/wsczx/remlink/dbdata"
)

func UserIpMapList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	page, _ := parsePaging(r)

	var pageSize = dbdata.PageSize

	// 筛选条件
	conditions := []string{}
	args := []any{}
	if v := strings.TrimSpace(r.FormValue("ip")); v != "" {
		conditions = append(conditions, "ip_addr LIKE ?")
		args = append(args, "%"+v+"%")
	}
	if v := strings.TrimSpace(r.FormValue("mac")); v != "" {
		conditions = append(conditions, "mac_addr LIKE ?")
		args = append(args, "%"+v+"%")
	}
	if v := strings.TrimSpace(r.FormValue("username")); v != "" {
		fuzzy := "%" + v + "%"
		conditions = append(conditions, "(username LIKE ? OR username IN (SELECT username FROM "+dbdata.QuoteIdent("user")+" WHERE nickname LIKE ?))")
		args = append(args, fuzzy, fuzzy)
	}
	if v := strings.TrimSpace(r.FormValue("group")); v != "" {
		conditions = append(conditions, "ip_group LIKE ?")
		args = append(args, "%"+v+"%")
	}
	if v := r.FormValue("keep"); v == "1" {
		conditions = append(conditions, "keep = ?")
		args = append(args, true)
	} else if v == "0" {
		conditions = append(conditions, "keep = ?")
		args = append(args, false)
	}

	var where string
	if len(conditions) > 0 {
		where = strings.Join(conditions, " AND ")
	}

	datas, count, ok := findPage(w, &dbdata.IpMap{}, page, pageSize, where, args...)
	if !ok {
		return
	}

	names := make([]string, 0, len(datas))
	for _, d := range datas {
		names = append(names, d.Username)
	}
	nickMap := dbdata.Users.NicknameMap(names)
	for i := range datas {
		datas[i].Nickname = nickMap[datas[i].Username]
	}

	listResp(w, count, pageSize, datas)
}

// 批量删除 IP 映射（按选中的 ID 列表）
func UserIpMapBatchDel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ids []int `json:"ids"`
	}
	if err := readJSON(w, r, 1<<16, &req); err != nil {
		return
	}

	if len(req.Ids) == 0 {
		RespError(w, RespParamErr, "请选择要删除的 IP 映射")
		return
	}

	successCount, failCount := 0, 0
	for _, id := range req.Ids {
		var data dbdata.IpMap
		if err := dbdata.IpMaps.GetByID(id, &data); err != nil {
			failCount++
			continue
		}
		if err := dbdata.IpMaps.Delete(id); err != nil {
			failCount++
			continue
		}
		successCount++
	}

	msg := fmt.Sprintf("批量删除完成，成功：%d，失败：%d", successCount, failCount)
	dbdata.AdminLogs.AdminLog("IP映射管理", "批量删除", fmt.Sprintf("批量删除了%d个IP映射", successCount), r.RemoteAddr)

	if successCount > 0 {
		RespSuccess(w, msg)
	} else {
		RespError(w, RespInternalErr, errors.New(msg))
	}
}

func UserIpMapDetail(w http.ResponseWriter, r *http.Request) {
	detailByID[dbdata.IpMap](w, r, "用户名错误")
}

func UserIpMapSet(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()

	v := &dbdata.IpMap{}
	if err := readJSON(w, r, 1<<16, v); err != nil {
		return
	}

	if err := dbdata.IpMaps.SetIpMap(v); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLogs.AdminLog("IP映射管理", v.IpAddr, "创建/修改了IP映射", r.RemoteAddr)
	RespSuccess(w, nil)
}

func UserIpMapDel(w http.ResponseWriter, r *http.Request) {
	delByID(w, r, "IP映射id错误", func(data *dbdata.IpMap) {
		dbdata.AdminLogs.AdminLog("IP映射管理", data.IpAddr, "删除了IP映射", r.RemoteAddr)
	})
}
