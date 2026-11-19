package admin

import (
	"encoding/json"
	"net/http"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/service"
)

// 返回所有用户组的分页列表
func GroupList(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePaging(r)

	datas, count, ok := findPage(w, &dbdata.Group{}, page, pageSize, "")
	if !ok {
		return
	}

	// 统计卡片按"全量数据"聚合，不受分页影响
	statActive := dbdata.Groups.CountActive()
	statWithPolicy := dbdata.Groups.CountWithPolicy()
	// 多因子认证需解析 AuthProfile，组表量小，仅拉组本身（不拉用户等大表）
	var all []dbdata.Group
	// 全量查询失败时统计按 0 计
	_ = dbdata.Groups.FindAll(&all)
	statWithAuth := 0
	for _, g := range all {
		if dbdata.Groups.HasMultiAuth(g.AuthProfile) {
			statWithAuth++
		}
	}

	data := map[string]any{
		"count":            count,
		"page_size":        pageSize,
		"datas":            datas,
		"stats_active":     statActive,
		"stats_policy":     statWithPolicy,
		"stats_multi_auth": statWithAuth,
	}

	RespSuccess(w, data)
}

// 返回所有用户组名称列表
func GroupNames(w http.ResponseWriter, r *http.Request) {
	names := dbdata.Groups.GetGroupNames()
	listResp(w, len(names), 0, names)
}

// 返回所有用户组的 ID-名称映射
func GroupNamesIds(w http.ResponseWriter, r *http.Request) {
	names := dbdata.Groups.GetGroupNamesIds()
	listResp(w, len(names), 0, names)
}

// 获取单个用户组的详细信息
func GroupDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "Id错误")
	if !ok {
		return
	}

	var data dbdata.Group
	err := dbdata.Groups.Get(id, &data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	// AuthProfile 为空时生成默认 Pipeline
	if len(data.AuthProfile) == 0 {
		data.AuthProfile = json.RawMessage(`{"step":[{"type":"local"}]}`)
	}
	if data.SplitDns == nil {
		data.SplitDns = []dbdata.ValData{}
	}

	// 附加策略信息
	type GroupDetailResp struct {
		dbdata.Group
		PolicyName string `json:"policy_name"`
	}
	resp := GroupDetailResp{Group: data}
	if data.PolicyId > 0 {
		var policy dbdata.Policy
		if err := dbdata.Policies.Get(data.PolicyId, &policy); err == nil {
			resp.PolicyName = policy.Name
		}
	}

	RespSuccess(w, resp)
}

// 新增或更新用户组配置
func GroupSet(w http.ResponseWriter, r *http.Request) {
	v := &dbdata.Group{}
	if err := readJSON(w, r, 1<<23, v); err != nil { // 8MB
		return
	}

	if err := service.SaveGroup(v); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLogs.AdminLog("用户组管理", v.Name, "创建/修改了用户组", r.RemoteAddr)
	RespSuccess(w, nil)
}

// 删除指定用户组
func GroupDel(w http.ResponseWriter, r *http.Request) {
	delByID(w, r, "Id错误", func(g *dbdata.Group) {
		service.DeleteGroup(g)
		dbdata.AdminLogs.AdminLog("用户组管理", g.Name, "删除了用户组", r.RemoteAddr)
	})
}

// 测试用户组认证配置
func (a *Api) GroupAuthLogin(w http.ResponseWriter, r *http.Request) {
	type AuthLoginData struct {
		Name        string          `json:"name"`
		Pwd         string          `json:"pwd"`
		AuthProfile json.RawMessage `json:"auth_profile"`
	}

	v := &AuthLoginData{}
	if err := readJSON(w, r, 1<<16, v); err != nil {
		return
	}
	// 接入防暴力破解：测试接口携带真实凭据，须受 LockManager 约束
	if !a.Locks.Check(v.Name, r.RemoteAddr) {
		RespError(w, RespParamErr, "尝试次数过多，请稍后再试")
		return
	}
	if err := dbdata.Groups.VerifyAuth(v.Name, v.Pwd, v.AuthProfile); err != nil {
		a.Locks.Fail(v.Name, r.RemoteAddr)
		RespError(w, RespInternalErr, err)
		return
	}
	a.Locks.Success(v.Name, r.RemoteAddr)
	dbdata.AdminLogs.AdminLog("用户组管理", v.Name, "测试了用户组认证登录", r.RemoteAddr)
	RespSuccess(w, "ok")
}

// 测试 Provider 的认证功能（LDAP/RADIUS）
func (a *Api) ProviderAuthLogin(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
		Pwd  string `json:"pwd"`
	}

	req := &Req{}
	if err := readJSON(w, r, 1<<16, req); err != nil {
		return
	}

	// 获取 Provider
	p := &dbdata.Provider{}
	if err := dbdata.Providers.GetByID(req.Id, p); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if p.Status != 1 {
		RespError(w, RespParamErr, "该认证源已停用")
		return
	}
	if p.Type != "ldap" && p.Type != "radius" {
		RespError(w, RespParamErr, "仅支持 LDAP/RADIUS 类型的测试登录")
		return
	}

	// 构建最小 Pipeline：仅包含当前 Provider
	profile := auth.GroupAuthProfile{
		Step: []auth.AuthMethodConfig{{
			Type:     p.Type,
			Provider: p.Name,
		}},
	}
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	// 接入防暴力破解：Provider 测试登录同样携带真实凭据
	if !a.Locks.Check(req.Name, r.RemoteAddr) {
		RespError(w, RespParamErr, "尝试次数过多，请稍后再试")
		return
	}
	if err := dbdata.Groups.VerifyAuth(req.Name, req.Pwd, profileJSON); err != nil {
		a.Locks.Fail(req.Name, r.RemoteAddr)
		RespError(w, RespInternalErr, err)
		return
	}
	a.Locks.Success(req.Name, r.RemoteAddr)
	dbdata.AdminLogs.AdminLog("Provider管理", p.Name, "测试了Provider认证登录("+p.Type+")", r.RemoteAddr)
	RespSuccess(w, "ok")
}

