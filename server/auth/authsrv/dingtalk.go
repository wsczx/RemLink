package authsrv

import (
	"github.com/wsczx/remlink/auth"
)

type DingTalkAuth struct {
	auth.DingtalkConfig
}

func (a *DingTalkAuth) Name() string { return "dingtalk" }

func (a *DingTalkAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	var accessToken string
	return ssoStep(ctx, "钉钉",
		func(code string) (string, string, error) {
			uid, name, token, err := a.GetDingtalkUser(code)
			accessToken = token
			return uid, name, err
		},
		func(userID string, depts []string) (bool, error) {
			return a.CheckUserDepartment(accessToken, userID, depts)
		},
		func(userID string, blocked []string) error {
			return a.CheckUserID(userID, blocked)
		},
		a.ParseDepartments,
		a.ParseAllowedUserIDs,
		a.ParseBlockedUserIDs,
	)
}

func (a *DingTalkAuth) Challenge() *auth.ChallengeInfo {
	return ssoChallenge("dingtalk", a.UseDefaultBrowser)
}
