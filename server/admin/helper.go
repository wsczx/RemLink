package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/wsczx/remlink/dbdata"
)

// 读取并解析 JSON 请求体（带大小上限）
// 调用方判错后直接 return，不再写响应
//
//	if err := readJSON(w, r, 1<<16, v); err != nil {
//	    return
//	}
func readJSON(w http.ResponseWriter, r *http.Request, maxLen int64, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxLen)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		RespError(w, RespParamErr, "参数错误")
		return err
	}
	return nil
}

// 解析表单 id 参数。非法时已写好错误响应，返回 (0, false)
//
//	id, ok := parseID(w, r, "Id错误")
//	if !ok {
//	    return
//	}
func parseID(w http.ResponseWriter, r *http.Request, errMsg string) (int, bool) {
	_ = r.ParseForm()
	id, _ := strconv.Atoi(r.FormValue("id"))
	if id < 1 {
		RespError(w, RespParamErr, errMsg)
		return 0, false
	}
	return id, true
}

// 解析分页参数（page/page_size），非法值回退默认
func parsePaging(r *http.Request) (page, pageSize int) {
	page, _ = strconv.Atoi(r.FormValue("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ = strconv.Atoi(r.FormValue("page_size"))
	if pageSize <= 0 {
		pageSize = dbdata.PageSize
	}
	return page, pageSize
}

// 按 id 查询单条记录并响应
func detailByID[T any](w http.ResponseWriter, r *http.Request, errMsg string) {
	id, ok := parseID(w, r, errMsg)
	if !ok {
		return
	}
	var data T
	if err := dbdata.One("Id", id, &data); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	RespSuccess(w, data)
}

// 按 id 删除记录：先查出完整记录（供 onDeleted 审计使用）
// 删除成功后执行 onDeleted 回调（可空，用于吊销/清理/审计等收尾动作）
func delByID[T any](w http.ResponseWriter, r *http.Request, errMsg string, onDeleted func(*T)) {
	id, ok := parseID(w, r, errMsg)
	if !ok {
		return
	}
	data := new(T)
	if err := dbdata.One("Id", id, data); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if err := dbdata.Del(data); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if onDeleted != nil {
		onDeleted(data)
	}
	RespSuccess(w, nil)
}
