package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wsczx/remlink/base"
)

var dingtalkHttpClient = &http.Client{
	Timeout: 15 * time.Second,
}

// 钉钉认证配置（企业内部应用 OAuth2 扫码登录）
type DingtalkConfig struct {
	ClientID           string `json:"client_id"`           // 钉钉应用 AppKey
	ClientSecret       string `json:"client_secret"`       // 钉钉应用 AppSecret
	AllowedDepartments string `json:"allowed_departments"` // 允许的部门 ID，逗号分隔；为空表示不限制
	BlockedUserIDs     string `json:"blocked_userids"`     // 拒绝的用户 ID 列表，逗号分隔
	UseDefaultBrowser  bool   `json:"use_default_browser"` // 默认使用浏览器打开钉钉授权页面
	SyncUsers          bool   `json:"sync_users"`          // 同步用户到本地
}

// 校验配置完整性
func (c *DingtalkConfig) ValidateConfig() error {
	if c.ClientID == "" || c.ClientSecret == "" {
		return fmt.Errorf("钉钉配置不完整：client_id 与 client_secret 均必填")
	}
	return nil
}

// 解析允许的部门 ID 列表（仅保留非空项）
func (c *DingtalkConfig) ParseDepartments() []string {
	out := make([]string, 0)
	for part := range strings.SplitSeq(c.AllowedDepartments, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// 解析拒绝的用户 ID 列表
func (c *DingtalkConfig) ParseBlockedUserIDs() []string {
	out := make([]string, 0)
	for part := range strings.SplitSeq(c.BlockedUserIDs, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// 命中拒绝名单时返回 error。
func (c *DingtalkConfig) CheckUserID(userID string, list []string) error {
	userID = strings.TrimSpace(userID)
	for _, v := range list {
		if strings.TrimSpace(v) == userID {
			return fmt.Errorf("%s 在拒绝的用户列表中", userID)
		}
	}
	return nil
}

// accessToken 响应
type dingtalkTokenResp struct {
	AccessToken string `json:"accessToken"`
	ExpiresIn   int    `json:"expiresIn"`
	Code        string `json:"code"`
	Message     string `json:"message"`
}

// sns 用户信息响应（/v1.0/contact/users/me）
type dingtalkSnsResp struct {
	UserId  string `json:"userId"`
	UnionID string `json:"unionId"`
	OpenID  string `json:"openId"`
	Nick    string `json:"nick"`
	Mobile  string `json:"mobile"`
}

// 用户详情（部门）
type dingtalkUserDetailResp struct {
	DeptIdList []int64 `json:"dept_id_list"`
	Code       string  `json:"code"`
	Message    string  `json:"message"`
}

// 用授权 code 换取用户 accessToken（钉钉新版 OAuth2）
func (c *DingtalkConfig) GetAccessToken(code string) (string, error) {
	body := map[string]string{
		"clientId":     c.ClientID,
		"clientSecret": c.ClientSecret,
		"code":         code,
		"grantType":    "authorization_code",
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.dingtalk.com/v1.0/oauth2/userAccessToken", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := dingtalkHttpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var tr dingtalkTokenResp
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return "", fmt.Errorf("解析钉钉 accessToken 响应失败: %w (raw=%s)", err, string(respBody))
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("获取钉钉 accessToken 失败: code=%s msg=%s", tr.Code, tr.Message)
	}
	return tr.AccessToken, nil
}

// 用 OAuth code 解析出登录用户标识。
// 流程：code 换用户 token -> /contact/users/me -> (若无 userId) 依次尝试【手机号反查】和【unionid反查】。
func (c *DingtalkConfig) GetDingtalkUser(code string) (string, string, string, error) {
	accessToken, err := c.GetAccessToken(code)
	if err != nil {
		return "", "", "", err
	}

	req, err := http.NewRequest(http.MethodGet, "https://api.dingtalk.com/v1.0/contact/users/me", nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("x-acs-dingtalk-access-token", accessToken)

	resp, err := dingtalkHttpClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", err
	}
	if resp.StatusCode == http.StatusForbidden &&
		strings.Contains(string(respBody), "AccessTokenPermissionDenied") {
		return "", "", "", fmt.Errorf(
			"用户未授权 Contact.User.Read，请在钉钉应用的【权限管理】中开通【通讯录个人信息读权限(Contact.User.Read)】，并在 OAuth 链接的 scope 中包含 openid 与 Contact.User.Read")
	}

	var sr dingtalkSnsResp
	if err := json.Unmarshal(respBody, &sr); err != nil {
		return "", "", "", fmt.Errorf("解析钉钉用户信息失败: %w (raw=%s)", err, string(respBody))
	}

	userID := strings.TrimSpace(sr.UserId)

	// 兜底策略 1：如果 me 接口未返回 userId，优先尝试通过手机号反查
	if userID == "" && strings.TrimSpace(sr.Mobile) != "" {
		contactToken, err := c.GetContactToken()
		if err == nil {
			uid, err := c.GetUserIdByMobile(contactToken, strings.TrimSpace(sr.Mobile))
			if err == nil && uid != "" {
				userID = uid
			} else {
				base.Error("通过手机号反查 userID 失败:", err)
			}
		} else {
			base.Error("获取通讯录 Token 失败:", err)
		}
	}

	// 兜底策略 2：如果手机号未获取或反查失败，尝试通过 unionId 反查
	if userID == "" && strings.TrimSpace(sr.UnionID) != "" {
		base.Debug("尝试通过 unionId 反查 userId: unionId=", sr.UnionID)
		contactToken, err := c.GetContactToken()
		if err == nil {
			uid, err := c.GetUserIdByUnionId(contactToken, strings.TrimSpace(sr.UnionID))
			if err == nil && uid != "" {
				userID = uid
				base.Debug("通过 unionId 成功反查到 userID=", userID)
			} else {
				base.Error("通过 unionId 反查 userID 失败:", err)
			}
		}
	}

	if userID == "" {
		return "", "", "", fmt.Errorf("钉钉未返回有效 userId (已尝试手机号与 unionId 反查): raw=%s", string(respBody))
	}
	return userID, sr.Nick, accessToken, nil
}

// 通过手机号获取 userid
func (c *DingtalkConfig) GetUserIdByMobile(contactToken, mobile string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"mobile": mobile,
	})
	if err != nil {
		return "", err
	}
	url := "https://oapi.dingtalk.com/topapi/v2/user/getbymobile?access_token=" + contactToken
	resp, err := dingtalkHttpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var dr struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Result  struct {
			Userid string `json:"userid"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &dr); err != nil {
		return "", err
	}
	if dr.ErrCode != 0 {
		return "", fmt.Errorf("手机号查询 userid 失败: %s", dr.ErrMsg)
	}
	return dr.Result.Userid, nil
}

// 通过 unionId 获取 userid
func (c *DingtalkConfig) GetUserIdByUnionId(contactToken, unionId string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"unionid": unionId,
	})
	if err != nil {
		return "", err
	}
	url := "https://oapi.dingtalk.com/topapi/user/getbyunionid?access_token=" + contactToken
	resp, err := dingtalkHttpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var dr struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Result  struct {
			Userid string `json:"userid"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &dr); err != nil {
		return "", err
	}
	if dr.ErrCode != 0 {
		return "", fmt.Errorf("unionId 查询 userid 失败: %s", dr.ErrMsg)
	}
	return dr.Result.Userid, nil
}

// 校验用户是否在某允许部门内（通过通讯录接口查询用户部门）。
func (c *DingtalkConfig) CheckUserDepartment(accessToken, userID string, allowed []string) (bool, error) {
	if len(allowed) == 0 {
		return true, nil
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.dingtalk.com/v1.0/contact/users/"+userID, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("x-acs-dingtalk-access-token", accessToken)
	resp, err := dingtalkHttpClient.Do(req)
	if err != nil {
		return true, nil // 查询失败默认放行
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return true, nil
	}
	var dr dingtalkUserDetailResp
	if err := json.Unmarshal(respBody, &dr); err != nil {
		return true, nil
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, d := range allowed {
		allowedSet[strings.TrimSpace(d)] = true
	}
	for _, d := range dr.DeptIdList {
		if allowedSet[fmt.Sprintf("%d", d)] {
			return true, nil
		}
	}
	return false, nil
}

// 获取通讯录访问令牌（用于用户同步）。
func (c *DingtalkConfig) GetContactToken() (string, error) {
	body, err := json.Marshal(map[string]string{
		"appKey":    c.ClientID,
		"appSecret": c.ClientSecret,
	})
	if err != nil {
		return "", fmt.Errorf("序列化钉钉应用令牌请求失败: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.dingtalk.com/v1.0/oauth2/accessToken", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := dingtalkHttpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var tr struct {
		AccessToken string `json:"accessToken"`
		Code        string `json:"code"`
		Message     string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return "", fmt.Errorf("解析钉钉应用令牌失败: %w (raw=%s)", err, string(respBody))
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("获取钉钉应用令牌失败: code=%s msg=%s", tr.Code, tr.Message)
	}
	return tr.AccessToken, nil
}

// 钉钉部门成员项
type DingtalkDeptUser struct {
	UserId string `json:"userid"`
	Name   string `json:"name"`
	Mobile string `json:"mobile"`
	Email  string `json:"email"`
}

// 拉取钉钉部门成员。recursive=true 时以该部门为根向下遍历所有子部门（含自身），
// recursive=false 时仅拉取该部门直属成员。
func (c *DingtalkConfig) GetDepartmentUsers(contactToken string, deptID string, recursive bool) ([]DingtalkDeptUser, error) {
	return c.getDepartmentUsers(contactToken, deptID, recursive)
}

// 拉取钉钉权限范围内全部用户（从根部门 1 递归所有子部门）
func (c *DingtalkConfig) GetAllUsers(contactToken string) ([]DingtalkDeptUser, error) {
	return c.getDepartmentUsers(contactToken, "1", true)
}

// 获取钉钉指定部门的直属子部门 ID 列表（不含自身）
func (c *DingtalkConfig) getChildDepartmentIDs(contactToken, deptID string) ([]string, error) {
	var ids []string
	url := "https://oapi.dingtalk.com/topapi/v2/department/listsub?access_token=" + contactToken +
		"&dept_id=" + deptID
	resp, err := dingtalkHttpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("获取钉钉子部门列表请求失败: %w", err)
	}
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("读取钉钉子部门列表失败: %w", err)
	}
	var dr struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Result  []struct {
			DeptID int64 `json:"dept_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &dr); err != nil {
		return nil, fmt.Errorf("解析钉钉子部门列表失败: %w", err)
	}
	if dr.ErrCode != 0 {
		return nil, fmt.Errorf("获取钉钉子部门列表失败: %s", dr.ErrMsg)
	}
	for _, d := range dr.Result {
		ids = append(ids, strconv.FormatInt(d.DeptID, 10))
	}
	return ids, nil
}

// 递归收集指定部门及其所有子部门（含自身），用于遍历部门树
func (c *DingtalkConfig) collectDepartmentTree(contactToken, rootID string, deptIDs *[]string, visited map[string]bool) error {
	if visited[rootID] {
		return nil
	}
	visited[rootID] = true
	*deptIDs = append(*deptIDs, rootID)

	childIDs, err := c.getChildDepartmentIDs(contactToken, rootID)
	if err != nil {
		return fmt.Errorf("获取钉钉子部门失败（dept_id=%s）: %w", rootID, err)
	}
	for _, id := range childIDs {
		if err := c.collectDepartmentTree(contactToken, id, deptIDs, visited); err != nil {
			return err
		}
	}
	return nil
}

// 探针：校验通讯录读权限与部门可见范围。
// 钉钉在「无通讯录读权限 / 部门不在应用可见范围」时，user/list 会静默返回空列表（errcode=0），
// 难以定位原因；而 department/get 接口会明确返回 errcode（如 60011 无权限）。
// 探针失败直接返回钉钉原生错误，便于排错。
func (c *DingtalkConfig) probeContactPermission(contactToken, deptID string) error {
	url := "https://oapi.dingtalk.com/topapi/v2/department/get?access_token=" + contactToken +
		"&dept_id=" + deptID
	resp, err := dingtalkHttpClient.Get(url)
	if err != nil {
		return fmt.Errorf("钉钉通讯录权限探针请求失败: %w", err)
	}
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return fmt.Errorf("钉钉通讯录权限探针读取失败: %w", err)
	}
	base.Debug("钉钉通讯录权限探针响应:", string(respBody))
	var dr struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		SubCode string `json:"sub_code"`
		SubMsg  string `json:"sub_msg"`
		Result  any    `json:"result"`
	}
	if err := json.Unmarshal(respBody, &dr); err != nil {
		return fmt.Errorf("解析钉钉通讯录权限探针响应失败: %w", err)
	}
	if dr.ErrCode != 0 {
		msg := dr.ErrMsg
		if dr.SubCode != "" {
			msg += fmt.Sprintf(" (sub_code=%s sub_msg=%s)", dr.SubCode, dr.SubMsg)
		}
		return fmt.Errorf("钉钉通讯录权限不足（dept_id=%s）: errcode=%d %s", deptID, dr.ErrCode, msg)
	}
	return nil
}

func (c *DingtalkConfig) getDepartmentUsers(contactToken, deptID string, recursive bool) ([]DingtalkDeptUser, error) {
	// 权限探针仅对本次同步的入口部门跑一次（无论递归与否），便于在缺通讯录权限时透传原生错误码；
	// 递归遍历到的子部门不再逐个探针，其权限不足会由 user/list 的 errcode 直接暴露。
	if err := c.probeContactPermission(contactToken, deptID); err != nil {
		return nil, err
	}

	// 递归模式：先收集部门树（含自身及所有子部门），再逐个部门拉取直属成员。
	if recursive {
		deptIDs := make([]string, 0)
		if err := c.collectDepartmentTree(contactToken, deptID, &deptIDs, make(map[string]bool)); err != nil {
			return nil, err
		}
		var result []DingtalkDeptUser
		for _, id := range deptIDs {
			users, err := c.fetchDeptUsersOnce(contactToken, id)
			if err != nil {
				base.Error("拉取钉钉子部门成员失败，跳过:", id, err)
				continue
			}
			result = append(result, users...)
		}
		return result, nil
	}

	return c.fetchDeptUsersOnce(contactToken, deptID)
}

// 拉取单个部门的直属成员（不含子部门、不含权限探针）
func (c *DingtalkConfig) fetchDeptUsersOnce(contactToken, deptID string) ([]DingtalkDeptUser, error) {
	var (
		result  []DingtalkDeptUser
		cursor  int64
		hasMore = true
	)
	deptIDInt, err := strconv.Atoi(deptID)
	if err != nil {
		return nil, fmt.Errorf("钉钉部门 ID 必须为数字: %s", deptID)
	}
	for hasMore {
		body, err := json.Marshal(map[string]any{
			"dept_id": deptIDInt,
			"cursor":  cursor,
			"size":    100,
		})
		if err != nil {
			return nil, fmt.Errorf("序列化钉钉请求体失败: %w", err)
		}
		url := "https://oapi.dingtalk.com/topapi/v2/user/list?access_token=" + contactToken
		resp, err := dingtalkHttpClient.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		var dr struct {
			ErrCode int    `json:"errcode"`
			ErrMsg  string `json:"errmsg"`
			Result  struct {
				List       []DingtalkDeptUser `json:"list"`
				HasMore    bool               `json:"has_more"`
				NextCursor int64              `json:"next_cursor"`
			} `json:"result"`
		}
		if err := json.Unmarshal(respBody, &dr); err != nil {
			return nil, fmt.Errorf("解析钉钉部门成员失败: %w", err)
		}
		if dr.ErrCode != 0 {
			return nil, fmt.Errorf("拉取钉钉部门成员失败: %s", dr.ErrMsg)
		}
		if len(dr.Result.List) == 0 && dr.Result.HasMore {
			return nil, fmt.Errorf("拉取钉钉部门成员返回空列表（疑似通讯录读权限或应用可见范围不足）")
		}
		result = append(result, dr.Result.List...)
		hasMore = dr.Result.HasMore
		cursor = dr.Result.NextCursor
	}
	return result, nil
}
