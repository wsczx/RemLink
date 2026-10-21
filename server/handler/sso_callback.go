package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 三端（原生 XML 回调 / WebAuth 回调 / 门户）共用
type ssoCallback interface {
	// getUser 用授权码换取 (用户ID, 昵称)
	getUser(code string) (userID, name string, err error)
	// checkDept 部门过滤（内部自行判断是否配置了允许部门；未配置直接通过）
	checkDept(userID string) error
	// isBlocked 用户是否在拒绝名单中
	isBlocked(userID string) bool
}

// SSO 源回调适配器注册表（与 sso.go 的 ssoProviders 对称）
// 新增 SSO 类型时在此加一项，三端回调自动获得支持
var ssoCallbacks = map[string]func(groupName string) (ssoCallback, error){
	"wxwork": func(groupName string) (ssoCallback, error) {
		cfg, err := dbdata.GetAuthWework(groupName)
		if err != nil {
			return nil, err
		}
		return &wxworkSSOCallback{cfg: cfg}, nil
	},
	"feishu": func(groupName string) (ssoCallback, error) {
		cfg, err := dbdata.GetAuthFeishu(groupName)
		if err != nil {
			return nil, err
		}
		return &feishuSSOCallback{cfg: cfg}, nil
	},
	"dingtalk": func(groupName string) (ssoCallback, error) {
		cfg, err := dbdata.GetAuthDingtalk(groupName)
		if err != nil {
			return nil, err
		}
		return &dingtalkSSOCallback{cfg: cfg}, nil
	},
}

// 按 SSO 类型取回调适配器
func ssoCallbackFor(ssoType, groupName string) (ssoCallback, error) {
	resolve, ok := ssoCallbacks[ssoType]
	if !ok {
		return nil, fmt.Errorf("不支持的 SSO 类型: %s", ssoType)
	}
	return resolve(groupName)
}

var errSSOBlocked = errors.New("用户已被拒绝登录")

// 执行部门过滤 + 拒绝名单校验（SAML/门户 与 WebAuthn 回调共用）
// 返回非 nil 表示拒绝登录；由调用方决定如何响应（SAMLError / http.Error）
func ssoAuthorize(cb ssoCallback, userID string) error {
	if err := cb.checkDept(userID); err != nil {
		return err
	}
	if cb.isBlocked(userID) {
		return errSSOBlocked
	}
	return nil
}

// 处理 SSO 回调的公共流程
// 参数校验 → state 校验（防重放/IP 绑定）→ 换取用户 → 部门过滤 →
// 拒绝名单 → 完成登录
func ssoCallbackHandler(w http.ResponseWriter, r *http.Request, ssoType, name string) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		base.Error(name + "认证回调缺少参数")
		http.Error(w, "缺少认证参数", http.StatusBadRequest)
		return
	}

	pending, groupname, ok := verifySAMLPending(w, r, state)
	if !ok {
		return
	}

	cb, err := ssoCallbackFor(ssoType, groupname)
	if err != nil {
		base.Error("获取"+name+"配置失败", err)
		SAMLError(w, err)
		return
	}

	userID, _, err := cb.getUser(code)
	if err != nil {
		base.Error(name+"用户信息获取失败", err)
		SAMLError(w, err)
		return
	}

	// 部门过滤 + 拒绝名单：在回调阶段完成校验，避免后续管道绕过
	if err := ssoAuthorize(cb, userID); err != nil {
		base.Error(name+"访问控制校验失败:", err)
		SAMLError(w, err)
		return
	}

	finishSAMLOAuth(w, r, state, ssoType, userID, pending)
}

// ---- 各源适配器 ----

type wxworkSSOCallback struct {
	cfg *dbdata.AuthWXwork
}

func (c *wxworkSSOCallback) getUser(code string) (string, string, error) {
	return c.cfg.GetWeworkUser(code)
}

func (c *wxworkSSOCallback) checkDept(userID string) error {
	allowedDepts := c.cfg.ParseDepartments()
	if len(allowedDepts) == 0 {
		return nil
	}
	ok, err := c.cfg.CheckUserDepartment(userID, allowedDepts)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("用户不在允许的部门范围内")
	}
	return nil
}

func (c *wxworkSSOCallback) isBlocked(userID string) bool {
	blocked := c.cfg.ParseBlockedUserIDs()
	return len(blocked) > 0 && c.cfg.CheckUserID(userID, blocked)
}

type feishuSSOCallback struct {
	cfg *dbdata.AuthFeishu
}

func (c *feishuSSOCallback) getUser(code string) (string, string, error) {
	return c.cfg.GetFeishuUser(code)
}

func (c *feishuSSOCallback) checkDept(userID string) error {
	allowedDepts := c.cfg.ParseDepartments()
	if len(allowedDepts) == 0 {
		return nil
	}
	accessToken, err := c.cfg.GetAppAccessToken()
	if err != nil {
		return err
	}
	ok, err := c.cfg.CheckUserDepartment(accessToken, userID, allowedDepts)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("用户不在允许的部门范围内")
	}
	return nil
}

func (c *feishuSSOCallback) isBlocked(userID string) bool {
	return c.cfg.CheckUserID(userID, c.cfg.ParseBlockedUserIDs()) != nil
}

type dingtalkSSOCallback struct {
	cfg   *dbdata.AuthDingtalk
	token string // getUser 时由 GetDingtalkUser 返回，供 checkDept 使用
}

func (c *dingtalkSSOCallback) getUser(code string) (string, string, error) {
	userID, name, token, err := c.cfg.GetDingtalkUser(code)
	c.token = token
	return userID, name, err
}

func (c *dingtalkSSOCallback) checkDept(userID string) error {
	allowed := c.cfg.ParseDepartments()
	if len(allowed) == 0 {
		return nil
	}
	ok, err := c.cfg.CheckUserDepartment(c.token, userID, allowed)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("用户不在允许的部门范围内")
	}
	return nil
}

func (c *dingtalkSSOCallback) isBlocked(userID string) bool {
	return c.cfg.CheckUserID(userID, c.cfg.ParseBlockedUserIDs()) != nil
}
