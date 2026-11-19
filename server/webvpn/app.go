package webvpn

import (
	"slices"

	"github.com/wsczx/remlink/dbdata"
)

type AppStore struct{}

func NewAppStore() *AppStore {
	return &AppStore{}
}

// 按名称查找应用
func (s *AppStore) GetByName(name string) (*dbdata.WebVpnApp, error) {
	return dbdata.WebVpn.GetAppByName(name)
}

// 配置变更后失效缓存
func (s *AppStore) Invalidate() {
	dbdata.WebVpn.InvalidateAppCache()
}

// 返回用户可访问的启用应用
func (s *AppStore) ForUser(user *dbdata.User) ([]dbdata.WebVpnApp, error) {
	apps, err := dbdata.WebVpn.EnabledApps()
	if err != nil {
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
		if !dbdata.WebVpn.UserAllowed(&a, user) {
			continue
		}
		result = append(result, a)
	}
	return result, nil
}
