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

// 新增 SSO 类型需同时登记：providers 表、authsrv 的 Challenger 与回调路由、
// dbdata 的 Provider 配置与用户同步、base 配置结构、前端下拉选项
type SSOHandler struct {
	providers    map[string]ssoProvider
	callbacks    map[string]func(groupName string) (ssoCallback, error)
	authSessions *AuthSessionManager
	portal       *PortalService // 组装期由 App.UsePortal 接线
}

func (h *SSOHandler) UsePortal(p *PortalService) { h.portal = p }

func NewSSOHandler(authSessions *AuthSessionManager) *SSOHandler {
	return &SSOHandler{
		providers: map[string]ssoProvider{
			"wxwork": {
				callbackPath: "WXAuth",
				buildAuthURL: func(groupName, redirectURI, state string) (string, error) {
					cfg, err := dbdata.Groups.GetAuthWework(groupName)
					if err != nil {
						return "", err
					}
					return fmt.Sprintf("https://login.work.weixin.qq.com/wwlogin/sso/login?login_type=CorpApp&appid=%s&agentid=%s&redirect_uri=%s&state=%s",
						cfg.CorpID, cfg.AgentID, url.QueryEscape(redirectURI), url.QueryEscape(state)), nil
				}},
			"feishu": {
				callbackPath: "FeishuAuth",
				buildAuthURL: func(groupName, redirectURI, state string) (string, error) {
					cfg, err := dbdata.Groups.GetAuthFeishu(groupName)
					if err != nil {
						return "", err
					}
					return fmt.Sprintf("https://open.feishu.cn/open-apis/authen/v1/authorize?app_id=%s&redirect_uri=%s&state=%s",
						cfg.AppID, url.QueryEscape(redirectURI), url.QueryEscape(state)), nil
				}},
			"dingtalk": {
				callbackPath: "DingtalkAuth",
				buildAuthURL: func(groupName, redirectURI, state string) (string, error) {
					cfg, err := dbdata.Groups.GetAuthDingtalk(groupName)
					if err != nil {
						return "", err
					}
					return fmt.Sprintf("https://login.dingtalk.com/oauth2/auth?redirect_uri=%s&response_type=code&client_id=%s&state=%s&scope=%s&prompt=consent",
						url.QueryEscape(redirectURI), cfg.ClientID, url.QueryEscape(state), dingtalkSSOScope), nil
				}},
		},
		callbacks: map[string]func(groupName string) (ssoCallback, error){
			"wxwork": func(groupName string) (ssoCallback, error) {
				cfg, err := dbdata.Groups.GetAuthWework(groupName)
				if err != nil {
					return nil, err
				}
				return &wxworkSSOCallback{cfg: cfg}, nil
			},
			"feishu": func(groupName string) (ssoCallback, error) {
				cfg, err := dbdata.Groups.GetAuthFeishu(groupName)
				if err != nil {
					return nil, err
				}
				return &feishuSSOCallback{cfg: cfg}, nil
			},
			"dingtalk": func(groupName string) (ssoCallback, error) {
				cfg, err := dbdata.Groups.GetAuthDingtalk(groupName)
				if err != nil {
					return nil, err
				}
				return &dingtalkSSOCallback{cfg: cfg}, nil
			},
		},
		authSessions: authSessions,
	}
}

// 统一生成 SSO OAuth 跳转地址
func (h *SSOHandler) BuildAuthURL(ssoType, groupName, redirectURI, state string) (string, error) {
	p, ok := h.providers[ssoType]
	if !ok {
		return "", fmt.Errorf("不支持的 SSO 类型: %s", ssoType)
	}
	return p.buildAuthURL(groupName, redirectURI, state)
}

// 校验 SAML 回调的待处理会话：state 有效、未被使用
// （防重放）、来源 IP 一致，并使用 pending。返回会话与组名
func (h *SSOHandler) verifySAMLPending(w http.ResponseWriter, r *http.Request, state string) (*AuthSession, string, bool) {
	pending, err := h.authSessions.Get(state)
	if err != nil {
		base.Error("非法的 SSO state:", state[:min(16, len(state))], ", err:", err)
		SAMLError(w, fmt.Errorf("认证会话已过期或无效"))
		return nil, "", false
	}
	if pending.Ctx == nil {
		base.Error("SSO pending 会话缺少上下文:", state[:min(16, len(state))])
		SAMLError(w, fmt.Errorf("认证会话已过期或无效"))
		return nil, "", false
	}
	if pending.Ctx.SSO == nil || pending.Ctx.SSO.Authenticated {
		base.Error("SSO state 已被使用（疑似重放）:", state[:min(16, len(state))])
		SAMLError(w, fmt.Errorf("认证会话已过期或无效"))
		return nil, "", false
	}
	// 仅接受 SSO 待回调会话，防止其它类型会话被当作 SSO state 使用
	if pending.Kind != AuthKindSSOPending {
		base.Error("SSO state 类型非法:", state[:min(16, len(state))])
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
	h.authSessions.Delete(state)
	groupname := pending.Ctx.Conn.GroupName
	if groupname == "" {
		base.Error("SSO pending 会话缺少组名")
		SAMLError(w, fmt.Errorf("认证会话数据异常"))
		return nil, "", false
	}
	return pending, groupname, true
}

// 建立 SAML 会话、写入 Cookie 并跳转至断言消费端点
func (h *SSOHandler) completeSSO(w http.ResponseWriter, r *http.Request, state, ssoType, groupname, userID string) {
	samlSession := &AuthSession{
		Kind: AuthKindSSODone,
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{GroupName: groupname, Username: userID},
			SSO: &auth.SSOState{
				Type:          ssoType,
				Authenticated: true,
				UserID:        userID,
			},
		},
	}
	h.authSessions.Save(state, samlSession)

	// 设置 Cookie（Base64 编码 state）
	encodeState := base64.StdEncoding.EncodeToString([]byte(state))
	SetCookie(w, "acSamlv2Token", encodeState, 0)

	http.Redirect(w, r, "/+CSCOE+/saml_ac_login.html", http.StatusFound)
}

// 优先走门户二次登录；否则建立 SAML 会话并完成跳转
func (h *SSOHandler) finishSAMLOAuth(w http.ResponseWriter, r *http.Request, state, ssoType, userID string, pending *AuthSession) {
	if h.portal.portalSSOLogin(w, r, pending, ssoType, userID) {
		return
	}
	h.completeSSO(w, r, state, ssoType, pending.Ctx.Conn.GroupName, userID)
}

// 生成 SSO state、创建 pending 会话并跳转至厂商授权页
// 回调路径前缀从 SSOHandler.providers 表按 ssoType 取得
func (h *SSOHandler) Start(w http.ResponseWriter, r *http.Request, tgname, ssoType, redirect string) {
	p, ok := h.providers[ssoType]
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
	pending.Kind = AuthKindSSOPending // 标记为 SSO 待回调会话
	h.authSessions.Save(state, pending)

	redirectURI := fmt.Sprintf("%s/%s/callback", getServerAddr(r), p.callbackPath)
	authURL, err := h.BuildAuthURL(ssoType, tgname, redirectURI, state)
	if err != nil {
		base.Error("生成 SSO 授权地址失败:", err)
		SAMLError(w, fmt.Errorf("生成认证地址失败"))
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}
