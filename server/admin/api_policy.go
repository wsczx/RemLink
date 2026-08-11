package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/wsczx/remlink/dbdata"
)

// 返回所有策略的分页列表
func PolicyList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	pageS := r.FormValue("page")
	page, _ := strconv.Atoi(pageS)
	if page < 1 {
		page = 1
	}

	pageSizeS := r.FormValue("page_size")
	pageSize, _ := strconv.Atoi(pageSizeS)
	if pageSize <= 0 {
		pageSize = dbdata.PageSize
	}

	count := dbdata.CountAll(&dbdata.Policy{})

	var datas []dbdata.Policy
	err := dbdata.Find(&datas, pageSize, page)
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}

	// 确保空结果返回 [] 而非 null
	if datas == nil {
		datas = []dbdata.Policy{}
	}

	// 为每个策略附加引用统计
	type PolicyWithStats struct {
		dbdata.Policy
		GroupCount int `json:"group_count"`
		UserCount  int `json:"user_count"`
	}
	result := make([]PolicyWithStats, len(datas))
	for i, p := range datas {
		result[i] = PolicyWithStats{
			Policy:     p,
			GroupCount: dbdata.FindWhereCount(&dbdata.Group{}, "policy_id=?", p.Id),
			UserCount:  dbdata.FindWhereCount(&dbdata.User{}, "policy_id=?", p.Id),
		}
	}

	// 统计卡片按"全量数据"聚合，不受分页影响
	var all []dbdata.Policy
	_ = dbdata.Find(&all, 0, 0)
	statActive, statFakeDNS, statBandwidth, statQuota := 0, 0, 0, 0
	for _, p := range all {
		if p.Status == 1 {
			statActive++
		}
		if p.EnableFakeDNS {
			statFakeDNS++
		}
		if p.Bandwidth > 0 || p.BandwidthUp > 0 {
			statBandwidth++
		}
		if p.TrafficQuota > 0 {
			statQuota++
		}
	}

	data := map[string]any{
		"count":         count,
		"page_size":     pageSize,
		"datas":         result,
		"stats_active":  statActive,
		"stats_fakedns": statFakeDNS,
		"stats_band":    statBandwidth,
		"stats_quota":   statQuota,
	}

	RespSucess(w, data)
}

// 返回已启用策略的 ID-名称列表（供用户/组编辑下拉选择）
func PolicyNames(w http.ResponseWriter, r *http.Request) {
	names := dbdata.GetPolicyNames()
	if names == nil {
		names = []dbdata.GroupNameId{}
	}
	data := map[string]any{
		"count":     len(names),
		"page_size": 0,
		"datas":     names,
	}
	RespSucess(w, data)
}

// 返回全部策略的 ID-名称列表（含停用，供管理列表筛选使用）
func AllPolicyNames(w http.ResponseWriter, r *http.Request) {
	names := dbdata.GetAllPolicyNames()
	if names == nil {
		names = []dbdata.GroupNameId{}
	}
	data := map[string]any{
		"count":     len(names),
		"page_size": 0,
		"datas":     names,
	}
	RespSucess(w, data)
}

// 获取单个策略的详细信息
func PolicyDetail(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)
	if id < 1 {
		RespError(w, RespParamErr, "Id错误")
		return
	}

	var data dbdata.Policy
	err := dbdata.One("Id", id, &data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	RespSucess(w, data)
}

// 新增或更新策略配置
func PolicySet(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<23) // 8MB
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()
	v := &dbdata.Policy{}
	err = json.Unmarshal(body, v)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	err = dbdata.SetPolicy(v)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	dbdata.AdminLog("策略管理", v.Name, "创建/修改了策略", r.RemoteAddr)
	RespSucess(w, nil)
}

// 删除指定策略（需检查是否被引用）
func PolicyDel(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)
	if id < 1 {
		RespError(w, RespParamErr, "Id错误")
		return
	}

	// 检查是否被用户组引用
	groupCount := dbdata.FindWhereCount(&dbdata.Group{}, "policy_id=?", id)
	if groupCount > 0 {
		RespError(w, RespParamErr, "该策略被 "+strconv.Itoa(groupCount)+" 个用户组引用，无法删除")
		return
	}

	// 检查是否被用户引用
	userCount := dbdata.FindWhereCount(&dbdata.User{}, "policy_id=?", id)
	if userCount > 0 {
		RespError(w, RespParamErr, "该策略被 "+strconv.Itoa(userCount)+" 个用户引用，无法删除")
		return
	}

	p := &dbdata.Policy{}
	dbdata.One("Id", id, p)

	data := dbdata.Policy{Id: id}
	err := dbdata.Del(&data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("策略管理", p.Name, "删除了策略", r.RemoteAddr)
	RespSucess(w, nil)
}

