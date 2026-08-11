// 企业微信 Provider 配置类型及 API 调用

package auth

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wsczx/remlink/base"
)

type WXWorkConfig struct {
	CorpID             string `json:"corp_id"`
	AgentID            string `json:"agent_id"`
	Secret             string `json:"secret"`
	UseDefaultBrowser  bool   `json:"use_default_browser"`
	AllowedDepartments string `json:"allowed_departments"`
	BlockedUserIDs     string `json:"blocked_userids"`
	SyncUsers          bool   `json:"sync_users"`          // 定时自动同步用户
	VerifyFileName     string `json:"verify_file_name"`    // 企微回调域名验证文件名
	VerifyFileContent  string `json:"verify_file_content"` // 企微回调域名验证文件内容
}

func (c *WXWorkConfig) ValidateConfig() error {
	if c.CorpID == "" {
		return fmt.Errorf("企业 ID 不能为空")
	}
	if c.AgentID == "" {
		return fmt.Errorf("应用 ID 不能为空")
	}
	if c.Secret == "" {
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
	if c.BlockedUserIDs != "" {
		for part := range strings.SplitSeq(c.BlockedUserIDs, ",") {
			if strings.TrimSpace(part) == "" {
				return fmt.Errorf("拒绝的用户ID列表格式错误：存在空值")
			}
		}
	}
	return nil
}

// 解析允许的部门列表为整数数组
func (c *WXWorkConfig) ParseDepartments() []int {
	if c.AllowedDepartments == "" {
		return nil
	}
	var depts []int
	parts := strings.SplitSeq(c.AllowedDepartments, ",")
	for part := range parts {
		if id, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && id > 0 {
			depts = append(depts, id)
		}
	}
	return depts
}

type WXWorkTokenResponse struct {
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type WXWorkUserResponse struct {
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
	UserID     string `json:"userid"`
	Name       string `json:"name"`
	Mobile     string `json:"mobile"`
	Email      string `json:"email"`
	Department []int  `json:"department"`
}

// 获取企业微信 access_token
func (c *WXWorkConfig) GetAccessToken() (string, error) {
	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s", c.CorpID, c.Secret)

	tokenResp := &WXWorkTokenResponse{}
	if err := fetchJSON("获取企微access_token", "GET", url, nil, nil, tokenResp, 0); err != nil {
		return "", err
	}
	if tokenResp.ErrCode != 0 {
		return "", fmt.Errorf("获取 access_token 失败: %s", tokenResp.ErrMsg)
	}
	return tokenResp.AccessToken, nil
}

// 通过 code 获取企微用户 ID
func (c *WXWorkConfig) GetWeworkUser(code string) (string, error) {
	accessToken, err := c.GetAccessToken()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/auth/getuserinfo?access_token=%s&code=%s", accessToken, code)
	userInfo := &WXWorkUserResponse{}
	if err := fetchJSON("获取企微用户信息", "GET", url, nil, nil, userInfo, 0); err != nil {
		return "", err
	}
	if userInfo.ErrCode != 0 {
		return "", fmt.Errorf("获取用户信息失败: %s", userInfo.ErrMsg)
	}
	return userInfo.UserID, nil
}

// 解析拒绝的用户ID列表
func (c *WXWorkConfig) ParseBlockedUserIDs() []string {
	if c.BlockedUserIDs == "" {
		return nil
	}
	var ids []string
	for part := range strings.SplitSeq(c.BlockedUserIDs, ",") {
		if id := strings.TrimSpace(part); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// 检查用户ID是否在拒绝列表中（列表为空表示不限制）
func (c *WXWorkConfig) CheckUserID(userID string, blockedUserIDs []string) bool {
	return slices.Contains(blockedUserIDs, userID)
}

// 检查用户是否属于允许的部门（内部自取 access_token，调用处与 userid 校验对称）
func (c *WXWorkConfig) CheckUserDepartment(userID string, allowedDepts []int) (bool, error) {
	accessToken, err := c.GetAccessToken()
	if err != nil {
		return false, err
	}
	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/user/get?access_token=%s&userid=%s", accessToken, userID)
	userInfo := &WXWorkUserResponse{}
	if err := fetchJSON("获取企微用户详细信息", "GET", url, nil, nil, userInfo, 0); err != nil {
		return false, err
	}
	if userInfo.ErrCode != 0 {
		return false, fmt.Errorf("获取用户详细信息失败: %s", userInfo.ErrMsg)
	}

	for _, userDept := range userInfo.Department {
		if slices.Contains(allowedDepts, userDept) {
			return true, nil
		}
	}
	return false, nil
}

// GetUserDetail 获取企微用户详情（手机号、邮箱）。复用 user/get 接口逐个查询，
// 返回 (mobile, email)。企微手机/邮箱默认不返回，需后台开放通讯录敏感信息权限。
func (c *WXWorkConfig) GetUserDetail(accessToken, userID string) (string, string) {
	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/user/get?access_token=%s&userid=%s", accessToken, userID)
	userInfo := &WXWorkUserResponse{}
	if err := fetchJSON("获取企微用户详情", "GET", url, nil, nil, userInfo, 30*time.Second); err != nil {
		base.Warn("获取企微用户详情失败:", userID, err)
		return "", ""
	}
	if userInfo.ErrCode != 0 {
		base.Warn("获取企微用户详情失败:", userID, userInfo.ErrMsg)
		return "", ""
	}
	return userInfo.Mobile, userInfo.Email
}

type WXWorkDepartmentUser struct {
	UserID     string `json:"userid"`
	Name       string `json:"name"`
	Department []int  `json:"department"`
}

type WXWorkUserListResponse struct {
	ErrCode  int                    `json:"errcode"`
	ErrMsg   string                 `json:"errmsg"`
	UserList []WXWorkDepartmentUser `json:"userlist"`
}

// 获取部门成员列表（含子部门）
func (c *WXWorkConfig) GetDepartmentUsers(accessToken string, departmentID int) ([]WXWorkDepartmentUser, error) {
	url := fmt.Sprintf(
		"https://qyapi.weixin.qq.com/cgi-bin/user/simplelist?access_token=%s&department_id=%d&fetch_child=1",
		accessToken, departmentID,
	)

	listResp := &WXWorkUserListResponse{}
	if err := fetchJSON("获取企微部门成员", "GET", url, nil, nil, listResp, 30*time.Second); err != nil {
		return nil, err
	}
	if listResp.ErrCode != 0 {
		return nil, fmt.Errorf("获取部门成员列表失败: errcode=%d %s", listResp.ErrCode, listResp.ErrMsg)
	}
	return listResp.UserList, nil
}

// 生成企业微信 OAuth2 授权 URL
func (c *WXWorkConfig) GetAuthURL(redirectURI, state string) string {
	return fmt.Sprintf(
		"https://login.work.weixin.qq.com/wwlogin/sso/login?login_type=CorpApp&appid=%s&agentid=%s&redirect_uri=%s&state=%s",
		c.CorpID, c.AgentID, redirectURI, state,
	)
}
