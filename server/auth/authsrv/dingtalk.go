package authsrv

import (
	"fmt"

	"github.com/wsczx/remlink/auth"
)

func init() {
	auth.Registry.Register("dingtalk", func() auth.Authenticator {
		return &DingTalkAuth{}
	})
}

type DingTalkAuth struct {
	auth.DingtalkConfig
}

func (a *DingTalkAuth) Name() string { return "dingtalk" }

func (a *DingTalkAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	if sso := ctx.SSO; sso != nil {
		// SSO 回调已完成，用户身份与部门已在回调阶段校验
		if sso.Authenticated && sso.UserID != "" {
			ctx.Conn.Username = sso.UserID
			ctx.SetInfo("用户通过钉钉认证登录")
			return auth.StepPass, nil
		}

		// 管道内 SSO 流程，通过 OAuth code 换取用户信息
		if sso.Code != "" {
			userID, name, accessToken, err := a.GetDingtalkUser(sso.Code)
			if err != nil {
				return auth.StepFail, fmt.Errorf("钉钉认证失败: %w", err)
			}

			// 部门过滤
			allowedDepts := a.ParseDepartments()
			if len(allowedDepts) > 0 {
				ok, err := a.CheckUserDepartment(accessToken, userID, allowedDepts)
				if err != nil {
					return auth.StepFail, fmt.Errorf("验证部门失败: %w", err)
				}
				if !ok {
					return auth.StepFail, fmt.Errorf("用户不在允许的部门范围内")
				}
			}

			// 用户ID拒绝清单：与部门过滤叠加（先过部门白名单，再排除被拒 userid）
			if err := a.CheckUserID(userID, a.ParseBlockedUserIDs()); err != nil {
				return auth.StepFail, err
			}

			ctx.Conn.Username = userID
			ctx.Conn.Nickname = name
			ctx.SetInfo("用户通过钉钉认证登录")
			return auth.StepPass, nil
		}
	}

	ctx.SetInfo("需要通过钉钉 SSO 认证")
	return auth.StepPending, nil
}

func (a *DingTalkAuth) Challenge() *auth.ChallengeInfo {
	return &auth.ChallengeInfo{
		Type:     auth.ChallengeSSO,
		Template: "saml",
		Data: map[string]any{
			"sso_type":            "dingtalk",
			"use_default_browser": a.UseDefaultBrowser,
		},
	}
}