// 同步指定 Provider（LDAP/企微）的用户到本地
func ProviderSyncUsers(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		Id        int    `json:"id"`
		GroupName string `json:"group_name"`
	}

	req := &Req{}
	if err := readJSON(w, r, 1<<16, req); err != nil {
		return
	}

	// 获取 Provider
	p := &dbdata.Provider{}
	if err := dbdata.Providers.GetByID(req.Id, p); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if p.Type != "ldap" && p.Type != "wxwork" && p.Type != "feishu" && p.Type != "dingtalk" {
		RespError(w, RespParamErr, "仅支持 LDAP/企微/飞书/钉钉 类型的同步用户")
		return
	}

	g := &dbdata.Group{}
	if err := dbdata.Groups.GetByName(req.GroupName, g); err != nil {
		RespError(w, RespParamErr, "用户组不存在")
		return
	}
	if g.Status != 1 {
		RespError(w, RespParamErr, "用户组已停用")
		return
	}
	if !dbdata.Groups.UsesProvider(g, p.Name) {
		RespError(w, RespParamErr, "该用户组未引用此 Provider")
		return
	}

	if p.Type == "ldap" {
		authLdap, err := dbdata.Groups.ResolveLdapConfig(g)
		if err != nil {
			RespError(w, RespInternalErr, err)
			return
		}
		base.SafeGo(func() {
			if err := authLdap.SaveUsers(g); err != nil {
				base.Error("LDAP用户同步失败:", err)
			} else {
				base.Info("LDAP用户同步成功")
			}
		})
		dbdata.AdminLogs.AdminLog("用户组管理", req.GroupName, "同步LDAP用户", r.RemoteAddr)
		RespSuccess(w, "正在后台同步 LDAP 用户，请查看后台日志确认同步结果")
		return
	}

	// wxwork
	if p.Type == "wxwork" {
		authWx, err := dbdata.Groups.ResolveWxworkConfig(g)
		if err != nil {
			RespError(w, RespInternalErr, err)
			return
		}
		base.SafeGo(func() {
			if err := authWx.SaveUsers(g); err != nil {
				base.Error("企微用户同步失败, 组:", g.Name, ", err:", err)
			}
		})
		dbdata.AdminLogs.AdminLog("用户组管理", req.GroupName, "同步企微用户", r.RemoteAddr)
		RespSuccess(w, "正在后台同步企业微信用户，请查看后台日志确认同步结果")
		return
	}

	// dingtalk
	if p.Type == "dingtalk" {
		authDt, err := dbdata.Groups.ResolveDingtalkConfig(g)
		if err != nil {
			RespError(w, RespInternalErr, err)
			return
		}
		base.SafeGo(func() {
			if err := authDt.SaveUsers(g); err != nil {
				base.Error("钉钉用户同步失败, 组:", g.Name, ", err:", err)
			}
		})
		dbdata.AdminLogs.AdminLog("用户组管理", req.GroupName, "同步钉钉用户", r.RemoteAddr)
		RespSuccess(w, "正在后台同步钉钉用户，请查看后台日志确认同步结果")
		return
	}

	// feishu
	authFs, err := dbdata.Groups.ResolveFeishuConfig(g)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	base.SafeGo(func() {
		if err := authFs.SaveUsers(g); err != nil {
			base.Error("飞书用户同步失败, 组:", g.Name, ", err:", err)
		}
	})
	dbdata.AdminLogs.AdminLog("用户组管理", req.GroupName, "同步飞书用户", r.RemoteAddr)
	RespSuccess(w, "正在后台同步飞书用户，请查看后台日志确认同步结果")
}

// 检查指定组是否有已签发的客户端证书
// 返回证书数量，前端根据数量提示管理员是否适合配置证书认证
func GroupCertCheck(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	groupname := r.FormValue("groupname")
	if groupname == "" {
		RespError(w, RespParamErr, "组名称不能为空")
		return
	}

	count, err := dbdata.Certs.CountByGroupName(groupname)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	RespSuccess(w, map[string]any{
		"groupname":  groupname,
		"cert_count": count,
	})
}

// 检查指定组是否配置了证书认证步骤
// 用于生成客户端证书时提示管理员该组尚未启用证书认证
func GroupCertAuthCheck(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	groupname := r.FormValue("groupname")
	if groupname == "" {
		RespError(w, RespParamErr, "组名称不能为空")
		return
	}

	g, err := dbdata.Groups.ByName(groupname)
	if err != nil {
		if utils.CheckErrNotFound(err) {
			RespError(w, RespParamErr, "组不存在")
		} else {
			RespError(w, RespInternalErr, err)
		}
		return
	}

	hasCertAuth := dbdata.Groups.HasAuthType(g.AuthProfile, "cert")

	RespSuccess(w, map[string]any{
		"groupname":     groupname,
		"has_cert_auth": hasCertAuth,
	})
}

// 返回本机物理网卡列表，供前端组出网网卡(out_dev)下拉选择
func GroupIfaces(w http.ResponseWriter, r *http.Request) {
	ifaces := utils.GetPhysicalInterfaces()
	if ifaces == nil {
		ifaces = []string{}
	}
	RespSuccess(w, map[string]any{"ifaces": ifaces})
}
