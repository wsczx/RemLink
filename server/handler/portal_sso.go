package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

func portalSSOGroup(ssoType string) (string, error) {
	groups, err := dbdata.GetAllGroups()
	if err != nil {
		return "", fmt.Errorf("获取用户组失败")
	}
	// 查找纯SSO类型的组
	for _, group := range groups {
		if profile, perr := auth.ParseAuthProfile(group.AuthProfile); perr == nil {
			if len(profile.Step) == 1 && profile.Step[0].Type == ssoType {
				return group.Name, nil
			}
		}
	}
	// 无纯 SSO 组时返回第一个含该类型的组
	for _, group := range groups {
		if dbdata.HasAuthType(group.AuthProfile, ssoType) {
			return group.Name, nil
		}
	}
	return "", fmt.Errorf("未找到可用的第三方登录配置")
}

func portalSSOLogin(w http.ResponseWriter, r *http.Request, pending *AuthSession, ssoType, username string) bool {
	if pending == nil || pending.Ctx == nil {
		return false
	}
	if pending.Ctx.SSO == nil || pending.Ctx.SSO.From != "portal" {
		return false
	}
	if pending.SessionID != "" {
		defer AuthSessionManager.Delete(pending.SessionID)
	}
	user := &dbdata.User{}
	userExists := true
	if err := dbdata.One("Username", username, user); err != nil {
		if dbdata.CheckErrNotFound(err) {
			userExists = false
			user = &dbdata.User{Username: username, Type: ssoType, Status: 1}
		} else {
			SAMLError(w, fmt.Errorf("用户查询失败"))
			return true
		}
	}
	if userExists && user.Status != 1 {
		SAMLError(w, fmt.Errorf("用户不存在或已停用"))
		return true
	}
	group := pending.Ctx.Conn.GroupName
	if group == "" {
		SAMLError(w, fmt.Errorf("认证会话数据异常"))
		return true
	}
	// 确保 token 的 portal_groups 非空：SSO 用户（未同步本地）也需携带认证组，
	if !utils.InArrStr(user.Groups, group) {
		user.Groups = append(user.Groups, group)
	}
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:   username,
			GroupName:  group,
			RemoteAddr: r.RemoteAddr,
			UserAgent:  r.UserAgent(),
		},
		PortalLogin: true,
		SSO: &auth.SSOState{
			Type:          ssoType,
			Authenticated: true,
			UserID:        username,
		},
	}
	authsrv.LoadUserInfo(ctx)
	result := authsrv.Authenticate(ctx)
	switch result.Result {
	case auth.StepPass:
		_ = portalIssueLoginResponse(w, r, user, ctx.LogInfo())
		// 子域名第三方登录：认证成功后回跳原 WebVPN 子域名；否则回门户首页。
		// redirect 只允许本站注册域（.WebVpnDomain 后缀）内的 https URL，杜绝开放重定向。
		if rp := pending.Ctx.SSO.Redirect; webVpnSafeRedirect(rp) {
			http.Redirect(w, r, rp, http.StatusFound)
		} else {
			http.Redirect(w, r, "/ui/#/portal", http.StatusFound)
		}
	case auth.StepPending:
		sessionID := portalSaveChallengeSession(result, ctx)
		http.Redirect(w, r, "/ui/#/portal?session_id="+url.QueryEscape(sessionID)+"&challenge=otp", http.StatusFound)
	default:
		if result.Err != nil {
			SAMLError(w, result.Err)
		} else {
			SAMLError(w, fmt.Errorf("第三方登录失败"))
		}
	}
	return true
}

// 校验 SSO 登录成功后的回跳地址是否安全：
// 必须是 https，且 host 与 WebVpnDomain 主域相同或是其子域（如 app.wv.example.com），
// 防止第三方登录回调被用于开放重定向
func webVpnSafeRedirect(u string) bool {
	if u == "" {
		return false
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" {
		return false
	}
	domain := base.GetCfg().WebVpnDomain
	if domain == "" {
		return false
	}
	host := stripPort(parsed.Host)
	base2 := stripPort(domain)
	if host == "" || base2 == "" {
		return false
	}
	return host == base2 || strings.HasSuffix(host, "."+base2)
}
