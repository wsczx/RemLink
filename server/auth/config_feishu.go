// 飞书 Provider 配置类型及 API 调用

package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/wsczx/remlink/base"
)

type FeishuConfig struct {
	AppID              string `json:"app_id"`
	AppSecret          string `json:"app_secret"`
	UseDefaultBrowser  bool   `json:"use_default_browser"`
	AllowedDepartments string `json:"allowed_departments"`
	BlockedUserIDs     string `json:"blocked_userids"` // 拒绝的用户 ID 列表，逗号分隔
	SyncUsers          bool   `json:"sync_users"`      // 定时自动同步用户
}

func (c *FeishuConfig) ValidateConfig() error {
	if strings.TrimSpace(c.AppID) == "" {
		return fmt.Errorf("应用 ID 不能为空")
	}
	if strings.TrimSpace(c.AppSecret) == "" {
		return fmt.Errorf("应用 Secret 不能为空")
	}
	if c.AllowedDepartments != "" {
		parts := strings.SplitSeq(c.AllowedDepartments, ",")
		for part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				if _, err := strconv.Atoi(part); err != nil {
					return fmt.Errorf("部门 ID 必须为数字: %s", part)
				}
			}
		}
	}
	return nil
}

// 解析允许的部门列表
func (c *FeishuConfig) ParseDepartments() []string {
	if c.AllowedDepartments == "" {
		return nil
	}
	var depts []string
	parts := strings.SplitSeq(c.AllowedDepartments, ",")
	for part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			depts = append(depts, part)
		}
	}
	return depts
}

// 解析拒绝的用户 ID 列表
func (c *FeishuConfig) ParseBlockedUserIDs() []string {
	if c.BlockedUserIDs == "" {
		return nil
	}
	var ids []string
	parts := strings.SplitSeq(c.BlockedUserIDs, ",")
	for part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			ids = append(ids, part)
		}
	}
	return ids
}

// 校验用户是否在拒绝名单中
func (c *FeishuConfig) CheckUserID(userID string, blockedIDs []string) error {
	if slices.Contains(blockedIDs, userID) {
		return fmt.Errorf("%s 在拒绝的用户列表中", userID)
	}
	return nil
}

type FeishuTokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	AppAccessToken    string `json:"app_access_token"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

type FeishuUserResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		UserID        string   `json:"user_id"`
		OpenID        string   `json:"open_id"`
		UnionID       string   `json:"union_id"`
		Name          string   `json:"name"`
		Mobile        string   `json:"mobile"`
		Email         string   `json:"email"`
		DepartmentIDs []string `json:"department_ids"`
	} `json:"data"`
}

// 获取飞书 tenant_access_token（内部应用调用通讯录管理 API 必须使用此 Token）
func (c *FeishuConfig) GetAppAccessToken() (string, error) {
	type reqBody struct {
		AppID     string `json:"app_id"`
		AppSecret string `json:"app_secret"`
	}
	b, err := json.Marshal(reqBody{AppID: c.AppID, AppSecret: c.AppSecret})
	if err != nil {
		return "", fmt.Errorf("序列化请求体失败: %w", err)
	}

	url := "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"
	tokenResp := &FeishuTokenResponse{}
	if err := fetchJSON("获取飞书tenant_access_token", "POST", url, bytes.NewReader(b), nil, tokenResp, 0); err != nil {
		return "", err
	}
	if tokenResp.Code != 0 {
		return "", fmt.Errorf("获取 tenant_access_token 失败: code=%d msg=%s", tokenResp.Code, tokenResp.Msg)
	}
	if tokenResp.TenantAccessToken == "" {
		return "", fmt.Errorf("返回的 tenant_access_token 为空")
	}
	return tokenResp.TenantAccessToken, nil
}

type FeishuUserTokenResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

// 通过 code 获取飞书用户信息
func (c *FeishuConfig) GetFeishuUser(code string) (string, error) {
	tenantAccessToken, err := c.GetAppAccessToken()
	if err != nil {
		return "", err
	}

	// 获取 user_access_token
	type tokenReq struct {
		GrantType string `json:"grant_type"`
		Code      string `json:"code"`
	}
	b, err := json.Marshal(tokenReq{GrantType: "authorization_code", Code: code})
	if err != nil {
		return "", fmt.Errorf("序列化请求体失败: %w", err)
	}
	tokenResp := &FeishuUserTokenResponse{}
	headers := map[string]string{"Authorization": "Bearer " + tenantAccessToken}
	if err := fetchJSON("获取飞书user_access_token", "POST",
		"https://open.feishu.cn/open-apis/authen/v1/access_token",
		bytes.NewReader(b), headers, tokenResp, 0); err != nil {
		return "", err
	}
	if tokenResp.Code != 0 {
		return "", fmt.Errorf("获取 user_access_token 失败: code=%d msg=%s", tokenResp.Code, tokenResp.Msg)
	}
	userAccessToken := tokenResp.Data.AccessToken
	if userAccessToken == "" {
		return "", fmt.Errorf("获取 user_access_token 返回为空")
	}

	// 获取登录用户信息
	url := "https://open.feishu.cn/open-apis/authen/v1/user_info"
	userInfo := &FeishuUserResponse{}
	userHeaders := map[string]string{"Authorization": "Bearer " + userAccessToken}
	if err := fetchJSON("获取飞书用户信息", "GET", url, nil, userHeaders, userInfo, 0); err != nil {
		return "", err
	}
	if userInfo.Code != 0 {
		return "", fmt.Errorf("获取用户信息失败: code=%d msg=%s", userInfo.Code, userInfo.Msg)
	}
	userID := userInfo.Data.UserID
	if userID == "" {
		base.Warn("飞书 user_info 未返回 user_id（可能缺少通讯录权限），回退使用 open_id")
		userID = userInfo.Data.OpenID
	}
	return userID, nil
}

// 获取飞书用户详细信息（含部门、邮箱、手机号）
// 飞书 contact/v3/users 接口默认只返回基础字段，email/mobile 等需显式通过
// user_field_mask 请求，否则永远返回空串（这正是之前同步不到邮箱的原因）。
func (c *FeishuConfig) GetFeishuUserDetail(tenantAccessToken, userID string) (*FeishuUserResponse, error) {
	url := fmt.Sprintf("https://open.feishu.cn/open-apis/contact/v3/users/%s?user_id_type=user_id&user_field_mask=email,mobile,name,department_ids,open_id,union_id", userID)
	userInfo := &FeishuUserResponse{}
	headers := map[string]string{"Authorization": "Bearer " + tenantAccessToken}
	if err := fetchJSON("获取飞书用户详细信息", "GET", url, nil, headers, userInfo, 0); err != nil {
		return nil, err
	}
	if userInfo.Code != 0 {
		return nil, fmt.Errorf("获取用户详细信息失败: code=%d msg=%s", userInfo.Code, userInfo.Msg)
	}
	return userInfo, nil
}

// 检查用户是否属于允许的部门
func (c *FeishuConfig) CheckUserDepartment(tenantAccessToken, userID string, allowedDepts []string) (bool, error) {
	if len(allowedDepts) == 0 {
		return true, nil
	}
	userInfo, err := c.GetFeishuUserDetail(tenantAccessToken, userID)
	if err != nil {
		return false, err
	}
	for _, userDept := range userInfo.Data.DepartmentIDs {
		if slices.Contains(allowedDepts, userDept) {
			return true, nil
		}
	}
	return false, nil
}

type FeishuDeptUserListResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Items     []FeishuDeptUserItem `json:"items"`
		HasMore   bool                 `json:"has_more"`
		PageToken string               `json:"page_token"`
	} `json:"data"`
}

type FeishuDeptUserItem struct {
	UserID        string   `json:"user_id"`
	OpenID        string   `json:"open_id"`
	Name          string   `json:"name"`
	Mobile        string   `json:"mobile"`
	Email         string   `json:"email"`
	DepartmentIDs []string `json:"department_ids"`
}

// 获取飞书部门成员列表
func (c *FeishuConfig) GetDepartmentUsers(tenantAccessToken, departmentID string) ([]FeishuDeptUserItem, error) {
	var allUsers []FeishuDeptUserItem
	pageToken := ""
	headers := map[string]string{"Authorization": "Bearer " + tenantAccessToken}

	for {
		url := fmt.Sprintf(
			"https://open.feishu.cn/open-apis/contact/v3/users/find_by_department?department_id=%s&user_id_type=user_id&department_id_type=open_department_id&page_size=50&user_field_mask=mobile,email",
			departmentID,
		)
		// 如果根部门是 "0"，调整 department_id_type 适配
		if departmentID == "0" {
			url = "https://open.feishu.cn/open-apis/contact/v3/users/find_by_department?department_id=0&user_id_type=user_id&department_id_type=department_id&page_size=50&user_field_mask=mobile,email"
		}

		if pageToken != "" {
			url += "&page_token=" + pageToken
		}

		listResp := &FeishuDeptUserListResponse{}
		if err := fetchJSON("获取飞书部门成员", "GET", url, nil, headers, listResp, 0); err != nil {
			return nil, err
		}
		if listResp.Code != 0 {
			base.Warn("获取飞书部门成员失败 dept=", departmentID, " code=", listResp.Code, " msg=", listResp.Msg)
			return nil, fmt.Errorf("获取飞书部门成员失败: code=%d msg=%s", listResp.Code, listResp.Msg)
		}

		allUsers = append(allUsers, listResp.Data.Items...)
		if !listResp.Data.HasMore {
			break
		}
		pageToken = listResp.Data.PageToken
	}

	return allUsers, nil
}

// 获取飞书指定部门的子部门 ID 列表
func (c *FeishuConfig) getChildDepartmentIDs(tenantAccessToken, departmentID string) ([]string, error) {
	var ids []string
	pageToken := ""
	headers := map[string]string{"Authorization": "Bearer " + tenantAccessToken}

	for {
		deptType := "open_department_id"
		if departmentID == "0" {
			deptType = "department_id"
		}
		url := fmt.Sprintf(
			"https://open.feishu.cn/open-apis/contact/v3/departments/%s/children?department_id_type=%s&page_size=50",
			departmentID, deptType,
		)
		if pageToken != "" {
			url += "&page_token=" + pageToken
		}

		resp := &struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				Items []struct {
					DepartmentID     string `json:"department_id"`
					OpenDepartmentID string `json:"open_department_id"`
				} `json:"items"`
				HasMore   bool   `json:"has_more"`
				PageToken string `json:"page_token"`
			} `json:"data"`
		}{}
		if err := fetchJSON("获取飞书子部门列表", "GET", url, nil, headers, resp, 0); err != nil {
			return nil, err
		}
		if resp.Code != 0 {
			return nil, fmt.Errorf("获取子部门列表失败: code=%d msg=%s", resp.Code, resp.Msg)
		}
		for _, item := range resp.Data.Items {
			id := item.OpenDepartmentID
			if id == "" {
				id = item.DepartmentID
			}
			if id != "" {
				ids = append(ids, id)
			}
		}
		if !resp.Data.HasMore {
			break
		}
		pageToken = resp.Data.PageToken
	}
	return ids, nil
}

// 递归收集指定部门及其所有子部门
func (c *FeishuConfig) collectDepartmentTree(tenantAccessToken, rootID string, deptIDs *[]string, visited map[string]bool, isRoot bool) error {
	if visited[rootID] {
		return nil
	}
	visited[rootID] = true
	*deptIDs = append(*deptIDs, rootID)

	childIDs, err := c.getChildDepartmentIDs(tenantAccessToken, rootID)
	if err != nil {
		// 非根部门（叶子节点）失败可容忍跳过，防止单个部门权限问题阻塞全量同步
		if isRoot {
			return fmt.Errorf("获取根部门子部门失败（请检查应用通讯录权限及可用范围）: %w", err)
		}
		base.Warn("获取子部门失败，跳过该分支: dept=", rootID, " err=", err)
		return nil
	}
	for _, id := range childIDs {
		if err := c.collectDepartmentTree(tenantAccessToken, id, deptIDs, visited, false); err != nil {
			return err
		}
	}
	return nil
}

// 获取飞书权限范围内全部用户（结合部门白名单与用户黑名单过滤）
func (c *FeishuConfig) GetAllUsers(tenantAccessToken string) ([]FeishuDeptUserItem, error) {
	var allDeptIDs []string
	visited := make(map[string]bool)

	// 读取配置的允许部门
	allowedDepts := c.ParseDepartments()
	roots := allowedDepts
	if len(roots) == 0 {
		// 未配置允许部门时，才从 "0"（全公司根节点）开始递归
		roots = []string{"0"}
	}

	// 从各个允许的部门起点向下收集部门树
	for _, rootID := range roots {
		if err := c.collectDepartmentTree(tenantAccessToken, rootID, &allDeptIDs, visited, true); err != nil {
			// 如果配置了具体的部门但递归失败，记录日志或按需抛出
			base.Warn("同步时递归部门树失败, rootID:", rootID, "err:", err)
		}
	}

	if len(allDeptIDs) == 0 {
		return nil, fmt.Errorf("未能获取到任何有效的部门节点，请检查部门配置或通讯录权限")
	}

	// 读取拒绝的用户 ID 列表
	blockedIDs := c.ParseBlockedUserIDs()

	userMap := make(map[string]FeishuDeptUserItem)
	failCount := 0
	for _, deptID := range allDeptIDs {
		users, err := c.GetDepartmentUsers(tenantAccessToken, deptID)
		if err != nil {
			base.Warn("获取飞书部门成员失败，跳过:", deptID, err)
			failCount++
			continue
		}
		for _, u := range users {
			key := u.UserID
			if key == "" {
				key = u.OpenID
			}
			if key == "" {
				continue
			}

			// 过滤黑名单用户
			if c.CheckUserID(key, blockedIDs) != nil {
				base.Info("用户在黑名单中，同步时予以跳过:", key)
				continue
			}

			if _, exists := userMap[key]; !exists {
				userMap[key] = u
			}
		}
	}

	if len(userMap) == 0 {
		if failCount > 0 {
			return nil, fmt.Errorf("拉取飞书用户失败，请检查应用通讯录权限及可用范围是否覆盖目标部门")
		}
		return nil, fmt.Errorf("未在飞书通讯录中获取到任何符合过滤条件的用户")
	}

	all := make([]FeishuDeptUserItem, 0, len(userMap))
	for _, u := range userMap {
		all = append(all, u)
	}
	return all, nil
}

// 生成飞书 OAuth2 授权 URL
func (c *FeishuConfig) GetAuthURL(redirectURI, state string) string {
	return fmt.Sprintf(
		"https://open.feishu.cn/open-apis/authen/v1/authorize?app_id=%s&redirect_uri=%s&state=%s",
		c.AppID, redirectURI, state,
	)
}
