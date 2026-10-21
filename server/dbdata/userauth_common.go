package dbdata

import (
	"fmt"
	"slices"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/xlzd/gotp"
)

// 外部认证源拉取到的用户
// 各认证源（企微/飞书/钉钉/LDAP）把自有结构转换为此形式后交给 syncExternalUsers
type RemoteUser struct {
	ID    string
	Name  string
	Phone string
	Email string
}

// 将外部认证源拉取到的用户同步到本地数据库（四源共用骨架）
// 新增/更新/跳过计数与日志、拒绝名单、失联用户清理
//
// 参数差异点：
//   - userType: 用户类型标识（wxwork/feishu/dingtalk/ldap）
//   - sourceName: 中文源名（日志用，如"企微"）
//   - needOTP: 该组是否要求 OTP（决定 DisableOtp）
//   - isBlocked: 拒绝名单判断（LDAP 无此概念，传 nil）
//   - fillEmptyOnly: true=仅填充本地空字段（LDAP 语义，保留本地编辑）
//     false=远端有值即覆盖（三方源语义，远端为准）
//
// 注意：拉取为空的检查由各源自行完成（各源判据不同：三方源看归一化 map，
// LDAP 看过滤前的原始搜索结果），不在本函数内
func syncExternalUsers(g *Group, userType, sourceName string,
	remote map[string]RemoteUser, needOTP bool,
	isBlocked func(string) bool, fillEmptyOnly bool) error {

	syncedUsers := make(map[string]bool)
	var added, updated, skipped int
	for _, ru := range remote {
		syncedUsers[ru.ID] = true
		// 拒绝名单：不同步到本地，跳过后续新增/更新
		if isBlocked != nil && isBlocked(ru.ID) {
			base.Debug(sourceName, "同步跳过拒绝用户:", ru.ID)
			skipped++
			continue
		}

		newUser := &User{
			Type:       userType,
			Username:   ru.ID,
			Nickname:   ru.Name,
			Phone:      ru.Phone,
			Email:      ru.Email,
			Groups:     []string{g.Name},
			DisableOtp: !needOTP,
			OtpSecret:  gotp.RandomSecret(32),
			SendEmail:  false,
			Status:     1,
		}

		// 查现有用户
		u := &User{}
		if err := One("username", ru.ID, u); err != nil {
			if CheckErrNotFound(err) {
				if err := Add(newUser); err != nil {
					base.Error("新增"+sourceName+"用户失败", ru.ID, err)
					continue
				}
				added++
				continue
			}
			base.Error("查询用户失败", ru.ID, err)
			continue
		}
		if u.Type != userType {
			base.Warn("已存在本地同名用户:", ru.ID)
			skipped++
			continue
		}
		// 更新现有用户字段
		u.Nickname = ru.Name
		u.DisableOtp = !needOTP
		if u.OtpSecret == "" {
			u.OtpSecret = gotp.RandomSecret(32)
		}
		if fillEmptyOnly {
			// LDAP 语义：本地已填写的字段不覆盖（保留本地编辑）
			if u.Email == "" {
				u.Email = ru.Email
			}
			if u.Phone == "" {
				u.Phone = ru.Phone
			}
		} else {
			// 三方源语义：远端有值即覆盖（远端为准）
			if ru.Email != "" {
				u.Email = ru.Email
			}
			if ru.Phone != "" {
				u.Phone = ru.Phone
			}
		}
		if !slices.Contains(u.Groups, g.Name) {
			u.Groups = append(u.Groups, g.Name)
		}
		if err := Set(u); err != nil {
			base.Error("更新"+sourceName+"用户失败", u.Username, err)
		} else {
			updated++
		}
	}
	base.Info(sourceName+"用户同步完成，组:", g.Name, " 新增:", added, " 更新:", updated, " 跳过:", skipped)

	// 清理已不在远端目录中的本地用户
	return cleanupAbsentExternalUsers(g, userType, sourceName, syncedUsers, isBlocked)
}

// 清理失联用户：从组中移除；无其他组时删除
func cleanupAbsentExternalUsers(g *Group, userType, sourceName string, syncedUsers map[string]bool, isBlocked func(string) bool) error {
	var localUsers []User
	if err := FindWhere(&localUsers, 0, 0, "type = ?", userType); err != nil {
		base.Error("查询本地"+sourceName+"用户失败:", err)
		return fmt.Errorf("查询本地%s用户失败: %w", sourceName, err)
	}
	for _, localUser := range localUsers {
		if !slices.Contains(localUser.Groups, g.Name) {
			continue
		}
		if !syncedUsers[localUser.Username] || (isBlocked != nil && isBlocked(localUser.Username)) {
			if i := slices.Index(localUser.Groups, g.Name); i >= 0 {
				localUser.Groups = slices.Delete(localUser.Groups, i, i+1)
			}
			if len(localUser.Groups) == 0 {
				if err := Del(&localUser); err != nil {
					base.Error("删除本地"+sourceName+"用户失败:", localUser.Username, err)
				} else {
					base.Info("成功删除本地"+sourceName+"用户:", localUser.Username)
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

// 从组的 AuthProfile 中解析指定认证步骤的 Provider 配置
// 四类外部认证源（企微/飞书/钉钉/LDAP）共用；各源仅需提供目标结构与完整性校验
func resolveProviderConfigMap(g *Group, stepType, name string) (map[string]any, error) {
	profile, err := auth.ParseAuthProfile(g.AuthProfile)
	if err != nil {
		return nil, err
	}
	for _, step := range profile.Step {
		if step.Type == stepType {
			if step.Provider == "" {
				return nil, fmt.Errorf("%s步骤未设置 Provider", name)
			}
			resolved, err := ResolveProviderConfig(step.Provider, stepType)
			if err != nil {
				return nil, fmt.Errorf("解析%s Provider %q: %w", name, step.Provider, err)
			}
			return resolved, nil
		}
	}
	return nil, fmt.Errorf("认证配置中未找到%s步骤", name)
}

// 外部认证源的同步入口描述（驱动 syncExternalUsersBySource）
type extSyncSource struct {
	authType string // 认证步骤类型
	name     string // 中文源名（日志用）
	// resolve 解析配置，返回 (是否启用同步, 同步执行函数)
	resolve func(g *Group) (enabled bool, save func(*Group) error, err error)
}

// 遍历所有组，对配置了该认证源且开启 sync_users 的组
// 异步执行用户同步（四源共用）
func syncExternalUsersBySource(src extSyncSource) {
	groups, err := GetAllGroups()
	if err != nil {
		base.Error("获取所有组失败:", err)
		return
	}
	for _, g := range groups {
		if !HasAuthType(g.AuthProfile, src.authType) {
			continue
		}
		enabled, save, err := src.resolve(&g)
		if err != nil {
			base.Error("解析"+src.name+"配置失败:", g.Name, err)
			continue
		}
		if !enabled {
			continue
		}
		base.SafeGo(func() {
			if err := save(&g); err != nil {
				base.Error(src.name+"用户同步失败", g.Name, err)
				return
			}
			base.Info(src.name+"用户组同步成功:", g.Name)
		})
	}
}
