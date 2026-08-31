package authsrv

import (
	"fmt"

	"github.com/wsczx/remlink/auth"
)

func init() {
	auth.Registry.Register("wxwork", func() auth.Authenticator {
		return &WXWorkAuth{}
	})
}

type WXWorkAuth struct {
	auth.WXWorkConfig
}

func (a *WXWorkAuth) Name() string { return "wxwork" }

func (a *WXWorkAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	if sso := ctx.SSO; sso != nil {
		// SSO 回调已完成，用户身份 + 部门在回调阶段校验
		// handleSsoToken 从 SSO 会话恢复此标记，此处直接放行
		if sso.Authenticated && sso.UserID != "" {
			ctx.Conn.Username = sso.UserID
			ctx.SetInfo("用户通过企业微信认证登录")
			return auth.StepPass, nil
		}

		// 管道内 SSO 流程，通过 OAuth code 换取用户信息
		if sso.Code != "" {
			userID, name, err := a.GetWeworkUser(sso.Code)
			if err != nil {
				return auth.StepFail, fmt.Errorf("企业微信认证失败: %w", err)
			}

			allowedDepts := a.ParseDepartments()
			if len(allowedDepts) > 0 {
				ok, err := a.CheckUserDepartment(userID, allowedDepts)
				if err != nil {
					return auth.StepFail, fmt.Errorf("验证部门失败: %w", err)
				}
				if !ok {
					return auth.StepFail, fmt.Errorf("用户不在允许的部门范围内")
				}
			}

			// 用户ID拒绝清单：与部门过滤叠加（先过部门白名单，再排除被拒 userid）
			blockedUserIDs := a.ParseBlockedUserIDs()
			if len(blockedUserIDs) > 0 && a.CheckUserID(userID, blockedUserIDs) {
				return auth.StepFail, fmt.Errorf("用户在被拒绝的用户ID列表中")
			}

			ctx.Conn.Username = userID
			ctx.Conn.Nickname = name
			ctx.SetInfo("用户通过企业微信认证登录")
			return auth.StepPass, nil
		}
	}

	ctx.SetInfo("需要通过企业微信 SSO 认证")
	return auth.StepPending, nil
}

func (a *WXWorkAuth) Challenge() *auth.ChallengeInfo {
	return &auth.ChallengeInfo{
		Type:     auth.ChallengeSSO,
		Template: "saml",
		Data: map[string]any{
			"sso_type":            "wxwork",
			"use_default_browser": a.UseDefaultBrowser,
		},
	}
}
