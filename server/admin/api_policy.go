package admin

import (
	"net/http"
	"strconv"

	"github.com/wsczx/remlink/dbdata"
)

// 返回所有策略的分页列表
func PolicyList(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePaging(r)
	datas, count, ok := findPage(w, &dbdata.Policy{}, page, pageSize, "")
	if !ok {
		return
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
			GroupCount: dbdata.Groups.CountByPolicy(p.Id),
			UserCount:  dbdata.Users.CountByPolicy(p.Id),
		}
	}

	// 统计卡片按"全量数据"聚合，不受分页影响
	statActive := dbdata.Policies.CountActive()
	var all []dbdata.Policy
	// 全量查询失败时统计按 0 计
	_ = dbdata.Policies.FindAll(&all)
	statFakeDNS, statBandwidth, statQuota := 0, 0, 0
	for _, p := range all {
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

	RespSuccess(w, data)
}

// 返回已启用策略的 ID-名称列表（供用户/组编辑下拉选择）
func PolicyNames(w http.ResponseWriter, r *http.Request) {
	names := dbdata.Policies.GetPolicyNames()
	listResp(w, len(names), 0, names)
}

// 返回全部策略的 ID-名称列表（含停用，供管理列表筛选使用）
func AllPolicyNames(w http.ResponseWriter, r *http.Request) {
	names := dbdata.Policies.GetAllPolicyNames()
	listResp(w, len(names), 0, names)
}

// 获取单个策略的详细信息
func PolicyDetail(w http.ResponseWriter, r *http.Request) {
	detailByID[dbdata.Policy](w, r, "Id错误")
}

// 新增或更新策略配置
func PolicySet(w http.ResponseWriter, r *http.Request) {
	v := &dbdata.Policy{}
	if err := readJSON(w, r, 1<<23, v); err != nil { // 8MB
		return
	}
	if err := dbdata.Policies.SetPolicy(v); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLogs.AdminLog("策略管理", v.Name, "创建/修改了策略", r.RemoteAddr)
	RespSuccess(w, nil)
}

// 删除指定策略（需检查是否被引用）
func PolicyDel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "Id错误")
	if !ok {
		return
	}

	// 检查是否被用户组引用
	groupCount := dbdata.Groups.CountByPolicy(id)
	if groupCount > 0 {
		RespError(w, RespParamErr, "该策略被 "+strconv.Itoa(groupCount)+" 个用户组引用，无法删除")
		return
	}

	// 检查是否被用户引用
	userCount := dbdata.Users.CountByPolicy(id)
	if userCount > 0 {
		RespError(w, RespParamErr, "该策略被 "+strconv.Itoa(userCount)+" 个用户引用，无法删除")
		return
	}

	p := &dbdata.Policy{}
	dbdata.Policies.Get(id, p)

	if err := dbdata.Policies.Delete(id); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLogs.AdminLog("策略管理", p.Name, "删除了策略", r.RemoteAddr)
	RespSuccess(w, nil)
}

// 复制策略
func PolicyCopy(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "Id错误")
	if !ok {
		return
	}

	var orig dbdata.Policy
	err := dbdata.Policies.Get(id, &orig)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	// 创建副本
	copy := orig
	copy.Id = 0
	copy.Name = orig.Name + " (副本)"
	err = dbdata.Policies.SetPolicy(&copy)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	dbdata.AdminLogs.AdminLog("策略管理", orig.Name, "复制了策略→"+copy.Name, r.RemoteAddr)
	RespSuccess(w, copy)
}

// 将策略反向应用到指定用户组
func PolicyApplyToGroups(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		PolicyId int   `json:"policy_id"`
		GroupIds []int `json:"group_ids"`
	}
	req := &Req{}
	if err := readJSON(w, r, 1<<16, req); err != nil {
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

	p, err := dbdata.Policies.GetByID(req.PolicyId)
	if err != nil {
		RespError(w, RespParamErr, err.Error())
		return
	}
	if p.Status != 1 {
		RespError(w, RespParamErr, "策略已停用，无法应用到用户组")
		return
	}
	successCount, err := dbdata.Groups.SetPolicy(req.GroupIds, req.PolicyId)
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}

	dbdata.AdminLogs.AdminLog("策略管理", p.Name, "将策略应用到"+strconv.Itoa(successCount)+"个用户组", r.RemoteAddr)
	RespSuccess(w, "已成功应用到 "+strconv.Itoa(successCount)+" 个用户组")
}

