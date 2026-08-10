package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

// 返回所有用户组的分页列表
func GroupList(w http.ResponseWriter, r *http.Request) {
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

	count := dbdata.CountAll(&dbdata.Group{})

	var datas []dbdata.Group
	err := dbdata.Find(&datas, pageSize, page)
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}

	// 确保空结果返回 [] 而非 null
	if datas == nil {
		datas = []dbdata.Group{}
	}

	data := map[string]any{
		"count":     count,
		"page_size": pageSize,
		"datas":     datas,
	}

	RespSucess(w, data)
}

// 返回所有用户组名称列表
func GroupNames(w http.ResponseWriter, r *http.Request) {
	var names = dbdata.GetGroupNames()
	if names == nil {
		names = []string{}
	}
	data := map[string]any{
		"count":     len(names),
		"page_size": 0,
		"datas":     names,
	}
	RespSucess(w, data)
}

// 返回所有用户组的 ID-名称映射
func GroupNamesIds(w http.ResponseWriter, r *http.Request) {
	var names = dbdata.GetGroupNamesIds()
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

// 获取单个用户组的详细信息
func GroupDetail(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)
	if id < 1 {
		RespError(w, RespParamErr, "Id错误")
		return
	}

	var data dbdata.Group
	err := dbdata.One("Id", id, &data)
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
		if err := dbdata.One("Id", data.PolicyId, &policy); err == nil {
			resp.PolicyName = policy.Name
		}
	}

	RespSucess(w, resp)
}

// 新增或更新用户组配置
func GroupSet(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<23) // 8MB
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()
	v := &dbdata.Group{}
	err = json.Unmarshal(body, v)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	// 仅当网段(v4/v6)发生变更时才需清理旧 NAT 规则。
	var oldGroup dbdata.Group
	if v.Id > 0 {
		dbdata.One("Id", v.Id, &oldGroup)
	}

	err = dbdata.SetGroup(v)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	// 组认证配置可能变更了 cert 步骤，立即使证书认证缓存失效（TLS 层下次握手即生效）
	dbdata.InvalidateCertAuthCache()

	// 网段变更则清理旧 NAT 规则
	if oldGroup.ClientCidr != "" && oldGroup.ClientCidr != v.ClientCidr {
		sessdata.RemoveGroupNAT(oldGroup.ClientCidr, "", oldGroup.OutDev)
	}
	if oldGroup.ClientCidr6 != "" && oldGroup.ClientCidr6 != v.ClientCidr6 {
		sessdata.RemoveGroupNAT("", oldGroup.ClientCidr6, oldGroup.OutDev)
	}

	// 组配置了外部认证 + OTP 时自动同步用户
	dbdata.SyncExternalUsersForOTP(v)

	dbdata.AdminLog("用户组管理", v.Name, "创建/修改了用户组", r.RemoteAddr)
	RespSucess(w, nil)
}

// 删除指定用户组
func GroupDel(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)
	if id < 1 {
		RespError(w, RespParamErr, "Id错误")
		return
	}

	// 先查出组名称用于审计
	g := &dbdata.Group{}
	dbdata.One("Id", id, g)

	data := dbdata.Group{Id: id}
	err := dbdata.Del(&data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	// 删除组后可能移除了最后一个 cert 组，立即使证书认证缓存失效
	dbdata.InvalidateCertAuthCache()

	// 清理该组在防火墙里的 NAT/转发规则，避免删除后规则残留至整机重启
	sessdata.RemoveGroupNAT(g.ClientCidr, g.ClientCidr6, g.OutDev)

	// 删除用户组后，组内成员重新签发 WebVPN 会话
	dbdata.WebVpnRevokeGroupMembers([]string{g.Name})

	// 同步清理各用户 Groups 字段里的已删组名，避免用户列表残留
	if err := dbdata.RemoveGroupFromUsers(g.Name); err != nil {
		base.Error(err)
	}

	dbdata.AdminLog("用户组管理", g.Name, "删除了用户组", r.RemoteAddr)
	RespSucess(w, nil)
}

