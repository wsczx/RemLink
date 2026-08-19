package webvpn

import (
	"slices"

	"github.com/wsczx/remlink/dbdata"
)

// 应用配置存储。缓存委托 dbdata 层维护，避免双层 TTL 叠加导致配置变更不生效。
type AppStore struct{}

func NewAppStore() *AppStore {
	return &AppStore{}
}

// 按子域名前缀（Name）查找应用。
func (s *AppStore) GetByName(name string) (*dbdata.WebVpnApp, error) {
	return dbdata.GetWebVpnAppByName(name)
}

// 主动失效缓存（配置变更后调用）。
func (s *AppStore) Invalidate() {
	dbdata.InvalidateWebVpnAppCache()
}

// 返回用户有权访问的启用中应用（用户维度授权）。
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

// 用户维度授权兜底；IP/路径校验由 proxy 阶段执行。
func (s *AppStore) Authorized(app *dbdata.WebVpnApp, user *dbdata.User) bool {
	return dbdata.WebVpnUserAllowed(app, user)
}
