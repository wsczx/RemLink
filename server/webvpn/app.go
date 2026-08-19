package webvpn

import (
	"slices"

	"github.com/wsczx/remlink/dbdata"
)

// 管理 WebVPN 应用配置的进程内缓存与授权判断
// 进程内缓存由 dbdata 层统一维护，这里不再自建缓存
// 避免双层缓存 TTL 叠加导致配置变更长期不生效。
type AppStore struct{}

// 构造应用存储（缓存委托 dbdata 层）
func NewAppStore() *AppStore {
	return &AppStore{}
}

// 按子域名前缀（Name）查找应用（带 dbdata 层进程内缓存）
func (s *AppStore) GetByName(name string) (*dbdata.WebVpnApp, error) {
	return dbdata.GetWebVpnAppByName(name)
}

// 主动失效缓存（配置变更后调用）
func (s *AppStore) Invalidate() {
	dbdata.InvalidateWebVpnAppCache()
}

// 返回某用户有权访问的启用中的应用（用户维度授权）
func (s *AppStore) AppsForUser(user *dbdata.User) ([]dbdata.WebVpnApp, error) {
	var apps []dbdata.WebVpnApp
	if err := dbdata.FindWhere(&apps, 0, 0, "status=1", nil); err != nil {
		return nil, err
	}
	userGroups := make(map[string]bool, len(user.Groups))
	for _, g := range user.Groups {
		userGroups[g] = true
	}
	var byGroup, byUser, fallback []dbdata.WebVpnApp
	for _, a := range apps {
		if len(a.Groups) == 0 {
			fallback = append(fallback, a)
			continue
		}
		matched := false
		for _, g := range a.Groups {
			if userGroups[g] {
				byGroup = append(byGroup, a)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if slices.Contains(a.Users, user.Username) {
			byUser = append(byUser, a)
		}
	}
	seen := make(map[int]bool)
	result := make([]dbdata.WebVpnApp, 0)
	for _, a := range append(append(byGroup, byUser...), fallback...) {
		if seen[a.Id] {
			continue
		}
		seen[a.Id] = true
		if !dbdata.WebVpnUserAllowed(&a, user) {
			continue
		}
		result = append(result, a)
	}
	return result, nil
}

// 判断用户是否有权访问某应用（用户维度 + 请求级来源 IP/路径）
// 具体 IP/路径校验由调用方在 proxy 阶段执行，这里只做用户维度兜底
func (s *AppStore) Authorized(app *dbdata.WebVpnApp, user *dbdata.User) bool {
	return dbdata.WebVpnUserAllowed(app, user)
}