// 测试用户组认证配置
func GroupAuthLogin(w http.ResponseWriter, r *http.Request) {
	type AuthLoginData struct {
		Name        string          `json:"name"`
		Pwd         string          `json:"pwd"`
		AuthProfile json.RawMessage `json:"auth_profile"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()
	v := &AuthLoginData{}
	err = json.Unmarshal(body, &v)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	// 接入防暴力破解：测试接口携带真实凭据，须受 LockManager 约束
	if !auth.GetLockManager().Check(v.Name, r.RemoteAddr) {
		RespError(w, RespParamErr, "尝试次数过多，请稍后再试")
		return
	}
	err = dbdata.GroupAuthLogin(v.Name, v.Pwd, v.AuthProfile)
	if err != nil {
		auth.GetLockManager().Fail(v.Name, r.RemoteAddr)
		RespError(w, RespInternalErr, err)
		return
	}
	auth.GetLockManager().Success(v.Name, r.RemoteAddr)
	dbdata.AdminLog("用户组管理", v.Name, "测试了用户组认证登录", r.RemoteAddr)
	RespSucess(w, "ok")
}

// 测试 Provider 的认证功能（LDAP/RADIUS）
func ProviderAuthLogin(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
		Pwd  string `json:"pwd"`
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

	// 获取 Provider
	p := &dbdata.Provider{}
	if err := dbdata.One("Id", req.Id, p); err != nil {
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
	if !auth.GetLockManager().Check(req.Name, r.RemoteAddr) {
		RespError(w, RespParamErr, "尝试次数过多，请稍后再试")
		return
	}
	if err := dbdata.GroupAuthLogin(req.Name, req.Pwd, profileJSON); err != nil {
		auth.GetLockManager().Fail(req.Name, r.RemoteAddr)
		RespError(w, RespInternalErr, err)
		return
	}
	auth.GetLockManager().Success(req.Name, r.RemoteAddr)
	dbdata.AdminLog("Provider管理", p.Name, "测试了Provider认证登录("+p.Type+")", r.RemoteAddr)
	RespSucess(w, "ok")
}

// 同步指定 Provider（LDAP/企微）的用户到本地
func ProviderSyncUsers(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		Id        int    `json:"id"`
		GroupName string `json:"group_name"`
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

	// 获取 Provider
	p := &dbdata.Provider{}
	if err := dbdata.One("Id", req.Id, p); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if p.Type != "ldap" && p.Type != "wxwork" && p.Type != "feishu" && p.Type != "dingtalk" {
		RespError(w, RespParamErr, "仅支持 LDAP/企微/飞书/钉钉 类型的同步用户")
		return
	}

	g := &dbdata.Group{}
	if err := dbdata.One("Name", req.GroupName, g); err != nil {
		RespError(w, RespParamErr, "用户组不存在")
		return
	}
	if g.Status != 1 {
		RespError(w, RespParamErr, "用户组已停用")
		return
	}
	if !dbdata.GroupUsesProvider(g, p.Name) {
		RespError(w, RespParamErr, "该用户组未引用此 Provider")
		return
	}

	if p.Type == "ldap" {
		authLdap, err := dbdata.ResolveLdapConfig(g)
		if err != nil {
			RespError(w, RespInternalErr, err)
			return
		}
		go func() {
			if err := authLdap.SaveUsers(g); err != nil {
				base.Error("LDAP用户同步失败:", err)
			} else {
				base.Info("LDAP用户同步成功")
			}
		}()
		dbdata.AdminLog("用户组管理", req.GroupName, "同步LDAP用户", r.RemoteAddr)
		RespSucess(w, "LDAP用户同步成功")
		return
	}

	// wxwork
	if p.Type == "wxwork" {
		authWx, err := dbdata.ResolveWxworkConfig(g)
		if err != nil {
			RespError(w, RespInternalErr, err)
			return
		}
		go func() {
			if err := authWx.SaveUsers(g); err != nil {
				base.Error("企微用户同步失败:", err)
			} else {
				base.Info("企微用户同步成功")
			}
		}()
		dbdata.AdminLog("用户组管理", req.GroupName, "同步企微用户", r.RemoteAddr)
		RespSucess(w, "企微用户同步成功")
		return
	}

	// dingtalk
	if p.Type == "dingtalk" {
		authDt, err := dbdata.ResolveDingtalkConfig(g)
		if err != nil {
			RespError(w, RespInternalErr, err)
			return
		}
		go func() {
			if err := authDt.SaveUsers(g); err != nil {
				base.Error("钉钉用户同步失败:", err)
			} else {
				base.Info("钉钉用户同步成功")
			}
		}()
		dbdata.AdminLog("用户组管理", req.GroupName, "同步钉钉用户", r.RemoteAddr)
		RespSucess(w, "钉钉用户同步成功")
		return
	}

	// feishu
	authFs, err := dbdata.ResolveFeishuConfig(g)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	go func() {
		if err := authFs.SaveUsers(g); err != nil {
			base.Error("飞书用户同步失败:", err)
		} else {
			base.Info("飞书用户同步成功")
		}
	}()
	dbdata.AdminLog("用户组管理", req.GroupName, "同步飞书用户", r.RemoteAddr)
	RespSucess(w, "飞书用户同步成功")
}

// GroupCertCheck 检查指定组是否有已签发的客户端证书
// 返回证书数量，前端根据数量提示管理员是否适合配置证书认证
func GroupCertCheck(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	groupname := r.FormValue("groupname")
	if groupname == "" {
		RespError(w, RespParamErr, "组名称不能为空")
		return
	}

	count, err := dbdata.CountCertsByGroupName(groupname)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	RespSucess(w, map[string]any{
		"groupname":  groupname,
		"cert_count": count,
	})
}

// GroupCertAuthCheck 检查指定组是否配置了证书认证步骤
// 用于生成客户端证书时提示管理员该组尚未启用证书认证
func GroupCertAuthCheck(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	groupname := r.FormValue("groupname")
	if groupname == "" {
		RespError(w, RespParamErr, "组名称不能为空")
		return
	}

	var g dbdata.Group
	ok, err := dbdata.GetXdb().Where("name = ?", groupname).Get(&g)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if !ok {
		RespError(w, RespParamErr, "组不存在")
		return
	}

	hasCertAuth := dbdata.HasAuthType(g.AuthProfile, "cert")

	RespSucess(w, map[string]any{
		"groupname":     groupname,
		"has_cert_auth": hasCertAuth,
	})
}

// 返回本机物理网卡列表，供前端组出网网卡(out_dev)下拉选择。
func GroupIfaces(w http.ResponseWriter, r *http.Request) {
	ifaces := utils.GetPhysicalInterfaces()
	if ifaces == nil {
		ifaces = []string{}
	}
	RespSucess(w, map[string]any{"ifaces": ifaces})
}
