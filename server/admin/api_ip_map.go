package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/wsczx/remlink/dbdata"
)

func UserIpMapList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	pageS := r.FormValue("page")
	page, _ := strconv.Atoi(pageS)
	if page < 1 {
		page = 1
	}

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
		conditions = append(conditions, "username LIKE ?")
		args = append(args, "%"+v+"%")
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

	var count int
	if where != "" {
		count = dbdata.FindWhereCount(&dbdata.IpMap{}, where, args...)
	} else {
		count = dbdata.CountAll(&dbdata.IpMap{})
	}

	var datas []dbdata.IpMap
	var err error
	if where != "" {
		err = dbdata.FindWhere(&datas, pageSize, page, where, args...)
	} else {
		err = dbdata.Find(&datas, pageSize, page)
	}
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}

	// 确保空结果返回 [] 而非 null
	if datas == nil {
		datas = []dbdata.IpMap{}
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
		"page_size": pageSize,
		"datas":     datas,
	}

	RespSucess(w, data)
}

// 批量删除 IP 映射（按选中的 ID 列表）
func UserIpMapBatchDel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ids []int `json:"ids"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	if len(req.Ids) == 0 {
		RespError(w, RespParamErr, "请选择要删除的 IP 映射")
		return
	}

	successCount, failCount := 0, 0
	for _, id := range req.Ids {
		var data dbdata.IpMap
		if err := dbdata.One("Id", id, &data); err != nil {
			failCount++
			continue
		}
		if err := dbdata.Del(&data); err != nil {
			failCount++
			continue
		}
		successCount++
	}

	msg := fmt.Sprintf("批量删除完成，成功：%d，失败：%d", successCount, failCount)
	dbdata.AdminLog("IP映射管理", "批量删除", fmt.Sprintf("批量删除了%d个IP映射", successCount), r.RemoteAddr)

	if successCount > 0 {
		RespSucess(w, msg)
	} else {
		RespError(w, RespInternalErr, errors.New(msg))
	}
}

func UserIpMapDetail(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)
	if id < 1 {
		RespError(w, RespParamErr, "用户名错误")
		return
	}

	var data dbdata.IpMap
	err := dbdata.One("Id", id, &data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	RespSucess(w, data)
}

func UserIpMapSet(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()

	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()
	v := &dbdata.IpMap{}
	err = json.Unmarshal(body, v)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	err = dbdata.SetIpMap(v)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	dbdata.AdminLog("IP映射管理", v.IpAddr, "创建/修改了IP映射", r.RemoteAddr)
	RespSucess(w, nil)
}

func UserIpMapDel(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)

	if id < 1 {
		RespError(w, RespParamErr, "IP映射id错误")
		return
	}

	var data dbdata.IpMap
	err := dbdata.One("Id", id, &data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	err = dbdata.Del(&data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	dbdata.AdminLog("IP映射管理", data.IpAddr, "删除了IP映射", r.RemoteAddr)
	RespSucess(w, nil)
}
