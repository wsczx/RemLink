package service

import (
	"encoding/json"
	"fmt"
	"github.com/wsczx/remlink/pkg/utils"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 按用户名载入门户登录用户，返回 (用户, 记录是否存在, error)
// 记录不存在不是错误：外部认证用户首次登录时由 ResolvePortalUser 投影
func LoadPortalUser(username string) (*dbdata.User, bool, error) {
	user := &dbdata.User{}
	if err := dbdata.Users.GetByUsername(username, user); err != nil {
		if utils.CheckErrNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("用户查询失败")
	}
	switch user.Status {
	case 1:
		return user, true, nil
	case 2:
		return nil, true, fmt.Errorf("用户已过期")
	default:
		return nil, true, fmt.Errorf("用户不存在或已停用")
	}
}

// 按用户类型筛出可登录的候选组
// 外部认证类型只匹配对应认证方式；本地用户只匹配含 local 的组；
// 用户不存在（外部用户首登）时允许匹配 ldap/radius 组
func portalCandidateGroups(user *dbdata.User, userExists bool) []string {
	groups, err := dbdata.Groups.GetAllGroups()
	if err != nil {
		base.Error("用户门户获取用户组失败:", err)
		return nil
	}

	groupMap := make(map[string]json.RawMessage, len(groups))
	for _, g := range groups {
		groupMap[g.Name] = g.AuthProfile
	}

	candidateNames := user.Groups
	if !userExists || len(candidateNames) == 0 {
		candidateNames = make([]string, 0, len(groups))
		for _, g := range groups {
			candidateNames = append(candidateNames, g.Name)
		}
	}

	names := make([]string, 0, len(candidateNames))
	for _, name := range candidateNames {
		profile := groupMap[name]
		switch user.Type {
		case "ldap":
			if dbdata.Groups.HasAuthType(profile, "ldap") {
				names = append(names, name)
			}
		case "radius":
			if dbdata.Groups.HasAuthType(profile, "radius") {
				names = append(names, name)
			}
		default:
			if !userExists {
				if dbdata.Groups.HasAuthType(profile, "ldap") || dbdata.Groups.HasAuthType(profile, "radius") {
					names = append(names, name)
				}
			} else if dbdata.Groups.HasAuthType(profile, "local") {
				names = append(names, name)
			}
		}
	}
	return names
}

type PortalLoginPrecheck struct {
	User       *dbdata.User
	UserExists bool
	Groups     []string
}

// 载入用户并计算候选组；不存在时给出空壳对象供管道尝试外部认证
func PreparePortalLogin(username string) (*PortalLoginPrecheck, error) {
	user, exists, err := LoadPortalUser(username)
	if err != nil {
		return nil, err
	}
	if !exists {
		user = &dbdata.User{Username: username}
	}
	return &PortalLoginPrecheck{
		User:       user,
		UserExists: exists,
		Groups:     portalCandidateGroups(user, exists),
	}, nil
}

// 外部用户首次登录时投影成临时用户对象（不落库）
func ResolvePortalUser(username, group string, user *dbdata.User, userExists bool) (*dbdata.User, error) {
	if userExists {
		return user, nil
	}
	return &dbdata.User{
		Type:     "external",
		Username: username,
		Groups:   []string{group},
		Status:   1,
	}, nil
}
