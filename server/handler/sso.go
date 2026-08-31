package handler

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

// 钉钉新版 OAuth2 的多权限必须用空格分隔，且多个值之间需要用 %20 来分隔，拼 URL 时不能裸空格直接拼
const dingtalkSSOScope = "openid%20Contact.User.Read"

// 授权 URL 拼装方式与回调路径前缀
// 实际回调地址为 /callbackPath/callback
type ssoProvider struct {
	callbackPath string
	buildAuthURL func(groupName, redirectURI, state string) (string, error)
}

// 新增一种 SSO/OAuth2 认证类型的「最小改动清单」
//  1. 本文件 ssoProviders 加一项（callbackPath + buildAuthURL）——WebAuth / 原生 XML / 门户三端均自动透传 sso_type，无需改这三端
//  2. auth/authsrv/<type>.go 实现 Challenger（Challenge().Type==ChallengeSSO 且 Data 带 sso_type），并注册回调端点 <Type>AuthCallback
//  3. server.go 注册路由 /<callbackPath>/callback → <Type>AuthCallback
//  4. dbdata/provider.go + userauth_<type>.go 注册 Provider 配置与用户同步
//  5. auth/config_<type>.go + base/config.go 加配置结构体
//  6. 如需 [sso,otp] 组合，dbdata/group.go 的 SyncExternalUsersForOTP 加分支
//  7. 前端 Provider / 认证步骤下拉加该类型
var ssoProviders = map[string]ssoProvider{
	"wxwork": {
		callbackPath: "WXAuth",
		buildAuthURL: func(groupName, redirectURI, state string) (string, error) {
			cfg, err := dbdata.GetAuthWework(groupName)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("https://login.work.weixin.qq.com/wwlogin/sso/login?login_type=CorpApp&appid=%s&agentid=%s&redirect_uri=%s&state=%s",
				cfg.CorpID, cfg.AgentID, url.QueryEscape(redirectURI), url.QueryEscape(state)), nil
		}},
	"feishu": {
		callbackPath: "FeishuAuth",
		buildAuthURL: func(groupName, redirectURI, state string) (string, error) {
			cfg, err := dbdata.GetAuthFeishu(groupName)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("https://open.feishu.cn/open-apis/authen/v1/authorize?app_id=%s&redirect_uri=%s&state=%s",
				cfg.AppID, url.QueryEscape(redirectURI), url.QueryEscape(state)), nil
		}},
	"dingtalk": {
		callbackPath: "DingtalkAuth",
		buildAuthURL: func(groupName, redirectURI, state string) (string, error) {
			cfg, err := dbdata.GetAuthDingtalk(groupName)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("https://login.dingtalk.com/oauth2/auth?redirect_uri=%s&response_type=code&client_id=%s&state=%s&scope=%s&prompt=consent",
				url.QueryEscape(redirectURI), cfg.ClientID, url.QueryEscape(state), dingtalkSSOScope), nil
		}},
}

// 统一生成 SSO OAuth 跳转地址
func ssoBuildAuthURL(ssoType, groupName, redirectURI, state string) (string, error) {
	p, ok := ssoProviders[ssoType]
	if !ok {
		return "", fmt.Errorf("不支持的 SSO 类型: %s", ssoType)
	}
	return p.buildAuthURL(groupName, redirectURI, state)
}

// 校验 SAML 回调的待处理会话：state 有效、未被使用
// （防重放）、来源 IP 一致，并使用 pending。返回会话与组名
func verifySAMLPending(w http.ResponseWriter, r *http.Request, state string) (*AuthSession, string, bool) {
	pending, err := AuthSessionManager.Get(state)
	if err != nil {
		base.Error("非法的 SSO state:", state[:min(16, len(state))], ", err:", err)
		SAMLError(w, fmt.Errorf("认证会话已过期或无效"))
		return nil, "", false
	}
	if pending.Ctx.SSO == nil || pending.Ctx.SSO.Authenticated {
		base.Error("SSO state 已被使用（疑似重放）:", state[:min(16, len(state))])
		SAMLError(w, fmt.Errorf("认证会话已过期或无效"))
		return nil, "", false
	}
	// 发起登录与回调须来自同一客户端 IP，否则拒绝冒用
	if pending.Ctx.SSO.ClientIP != "" {
		want, _, _ := net.SplitHostPort(pending.Ctx.SSO.ClientIP)
		got, _, _ := net.SplitHostPort(r.RemoteAddr)
		if want != "" && got != "" && want != got {
			base.Error("SSO state 来源 IP 不匹配:", want, got)
			SAMLError(w, fmt.Errorf("认证会话来源异常"))
			return nil, "", false
		}
	}
	// 消费 pending，避免同一 state 被重复使用
	AuthSessionManager.Delete(state)
	groupname := pending.Ctx.Conn.GroupName
	if groupname == "" {
		base.Error("SSO pending 会话缺少组名")
		SAMLError(w, fmt.Errorf("认证会话数据异常"))
		return nil, "", false
	}
	return pending, groupname, true
}

// 建立 SAML 会话、写入 Cookie 并跳转至断言消费端点
func completeSSO(w http.ResponseWriter, r *http.Request, state, ssoType, groupname, userID string) {
	samlSession := &AuthSession{
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{GroupName: groupname, Username: userID},
			SSO: &auth.SSOState{
				Type:          ssoType,
				Authenticated: true,
				UserID:        userID,
			},
		},
	}
	AuthSessionManager.Save(state, samlSession)

	// 设置 Cookie（Base64 编码 state）
	encodeState := base64.StdEncoding.EncodeToString([]byte(state))
	SetCookie(w, "acSamlv2Token", encodeState, 0)

	http.Redirect(w, r, "/+CSCOE+/saml_ac_login.html", http.StatusFound)
}

// 优先走门户二次登录；否则建立 SAML 会话并完成跳转
func finishSAMLOAuth(w http.ResponseWriter, r *http.Request, state, ssoType, userID string, pending *AuthSession) {
	if portalSSOLogin(w, r, pending, ssoType, userID) {
		return
	}
	completeSSO(w, r, state, ssoType, pending.Ctx.Conn.GroupName, userID)
}

// 生成 SSO state、创建 pending 会话并跳转至厂商授权页
// 回调路径前缀从 ssoProviders 表按 ssoType 取得
func startSSO(w http.ResponseWriter, r *http.Request, tgname, ssoType, redirect string) {
	p, ok := ssoProviders[ssoType]
	if !ok {
		SAMLError(w, fmt.Errorf("不支持的 SSO 类型: %s", ssoType))
		return
	}
	state := utils.RandomRunes(ssoStatePrefixLen)
	pending := &AuthSession{
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{GroupName: tgname},
			SSO: &auth.SSOState{
				Type:     ssoType,
				From:     r.URL.Query().Get("from"),
				ClientIP: r.RemoteAddr,
				Redirect: redirect,
			},
		},
	}
	AuthSessionManager.Save(state, pending)

	redirectURI := fmt.Sprintf("%s/%s/callback", getServerAddr(r), p.callbackPath)
	authURL, err := ssoBuildAuthURL(ssoType, tgname, redirectURI, state)
	if err != nil {
		base.Error("生成 SSO 授权地址失败:", err)
		SAMLError(w, fmt.Errorf("生成认证地址失败"))
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}