// 将策略反向应用到指定用户
func PolicyApplyToUsers(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		PolicyId int   `json:"policy_id"`
		UserIds  []int `json:"user_ids"`
	}
	req := &Req{}
	if err := readJSON(w, r, 1<<16, req); err != nil {
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

	p, err := dbdata.Policies.GetByID(req.PolicyId)
	if err != nil {
		RespError(w, RespParamErr, err.Error())
		return
	}
	if p.Status != 1 {
		RespError(w, RespParamErr, "策略已停用，无法应用到用户")
		return
	}
	successCount, err := dbdata.Users.SetPolicy(req.UserIds, req.PolicyId)
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}

	dbdata.AdminLogs.AdminLog("策略管理", p.Name, "将策略应用到"+strconv.Itoa(successCount)+"个用户", r.RemoteAddr)
	RespSuccess(w, "已成功应用到 "+strconv.Itoa(successCount)+" 个用户")
}

// 查询策略被哪些组/用户引用
func PolicyUsedBy(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "Id错误")
	if !ok {
		return
	}

	groups := dbdata.Policies.UsedByGroups(id)
	users := dbdata.Policies.UsedByUsers(id)

	data := map[string]any{
		"groups": groups,
		"users":  users,
	}
	RespSuccess(w, data)
}

// 将指定用户组从该策略移出（仅清除当前引用本策略的组）
func PolicyRemoveFromGroups(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		PolicyId int   `json:"policy_id"`
		GroupIds []int `json:"group_ids"`
	}
	req := &Req{}
	if err := readJSON(w, r, 1<<16, req); err != nil {
		return
	}
	if req.PolicyId < 1 {
		RespError(w, RespParamErr, "策略ID错误")
		return
	}
	if len(req.GroupIds) == 0 {
		RespError(w, RespParamErr, "请选择要移出的用户组")
		return
	}

	p, err := dbdata.Policies.GetByID(req.PolicyId)
	if err != nil {
		RespError(w, RespParamErr, err.Error())
		return
	}
	// 仅清除当前确实引用本策略的组，避免误清引用其他策略的组
	successCount, err := dbdata.Groups.ClearPolicy(req.GroupIds, req.PolicyId)
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}

	dbdata.AdminLogs.AdminLog("策略管理", p.Name, "将"+strconv.Itoa(successCount)+"个用户组移出策略", r.RemoteAddr)
	RespSuccess(w, "已成功移出 "+strconv.Itoa(successCount)+" 个用户组")
}

// 将指定用户从该策略移出（仅清除当前引用本策略的用户）
func PolicyRemoveFromUsers(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		PolicyId int   `json:"policy_id"`
		UserIds  []int `json:"user_ids"`
	}
	req := &Req{}
	if err := readJSON(w, r, 1<<16, req); err != nil {
		return
	}
	if req.PolicyId < 1 {
		RespError(w, RespParamErr, "策略ID错误")
		return
	}
	if len(req.UserIds) == 0 {
		RespError(w, RespParamErr, "请选择要移出的用户")
		return
	}

	p, err := dbdata.Policies.GetByID(req.PolicyId)
	if err != nil {
		RespError(w, RespParamErr, err.Error())
		return
	}
	// 仅清除当前确实引用本策略的用户，避免误清引用其他策略的用户
	successCount, err := dbdata.Users.ClearPolicy(req.UserIds, req.PolicyId)
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}

	dbdata.AdminLogs.AdminLog("策略管理", p.Name, "将"+strconv.Itoa(successCount)+"个用户移出策略", r.RemoteAddr)
	RespSuccess(w, "已成功移出 "+strconv.Itoa(successCount)+" 个用户")
}
