package dbdata

import (
	"fmt"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
)

// AuthWXwork 企业微信认证配置（嵌入共享 WXWorkConfig）
type AuthWXwork struct {
	auth.WXWorkConfig
}

// 从组的 AuthProfile 中获取企微认证配置
func GetAuthWework(groupName string) (*AuthWXwork, error) {
	groupData := &Group{}
	if err := One("Name", groupName, groupData); err != nil {
		return nil, fmt.Errorf("用户组错误: %v", err)
	}
	return ResolveWxworkConfig(groupData)
}

// 从 Group 的 AuthProfile 解析企微配置
func ResolveWxworkConfig(g *Group) (*AuthWXwork, error) {
	cfg, err := resolveProviderConfigMap(g, "wxwork", "企微")
	if err != nil {
		return nil, err
	}

	result := &AuthWXwork{}
	if err := auth.GetProviderConfigFromMap(cfg, &result.WXWorkConfig); err != nil {
		return nil, err
	}
	if result.CorpID == "" || result.AgentID == "" || result.Secret == "" {
		return nil, fmt.Errorf("企微配置不完整")
	}
	return result, nil
}

// 从企业微信同步用户到本地数据库
func (a *AuthWXwork) SaveUsers(g *Group) error {
	accessToken, err := a.GetAccessToken()
	if err != nil {
		return fmt.Errorf("获取企微 access_token 失败: %w", err)
	}

	needOTP := HasAuthType(g.AuthProfile, "otp")
	blocked := a.ParseBlockedUserIDs()
	isBlocked := func(id string) bool { return a.CheckUserID(id, blocked) }

	// 拉取允许部门内的成员；未配置部门时拉取权限范围内全部用户
	departments := a.ParseDepartments()
	if len(departments) == 0 {
		base.Debug("企微未配置允许部门，同步权限范围内全部用户")
		departments = []int{1} // 企微根部门
	}
	remote := make(map[string]RemoteUser)
	for _, deptID := range departments {
		users, err := a.GetDepartmentUsers(accessToken, deptID)
		if err != nil {
			base.Error("获取部门成员失败", deptID, err)
			continue
		}
		for _, u := range users {
			if _, exists := remote[u.UserID]; exists {
				continue
			}
			if isBlocked(u.UserID) {
				remote[u.UserID] = RemoteUser{ID: u.UserID, Name: u.Name}
				continue
			}
			mobile, email := a.GetUserDetail(accessToken, u.UserID)
			remote[u.UserID] = RemoteUser{ID: u.UserID, Name: u.Name, Phone: mobile, Email: email}
		}
	}
	if len(remote) == 0 {
		return fmt.Errorf("企微拉取到的用户列表为空（可能部门配置错误、access_token 权限不足或应用可见范围未覆盖），组: %s", g.Name)
	}

	return syncExternalUsers(g, "wxwork", "企微", remote, needOTP, isBlocked, false)
}

// 同步所有企业微信认证源用户（按各企微认证源的 sync_users 独立控制）
func SyncWXworkUsers() {
	syncExternalUsersBySource(extSyncSource{
		authType: "wxwork",
		name:     "企微",
		resolve: func(g *Group) (bool, func(*Group) error, error) {
			cfg, err := ResolveWxworkConfig(g)
			if err != nil {
				return false, nil, err
			}
			return cfg.SyncUsers, cfg.SaveUsers, nil
		},
	})
}
