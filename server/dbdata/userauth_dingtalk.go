package dbdata

import (
	"fmt"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xlzd/gotp"
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
	profile, err := auth.ParseAuthProfile(g.AuthProfile)
	if err != nil {
		return nil, err
	}

	var cfg map[string]any
	for _, step := range profile.Step {
		if step.Type == "dingtalk" {
			if step.Provider == "" {
				return nil, fmt.Errorf("钉钉步骤未设置 Provider")
			}
			resolved, err := ResolveProviderConfig(step.Provider, "dingtalk")
			if err != nil {
				return nil, fmt.Errorf("解析钉钉 Provider %q: %w", step.Provider, err)
			}
			cfg = resolved
			break
		}
	}
	if cfg == nil {
		return nil, fmt.Errorf("认证配置中未找到钉钉步骤")
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

	// 拉取允许部门内的成员（含子部门）；未配置部门时拉取权限范围内全部用户（从根部门递归全公司）
	dtUserMap := make(map[string]auth.DingtalkDeptUser)
	if departments := a.ParseDepartments(); len(departments) > 0 {
		for _, deptID := range departments {
			users, err := a.GetDepartmentUsers(contactToken, deptID, true)
			if err != nil {
				base.Error("获取钉钉部门成员失败", deptID, err)
				continue
			}
			for _, u := range users {
				if _, exists := dtUserMap[u.UserId]; !exists {
					dtUserMap[u.UserId] = u
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
			if _, exists := dtUserMap[u.UserId]; !exists {
				dtUserMap[u.UserId] = u
			}
		}
	}

	// 同步到本地 DB
	syncedUsers := make(map[string]bool)
	var added, updated, skipped int
	for _, dtUser := range dtUserMap {
		syncedUsers[dtUser.UserId] = true

		// 拒绝名单：不同步到本地，跳过后续新增/更新
		if a.CheckUserID(dtUser.UserId, blocked) != nil {
			base.Warn("钉钉同步跳过拒绝用户:", dtUser.UserId)
			skipped++
			continue
		}

		newUser := &User{
			Type:       "dingtalk",
			Username:   dtUser.UserId,
			Nickname:   dtUser.Name,
			Email:      dtUser.Email,
			Phone:      dtUser.Mobile,
			Groups:     []string{g.Name},
			DisableOtp: !needOTP,
			OtpSecret:  gotp.RandomSecret(32),
			SendEmail:  false,
			Status:     1,
		}

		// 查现有用户
		u := &User{}
		if err := One("username", dtUser.UserId, u); err != nil {
			if CheckErrNotFound(err) {
				if err := Add(newUser); err != nil {
					base.Error("新增钉钉用户失败", dtUser.UserId, err)
					continue
				}
				added++
				continue
			}
			base.Error("查询用户失败", dtUser.UserId, err)
			continue
		}
		if u.Type != "dingtalk" {
			base.Warn("已存在本地同名用户:", dtUser.UserId)
			skipped++
			continue
		}
		// 更新现有钉钉用户字段
		u.Nickname = dtUser.Name
		if dtUser.Email != "" {
			u.Email = dtUser.Email
		}
		if dtUser.Mobile != "" {
			u.Phone = dtUser.Mobile
		}
		u.DisableOtp = !needOTP
		if u.OtpSecret == "" {
			u.OtpSecret = gotp.RandomSecret(32)
		}
		if !utils.InArrStr(u.Groups, g.Name) {
			u.Groups = append(u.Groups, g.Name)
		}
		if err := Set(u); err != nil {
			base.Error("更新钉钉用户失败", u.Username, err)
		} else {
			updated++
		}
	}
	if len(dtUserMap) == 0 {
		return fmt.Errorf("钉钉拉取到的用户列表为空（可能部门配置错误、access_token 权限不足或应用可见范围未覆盖），组: %s", g.Name)
	}

	// 清理已不在钉钉部门中的本地 dingtalk 用户
	var localDingtalkUsers []User
	if err := FindWhere(&localDingtalkUsers, 0, 0, "type = 'dingtalk'"); err != nil {
		base.Error("查询本地钉钉用户失败，跳过用户清理:", err)
		return fmt.Errorf("查询本地钉钉用户失败: %w", err)
	}
	for _, localUser := range localDingtalkUsers {
		if !utils.InArrStr(localUser.Groups, g.Name) {
			continue
		}
		if !syncedUsers[localUser.Username] || a.CheckUserID(localUser.Username, blocked) != nil {
			localUser.Groups = utils.RemoveStrFromArr(localUser.Groups, g.Name)
			if len(localUser.Groups) == 0 {
				if err := Del(&localUser); err != nil {
					base.Error("删除本地钉钉用户失败:", localUser.Username, err)
				} else {
					base.Info("成功删除本地钉钉用户:", localUser.Username)
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

// 同步所有钉钉认证源用户（按各钉钉认证源的 sync_users 独立控制）
func SyncDingtalkUsers() {
	groups, err := GetAllGroups()
	if err != nil {
		base.Error("获取所有组失败:", err)
		return
	}
	for _, g := range groups {
		if !HasAuthType(g.AuthProfile, "dingtalk") {
			continue
		}
		authDt, err := ResolveDingtalkConfig(&g)
		if err != nil {
			base.Error("解析钉钉配置失败:", g.Name, err)
			continue
		}
		if !authDt.SyncUsers {
			continue
		}
		go func(g Group, a *AuthDingtalk) {
			if err := a.SaveUsers(&g); err != nil {
				base.Error("钉钉用户同步失败", g.Name, err)
			}
		}(g, authDt)
	}
}
