package authsrv

import (
	"fmt"

	"github.com/wsczx/remlink/auth"
)

type WXWorkAuth struct {
	auth.WXWorkConfig
}

func (a *WXWorkAuth) Name() string { return "wxwork" }

func (a *WXWorkAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	return ssoStep(ctx, "企业微信",
		a.GetWeworkUser,
		func(userID string, depts []string) (bool, error) {
			ids, err := intsFromStrs(depts)
			if err != nil {
				return false, err
			}
			return a.CheckUserDepartment(userID, ids)
		},
		func(userID string, blocked []string) error {
			if len(blocked) > 0 && a.CheckUserID(userID, blocked) {
				return fmt.Errorf("用户在被拒绝的用户ID列表中")
			}
			return nil
		},
		func() []string { return intsToStrs(a.ParseDepartments()) },
		a.ParseBlockedUserIDs,
	)
}

func (a *WXWorkAuth) Challenge() *auth.ChallengeInfo {
	return ssoChallenge("wxwork", a.UseDefaultBrowser)
}
