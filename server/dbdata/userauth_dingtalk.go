package dbdata

import (
	"fmt"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
)

type AuthDingtalk struct {
	auth.DingtalkConfig
}

// 从组的 AuthProfile 中获取钉钉认证配置
func GetAuthDingtalk(groupName string) (*AuthDingtalk, error) {
	groupData := &Group{}
	if err := One("Name", groupName, groupData); err != nil {
		return nil, fmt.Errorf("用户组错误: %v", err)
	}
	return ResolveDingtalkConfig(groupData)
}

// 从 Group 的 AuthProfile 解析钉钉配置
func ResolveDingtalkConfig(g *Group) (*AuthDingtalk, error) {
	cfg, err := resolveProviderConfigMap(g, "dingtalk", "钉钉")
	if err != nil {
		return nil, err
	}

	result := &AuthDingtalk{}
	if err := auth.GetProviderConfigFromMap(cfg, &result.DingtalkConfig); err != nil {
		return nil, err
	}
	if result.ClientID == "" || result.ClientSecret == "" {
		return nil, fmt.Errorf("钉钉配置不完整")
	}
	return result, nil
}

// 从钉钉同步用户到本地数据库
func (a *AuthDingtalk) SaveUsers(g *Group) error {
	contactToken, err := a.GetContactToken()
	if err != nil {
		return fmt.Errorf("获取钉钉通讯录令牌失败: %w", err)
	}

	needOTP := HasAuthType(g.AuthProfile, "otp")
	blocked := a.ParseBlockedUserIDs()
	isBlocked := func(id string) bool { return a.CheckUserID(id, blocked) != nil }

	// 拉取允许部门内的成员（含子部门）；未配置部门时拉取权限范围内全部用户（从根部门递归全公司）
	remote := make(map[string]RemoteUser)
	if departments := a.ParseDepartments(); len(departments) > 0 {
		for _, deptID := range departments {
			users, err := a.GetDepartmentUsers(contactToken, deptID, true)
			if err != nil {
				base.Error("获取钉钉部门成员失败", deptID, err)
				continue
			}
			for _, u := range users {
				if _, exists := remote[u.UserId]; !exists {
					remote[u.UserId] = RemoteUser{ID: u.UserId, Name: u.Name, Phone: u.Mobile, Email: u.Email}
				}
			}
		}
	} else {
		base.Warn("钉钉未配置允许部门，同步权限范围内全部用户")
		users, err := a.GetAllUsers(contactToken)
		if err != nil {
			return fmt.Errorf("获取钉钉全部用户失败: %w", err)
		}
		for _, u := range users {
			if _, exists := remote[u.UserId]; !exists {
				remote[u.UserId] = RemoteUser{ID: u.UserId, Name: u.Name, Phone: u.Mobile, Email: u.Email}
			}
		}
	}
	if len(remote) == 0 {
		return fmt.Errorf("钉钉拉取到的用户列表为空（可能部门配置错误、access_token 权限不足或应用可见范围未覆盖），组: %s", g.Name)
	}

	return syncExternalUsers(g, "dingtalk", "钉钉", remote, needOTP, isBlocked, false)
}

// 同步所有钉钉认证源用户（按各钉钉认证源的 sync_users 独立控制）
func SyncDingtalkUsers() {
	syncExternalUsersBySource(extSyncSource{
		authType: "dingtalk",
		name:     "钉钉",
		resolve: func(g *Group) (bool, func(*Group) error, error) {
			cfg, err := ResolveDingtalkConfig(g)
			if err != nil {
				return false, nil, err
			}
			return cfg.SyncUsers, cfg.SaveUsers, nil
		},
	})
}
