package authsrv

import (
	"fmt"

	"github.com/wsczx/remlink/auth"
)

func init() {
	auth.Registry.Register("feishu", func() auth.Authenticator {
		return &FeishuAuth{}
	})
}

type FeishuAuth struct {
	auth.FeishuConfig
}

func (a *FeishuAuth) Name() string { return "feishu" }

func (a *FeishuAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	if sso := ctx.SSO; sso != nil {
		// SSO 回调已完成，用户身份与部门已在回调阶段校验
		if sso.Authenticated && sso.UserID != "" {
			ctx.Conn.Username = sso.UserID
			ctx.SetInfo("用户通过飞书认证登录")
			return auth.StepPass, nil
		}

		// 管道内 SSO 流程，通过 OAuth code 换取用户信息
		if sso.Code != "" {
			userID, name, err := a.GetFeishuUser(sso.Code)
			if err != nil {
				return auth.StepFail, fmt.Errorf("飞书认证失败: %w", err)
			}

			allowedDepts := a.ParseDepartments()
			if len(allowedDepts) > 0 {
				accessToken, err := a.GetAppAccessToken()
				if err != nil {
					return auth.StepFail, fmt.Errorf("获取飞书 access_token 失败: %w", err)
				}
				ok, err := a.CheckUserDepartment(accessToken, userID, allowedDepts)
				if err != nil {
					return auth.StepFail, fmt.Errorf("验证部门失败: %w", err)
				}
				if !ok {
					return auth.StepFail, fmt.Errorf("用户不在允许的部门范围内")
				}
			}

			if err := a.CheckUserID(userID, a.ParseBlockedUserIDs()); err != nil {
				return auth.StepFail, err
			}

			ctx.Conn.Username = userID
			ctx.Conn.Nickname = name
			ctx.SetInfo("用户通过飞书认证登录")
			return auth.StepPass, nil
		}
	}

	ctx.SetInfo("需要通过飞书 SSO 认证")
	return auth.StepPending, nil
}

func (a *FeishuAuth) Challenge() *auth.ChallengeInfo {
	return &auth.ChallengeInfo{
		Type:     auth.ChallengeSSO,
		Template: "saml",
		Data: map[string]any{
			"sso_type":            "feishu",
			"use_default_browser": a.UseDefaultBrowser,
		},
	}
}
