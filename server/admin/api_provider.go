package admin

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/mask"
)

// 脱敏 Provider 配置中的敏感字段
func maskProviderSecrets(p *dbdata.Provider) {
	keys := dbdata.ProvSecretKeys(p.Type)
	if len(keys) == 0 || len(p.Config.Data) == 0 {
		return
	}
	var m map[string]any
	if err := json.Unmarshal(p.Config.Data, &m); err != nil {
		return
	}
	for _, k := range keys {
		if v, ok := m[k].(string); ok {
			m[k] = mask.Secret(v)
		}
	}
	if b, err := json.Marshal(m); err == nil {
		p.Config.Data = b
	}
}

// 前端回传占位符时保留数据库中的旧值
func keepProviderSecrets(newP, oldP *dbdata.Provider) {
	keys := dbdata.ProvSecretKeys(newP.Type)
	if len(keys) == 0 {
		return
	}
	var nm, om map[string]any
	if json.Unmarshal(newP.Config.Data, &nm) != nil || json.Unmarshal(oldP.Config.Data, &om) != nil {
		return
	}
	changed := false
	for _, k := range keys {
		s, _ := nm[k].(string)
		if mask.IsPlaceholder(s) {
			nm[k] = om[k]
			changed = true
		}
	}
	if changed {
		if b, err := json.Marshal(nm); err == nil {
			newP.Config.Data = b
		}
	}
}

// 列出所有 Provider
func ProviderList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	page, _ := parsePaging(r)
	pageSize := dbdata.PageSize

	datas, count, err := dbdata.ProviderList(pageSize, page)
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}

	if datas == nil {
		datas = []dbdata.Provider{}
	}

	for i := range datas {
		maskProviderSecrets(&datas[i])
	}

	// 统计卡片按"全量数据"聚合，不受分页影响
	statActive := dbdata.FindWhereCount(&dbdata.Provider{}, "status=1")

	RespSuccess(w, map[string]any{
		"count":        count,
		"page_size":    pageSize,
		"datas":        datas,
		"callbackBase": providerCallbackUrl(r),
		"stats_active": statActive,
	})
}

// 返回所有启用的 Provider 名称（可按类型过滤）
func ProviderNames(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	typ := r.FormValue("type")
	names := dbdata.ProviderNamesByType(typ)
	if names == nil {
		names = []string{}
	}
	RespSuccess(w, map[string]any{
		"count": len(names),
		"datas": names,
	})
}

// 获取单个 Provider 详情
func ProviderDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "Id错误")
	if !ok {
		return
	}

	var data dbdata.Provider
	err := dbdata.One("Id", id, &data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	maskProviderSecrets(&data)
	// 下发各第三方认证源的回调url
	RespSuccess(w, map[string]any{
		"provider":     data,
		"callbackBase": providerCallbackUrl(r),
	})
}

// 生成第三方认证源的回调Url
func providerCallbackUrl(r *http.Request) string {
	addr := base.GetCfg().ServerAddr
	host, port, err := net.SplitHostPort(base.FormatListenAddr(addr))
	if err != nil || port == "" {
		return "https://" + r.Host
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		reqHost, _, herr := net.SplitHostPort(r.Host)
		if herr != nil {
			reqHost = r.Host
		}
		host = reqHost
	}
	if port == "443" {
		return "https://" + host
	}
	return "https://" + net.JoinHostPort(host, port)
}

// 新增或更新 Provider
func ProviderSet(w http.ResponseWriter, r *http.Request) {
	v := &dbdata.Provider{}
	if err := readJSON(w, r, 1<<16, v); err != nil {
		return
	}

	// 更新时保留未修改的敏感字段
	if v.Id > 0 {
		old := &dbdata.Provider{}
		if err := dbdata.One("Id", v.Id, old); err == nil {
			keepProviderSecrets(v, old)
		}
	}

	if err := dbdata.SetProvider(v); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	dbdata.AdminLog("Provider管理", v.Name, "创建/修改了认证源", r.RemoteAddr)
	RespSuccess(w, nil)
}

// 删除 Provider
func ProviderDel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "Id错误")
	if !ok {
		return
	}

	// 检查是否被引用
	p := &dbdata.Provider{}
	if err := dbdata.One("Id", id, p); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if dbdata.CheckProviderInUse(p.Name) {
		RespError(w, RespParamErr, "该 Provider 正被认证流程引用，无法删除")
		return
	}

	if err := dbdata.DelProvider(id); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("Provider管理", p.Name, "删除了认证源", r.RemoteAddr)
	RespSuccess(w, nil)
}
