package dbdata

import (
	"fmt"
	"strings"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xlzd/gotp"
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
	profile, err := auth.ParseAuthProfile(g.AuthProfile)
	if err != nil {
		return nil, err
	}

	var cfg map[string]any
	for _, step := range profile.Step {
		if step.Type == "feishu" {
			if step.Provider == "" {
				return nil, fmt.Errorf("飞书步骤未设置 Provider")
			}
			resolved, err := ResolveProviderConfig(step.Provider, "feishu")
			if err != nil {
				return nil, fmt.Errorf("解析飞书 Provider %q: %w", step.Provider, err)
			}
			cfg = resolved
			break
		}
	}
	if cfg == nil {
		return nil, fmt.Errorf("认证配置中未找到飞书步骤")
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

	// 拉取允许部门内的成员；未配置部门时拉取权限范围内全部用户
	feishuUserMap := make(map[string]auth.FeishuDeptUserItem)
	if departments := a.ParseDepartments(); len(departments) > 0 {
		for _, deptID := range departments {
			users, err := a.GetDepartmentUsers(accessToken, deptID)
			if err != nil {
				base.Error("获取飞书部门成员失败", deptID, err)
				continue
			}
			for _, u := range users {
				if _, exists := feishuUserMap[u.UserID]; !exists {
					feishuUserMap[u.UserID] = u
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
			if _, exists := feishuUserMap[u.UserID]; !exists {
				feishuUserMap[u.UserID] = u
			}
		}
	}

	// 同步到本地 DB
	syncedUsers := make(map[string]bool)
	var added, updated, skipped int
	for _, feishuUser := range feishuUserMap {
		// 拒绝名单：同步时跳过
		if a.CheckUserID(feishuUser.UserID, blocked) != nil {
			base.Debug("飞书同步跳过拒绝用户:", feishuUser.UserID)
			skipped++
			continue
		}
		syncedUsers[feishuUser.UserID] = true
		mobile, email := feishuUser.Mobile, ""
		if detail, derr := a.GetFeishuUserDetail(accessToken, feishuUser.UserID); derr == nil {
			if detail.Data.Mobile != "" {
				mobile = detail.Data.Mobile
			}
			email = detail.Data.Email
		} else {
			base.Debug("飞书获取用户明细失败:", feishuUser.UserID, derr)
		}

		newUser := &User{
			Type:       "feishu",
			Username:   feishuUser.UserID,
			Nickname:   feishuUser.Name,
			Phone:      strings.Split(mobile, "+86")[1],
			Email:      email,
			Groups:     []string{g.Name},
			DisableOtp: !needOTP,
			OtpSecret:  gotp.RandomSecret(32),
			SendEmail:  false,
			Status:     1,
		}

		// 查现有用户
		u := &User{}
		if err := One("username", feishuUser.UserID, u); err != nil {
			if CheckErrNotFound(err) {
				if err := Add(newUser); err != nil {
					base.Error("新增飞书用户失败", feishuUser.UserID, err)
					continue
				}
				added++
				continue
			}
			base.Error("查询用户失败", feishuUser.UserID, err)
			continue
		}
		if u.Type != "feishu" {
			base.Warn("已存在本地同名用户:", feishuUser.UserID)
			skipped++
			continue
		}
		// 更新现有飞书用户字段
		u.Nickname = feishuUser.Name
		if mobile != "" {
			u.Phone = strings.Split(mobile, "+86")[1]
		}
		if email != "" {
			u.Email = email
		}
		u.DisableOtp = !needOTP
		if u.OtpSecret == "" {
			u.OtpSecret = gotp.RandomSecret(32)
		}
		if !utils.InArrStr(u.Groups, g.Name) {
			u.Groups = append(u.Groups, g.Name)
		}
		if err := Set(u); err != nil {
			base.Error("更新飞书用户失败", u.Username, err)
		} else {
			updated++
		}
	}
	if len(feishuUserMap) == 0 {
		base.Warn("飞书拉取到的用户列表为空（可能部门配置错误或 access_token 权限不足），组:", g.Name)
	}

	// 清理已不在飞书部门中的本地 feishu 用户
	var localFeishuUsers []User
	if err := FindWhere(&localFeishuUsers, 0, 0, "type = 'feishu'"); err != nil {
		base.Error("查询本地飞书用户失败，跳过用户清理:", err)
		return fmt.Errorf("查询本地飞书用户失败: %w", err)
	}
	for _, localUser := range localFeishuUsers {
		if !utils.InArrStr(localUser.Groups, g.Name) {
			continue
		}
		if !syncedUsers[localUser.Username] {
			localUser.Groups = utils.RemoveStrFromArr(localUser.Groups, g.Name)
			if len(localUser.Groups) == 0 {
				if err := Del(&localUser); err != nil {
					base.Error("删除本地飞书用户失败:", localUser.Username, err)
				} else {
					base.Info("成功删除本地飞书用户:", localUser.Username)
				}
			} else {
				if err := Set(&localUser); err != nil {
					base.Error("更新用户组失败:", localUser.Username, err)
				} else {
					base.Info("成功从组 '"+g.Name+"' 移除用户:", localUser.Username)
				}
			}
		}
	}
	return nil
}

// 同步所有飞书认证源用户（按各飞书认证源的 sync_users 独立控制）
func SyncFeishuUsers() {
	groups, err := GetAllGroups()
	if err != nil {
		base.Error("获取所有组失败:", err)
		return
	}
	for _, g := range groups {
		if !HasAuthType(g.AuthProfile, "feishu") {
			continue
		}
		authFs, err := ResolveFeishuConfig(&g)
		if err != nil {
			base.Error("解析飞书配置失败:", g.Name, err)
			continue
		}
		if !authFs.SyncUsers {
			continue
		}
		go func(g Group, a *AuthFeishu) {
			if err := a.SaveUsers(&g); err != nil {
				base.Error("飞书用户同步失败", g.Name, err)
			}
		}(g, authFs)
	}
}
