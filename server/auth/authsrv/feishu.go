package authsrv

import (
	"fmt"

	"github.com/wsczx/remlink/auth"
)

// 注册由 RegisterProviders 统一执行，不再使用 init()
type FeishuAuth struct {
	auth.FeishuConfig
}

func (a *FeishuAuth) Name() string { return "feishu" }

func (a *FeishuAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	return ssoStep(ctx, "飞书",
		a.GetFeishuUser,
		func(userID string, depts []string) (bool, error) {
			token, err := a.GetAppAccessToken()
			if err != nil {
				return false, fmt.Errorf("获取飞书 access_token 失败: %w", err)
			}
			return a.CheckUserDepartment(token, userID, depts)
		},
		func(userID string, blocked []string) error {
			return a.CheckUserID(userID, blocked)
		},
		a.ParseDepartments,
		a.ParseBlockedUserIDs,
	)
}

func (a *FeishuAuth) Challenge() *auth.ChallengeInfo {
	return ssoChallenge("feishu", a.UseDefaultBrowser)
}
