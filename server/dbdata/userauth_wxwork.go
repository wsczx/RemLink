package dbdata

import (
	"fmt"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xlzd/gotp"
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
	profile, err := auth.ParseAuthProfile(g.AuthProfile)
	if err != nil {
		return nil, err
	}

	var cfg map[string]any
	for _, step := range profile.Step {
		if step.Type == "wxwork" {
			if step.Provider == "" {
				return nil, fmt.Errorf("企微步骤未设置 Provider")
			}
			resolved, err := ResolveProviderConfig(step.Provider, "wxwork")
			if err != nil {
				return nil, fmt.Errorf("解析企微 Provider %q: %w", step.Provider, err)
			}
			cfg = resolved
			break
		}
	}
	if cfg == nil {
		return nil, fmt.Errorf("认证配置中未找到企业微信步骤")
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

	// 拉取允许部门内的成员；未配置部门时拉取权限范围内全部用户
	departments := a.ParseDepartments()
	if len(departments) == 0 {
		base.Debug("企微未配置允许部门，同步权限范围内全部用户")
		departments = []int{1} // 企微根部门
	}
	wxUserMap := make(map[string]auth.WXWorkDepartmentUser)
	for _, deptID := range departments {
		users, err := a.GetDepartmentUsers(accessToken, deptID)
		if err != nil {
			base.Error("获取部门成员失败", deptID, err)
			continue
		}
		for _, u := range users {
			if _, exists := wxUserMap[u.UserID]; !exists {
				wxUserMap[u.UserID] = u
			}
		}
	}

	// 同步到本地 DB
	syncedUsers := make(map[string]bool)
	var added, updated, skipped int
	for _, wxUser := range wxUserMap {
		syncedUsers[wxUser.UserID] = true
		// 拒绝名单：不同步到本地，跳过后续新增/更新
		if a.CheckUserID(wxUser.UserID, blocked) {
			base.Debug("企微同步跳过拒绝用户:", wxUser.UserID)
			skipped++
			continue
		}

		mobile, email := a.GetUserDetail(accessToken, wxUser.UserID)
		newUser := &User{
			Type:       "wxwork",
			Username:   wxUser.UserID,
			Nickname:   wxUser.Name,
			Phone:      mobile,
			Email:      email,
			Groups:     []string{g.Name},
			DisableOtp: !needOTP,
			OtpSecret:  gotp.RandomSecret(32),
			SendEmail:  false,
			Status:     1,
		}

		// 查现有用户
		u := &User{}
		if err := One("username", wxUser.UserID, u); err != nil {
			if CheckErrNotFound(err) {
				if err := Add(newUser); err != nil {
					base.Error("新增企微用户失败", wxUser.UserID, err)
					continue
				}
				added++
				continue
			}
			base.Error("查询用户失败", wxUser.UserID, err)
			continue
		}
		if u.Type != "wxwork" {
			base.Warn("已存在本地同名用户:", wxUser.UserID)
			skipped++
			continue
		}
		// 更新现有企微用户字段
		u.Nickname = wxUser.Name
		if mobile != "" {
			u.Phone = mobile
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
			base.Error("更新企微用户失败", u.Username, err)
		} else {
			updated++
		}
	}
	if len(wxUserMap) == 0 {
		return fmt.Errorf("企微拉取到的用户列表为空（可能部门配置错误、access_token 权限不足或应用可见范围未覆盖），组: %s", g.Name)
	}

	// 清理已不在企微部门中的本地 wxwork 用户
	var localWxUsers []User
	if err := FindWhere(&localWxUsers, 0, 0, "type = 'wxwork'"); err != nil {
		base.Error("查询本地企微用户失败:", err)
		return fmt.Errorf("查询本地企微用户失败: %w", err)
	}
	for _, localUser := range localWxUsers {
		if !utils.InArrStr(localUser.Groups, g.Name) {
			continue
		}
		if !syncedUsers[localUser.Username] || a.CheckUserID(localUser.Username, blocked) {
			localUser.Groups = utils.RemoveStrFromArr(localUser.Groups, g.Name)
			if len(localUser.Groups) == 0 {
				if err := Del(&localUser); err != nil {
					base.Error("删除本地企微用户失败:", localUser.Username, err)
				} else {
					base.Info("成功删除本地企微用户:", localUser.Username)
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

// 同步所有企业微信认证源用户（按各企微认证源的 sync_users 独立控制）
func SyncWXworkUsers() {
	groups, err := GetAllGroups()
	if err != nil {
		base.Error("获取所有组失败:", err)
		return
	}
	for _, g := range groups {
		if !HasAuthType(g.AuthProfile, "wxwork") {
			continue
		}
		authWx, err := ResolveWxworkConfig(&g)
		if err != nil {
			base.Error("解析企微配置失败:", g.Name, err)
			continue
		}
		if !authWx.SyncUsers {
			continue
		}
		go func(g Group, a *AuthWXwork) {
			if err := a.SaveUsers(&g); err != nil {
				base.Error("企微用户同步失败", g.Name, err)
			}
		}(g, authWx)
	}
}
