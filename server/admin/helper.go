package admin

import (
	"encoding/json"
	"github.com/wsczx/remlink/pkg/utils"
	"io"
	"net/http"
	"strconv"

	"github.com/wsczx/remlink/dbdata"
)

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

func parseID(w http.ResponseWriter, r *http.Request, errMsg string) (int, bool) {
	_ = r.ParseForm()
	id, _ := strconv.Atoi(r.FormValue("id"))
	if id < 1 {
		RespError(w, RespParamErr, errMsg)
		return 0, false
	}
	return id, true
}

func parsePaging(r *http.Request) (page, pageSize int) {
	page, _ = strconv.Atoi(r.FormValue("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ = strconv.Atoi(r.FormValue("page_size"))
	if pageSize <= 0 {
		pageSize = dbdata.PageSize
	}
	// 防止超大 page_size 把整表载入内存
	const maxPageSize = 200
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func detailByID[T any](w http.ResponseWriter, r *http.Request, errMsg string) {
	id, ok := parseID(w, r, errMsg)
	if !ok {
		return
	}
	var data T
	if err := dbdata.GetDB().Get("Id", id, &data); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	RespSuccess(w, &data)
}

// 先查出完整记录供可选的 onDeleted 回调使用
func delByID[T any](w http.ResponseWriter, r *http.Request, errMsg string, onDeleted func(*T)) {
	id, ok := parseID(w, r, errMsg)
	if !ok {
		return
	}
	s := dbdata.GetDB()
	var data T
	if err := s.Get("Id", id, &data); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if err := s.Delete(&data); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if onDeleted != nil {
		onDeleted(&data)
	}
	RespSuccess(w, nil)
}

// 统一分页列表响应：空结果转 []，返回 {count,page_size,datas}
func listResp[T any](w http.ResponseWriter, count, pageSize int, datas []T) {
	if datas == nil {
		datas = []T{}
	}
	RespSuccess(w, map[string]any{
		"count":     count,
		"page_size": pageSize,
		"datas":     datas,
	})
}

// 取当前页数据与总数；查询出错时已写错误响应并返回 false
// where 为空表示无条件查询
func findPage[T any](w http.ResponseWriter, m *T, page, pageSize int, where string, args ...any) ([]T, int, bool) {
	var count int
	var datas []T
	var err error
	s := dbdata.GetDB()
	if where != "" {
		count = s.CountWhere(m, where, args...)
		err = s.FindWhere(&datas, pageSize, page, where, args...)
	} else {
		count = s.Count(m)
		err = s.Find(&datas, pageSize, page)
	}
	if err != nil && !utils.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return nil, 0, false
	}
	if datas == nil {
		datas = []T{}
	}
	return datas, count, true
}