// 复制策略
func PolicyCopy(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)
	if id < 1 {
		RespError(w, RespParamErr, "Id错误")
		return
	}

	var orig dbdata.Policy
	err := dbdata.One("Id", id, &orig)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	// 创建副本
	copy := orig
	copy.Id = 0
	copy.Name = orig.Name + " (副本)"
	err = dbdata.SetPolicy(&copy)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	dbdata.AdminLog("策略管理", orig.Name, "复制了策略→"+copy.Name, r.RemoteAddr)
	RespSucess(w, copy)
}

// 将策略反向应用到指定用户组
func PolicyApplyToGroups(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		PolicyId int   `json:"policy_id"`
		GroupIds []int `json:"group_ids"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	req := &Req{}
	if err := json.Unmarshal(body, req); err != nil {
		RespError(w, RespParamErr, "参数错误")
		return
	}
	if req.PolicyId < 1 {
		RespError(w, RespParamErr, "策略ID错误")
		return
	}
	if len(req.GroupIds) == 0 {
		RespError(w, RespParamErr, "请选择要应用的用户组")
		return
	}

	// 验证策略存在且已启用
	var p dbdata.Policy
	if err := dbdata.One("Id", req.PolicyId, &p); err != nil {
		RespError(w, RespParamErr, "策略不存在")
		return
	}
	if p.Status != 1 {
		RespError(w, RespParamErr, "策略已停用，无法应用到用户组")
		return
	}

	// 批量更新
	successCount := 0
	for _, gid := range req.GroupIds {
		var g dbdata.Group
		if err := dbdata.One("Id", gid, &g); err != nil {
			continue
		}
		g.PolicyId = req.PolicyId
		if err := dbdata.Set(&g); err != nil {
			RespError(w, RespInternalErr, "更新组 "+g.Name+" 失败: "+err.Error())
			return
		}
		successCount++
	}

	dbdata.AdminLog("策略管理", p.Name, "将策略应用到"+strconv.Itoa(successCount)+"个用户组", r.RemoteAddr)
	RespSucess(w, "已成功应用到 "+strconv.Itoa(successCount)+" 个用户组")
}

// 将策略反向应用到指定用户
func PolicyApplyToUsers(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		PolicyId int   `json:"policy_id"`
		UserIds  []int `json:"user_ids"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	req := &Req{}
	if err := json.Unmarshal(body, req); err != nil {
		RespError(w, RespParamErr, "参数错误")
		return
	}
	if req.PolicyId < 1 {
		RespError(w, RespParamErr, "策略ID错误")
		return
	}
	if len(req.UserIds) == 0 {
		RespError(w, RespParamErr, "请选择要应用的用户")
		return
	}

	// 验证策略存在且已启用
	var p dbdata.Policy
	if err := dbdata.One("Id", req.PolicyId, &p); err != nil {
		RespError(w, RespParamErr, "策略不存在")
		return
	}
	if p.Status != 1 {
		RespError(w, RespParamErr, "策略已停用，无法应用到用户")
		return
	}

	// 批量更新
	successCount := 0
	for _, uid := range req.UserIds {
		var u dbdata.User
		if err := dbdata.One("Id", uid, &u); err != nil {
			continue
		}
		u.PolicyId = req.PolicyId
		if err := dbdata.Set(&u); err != nil {
			RespError(w, RespInternalErr, "更新用户 "+u.Username+" 失败: "+err.Error())
			return
		}
		successCount++
	}

	dbdata.AdminLog("策略管理", p.Name, "将策略应用到"+strconv.Itoa(successCount)+"个用户", r.RemoteAddr)
	RespSucess(w, "已成功应用到 "+strconv.Itoa(successCount)+" 个用户")
}

// 查询策略被哪些组/用户引用
func PolicyUsedBy(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)
	if id < 1 {
		RespError(w, RespParamErr, "Id错误")
		return
	}

	groups := dbdata.PolicyUsedByGroups(id)
	users := dbdata.PolicyUsedByUsers(id)

	data := map[string]any{
		"groups": groups,
		"users":  users,
	}
	RespSucess(w, data)
}
