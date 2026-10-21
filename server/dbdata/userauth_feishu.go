package dbdata

import (
	"fmt"
	"strings"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
)

// AuthFeishu 飞书认证配置（嵌入共享 FeishuConfig）
type AuthFeishu struct {
	auth.FeishuConfig
}

// 从组的 AuthProfile 中获取飞书认证配置
func GetAuthFeishu(groupName string) (*AuthFeishu, error) {
	groupData := &Group{}
	if err := One("Name", groupName, groupData); err != nil {
		return nil, fmt.Errorf("用户组错误: %v", err)
	}
	return ResolveFeishuConfig(groupData)
}

// 从 Group 的 AuthProfile 解析飞书配置
func ResolveFeishuConfig(g *Group) (*AuthFeishu, error) {
	cfg, err := resolveProviderConfigMap(g, "feishu", "飞书")
	if err != nil {
		return nil, err
	}

	result := &AuthFeishu{}
	if err := auth.GetProviderConfigFromMap(cfg, &result.FeishuConfig); err != nil {
		return nil, err
	}
	if result.AppID == "" || result.AppSecret == "" {
		return nil, fmt.Errorf("飞书配置不完整")
	}
	return result, nil
}

// 从飞书同步用户到本地数据库
func (a *AuthFeishu) SaveUsers(g *Group) error {
	accessToken, err := a.GetAppAccessToken()
	if err != nil {
		return fmt.Errorf("获取飞书 access_token 失败: %w", err)
	}

	needOTP := HasAuthType(g.AuthProfile, "otp")
	blocked := a.ParseBlockedUserIDs()
	isBlocked := func(id string) bool { return a.CheckUserID(id, blocked) != nil }

	// 拉取允许部门内的成员；未配置部门时拉取权限范围内全部用户
	remote := make(map[string]RemoteUser)
	if departments := a.ParseDepartments(); len(departments) > 0 {
		for _, deptID := range departments {
			users, err := a.GetDepartmentUsers(accessToken, deptID)
			if err != nil {
				base.Error("获取飞书部门成员失败", deptID, err)
				continue
			}
			for _, u := range users {
				if _, exists := remote[u.UserID]; !exists {
					remote[u.UserID] = RemoteUser{ID: u.UserID, Name: u.Name, Phone: u.Mobile, Email: u.Email}
				}
			}
		}
	} else {
		base.Debug("飞书未配置允许部门，同步权限范围内全部用户")
		users, err := a.GetAllUsers(accessToken)
		if err != nil {
			return fmt.Errorf("获取飞书全部用户失败: %w", err)
		}
		for _, u := range users {
			if _, exists := remote[u.UserID]; !exists {
				remote[u.UserID] = RemoteUser{ID: u.UserID, Name: u.Name, Phone: u.Mobile, Email: u.Email}
			}
		}
	}

	// 明细补全 + 手机号去 +86 前缀
	for id, ru := range remote {
		if isBlocked(id) {
			continue
		}
		if detail, derr := a.GetFeishuUserDetail(accessToken, id); derr == nil {
			if detail.Data.Mobile != "" {
				ru.Phone = detail.Data.Mobile
			}
			if detail.Data.Email != "" {
				ru.Email = detail.Data.Email
			}
		} else {
			base.Debug("飞书获取用户明细失败:", id, derr)
		}
		ru.Phone = strings.TrimPrefix(ru.Phone, "+86")
		remote[id] = ru
	}
	if len(remote) == 0 {
		return fmt.Errorf("飞书拉取到的用户列表为空（可能部门配置错误、access_token 权限不足或应用可见范围未覆盖），组: %s", g.Name)
	}

	return syncExternalUsers(g, "feishu", "飞书", remote, needOTP, isBlocked, false)
}

// 同步所有飞书认证源用户（按各飞书认证源的 sync_users 独立控制）
func SyncFeishuUsers() {
	syncExternalUsersBySource(extSyncSource{
		authType: "feishu",
		name:     "飞书",
		resolve: func(g *Group) (bool, func(*Group) error, error) {
			cfg, err := ResolveFeishuConfig(g)
			if err != nil {
				return false, nil, err
			}
			return cfg.SyncUsers, cfg.SaveUsers, nil
		},
	})
}
