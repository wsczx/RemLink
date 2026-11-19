package authsrv

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/wsczx/remlink/auth"
)

// 是钉钉/飞书/企微三个 SSO provider 共用的认证状态机
// getUser 通过 OAuth code 换取用户；checkDept 校验部门白名单（已绑定 accessToken 等上下文）；
// checkBlocked 校验拒绝名单；parseDepts/parseAllowedUsers/parseBlocked 返回配置中的名单
func ssoStep(ctx *auth.Context, nickname string,
	getUser func(code string) (userID, name string, err error),
	checkDept func(userID string, depts []string) (bool, error),
	checkBlocked func(userID string, blocked []string) error,
	parseDepts func() []string,
	parseAllowedUsers func() []string,
	parseBlocked func() []string,
) (auth.StepResult, error) {
	if sso := ctx.SSO; sso != nil {
		if sso.Authenticated && sso.UserID != "" {
			ctx.Conn.Username = sso.UserID
			ctx.SetInfo("用户通过" + nickname + "认证登录")
			return auth.StepPass, nil
		}
		if sso.Code != "" {
			userID, name, err := getUser(sso.Code)
			if err != nil {
				return auth.StepFail, fmt.Errorf("%s认证失败: %w", nickname, err)
			}
			// 黑名单优先级最高：即使命中白名单也拒绝
			if err := checkBlocked(userID, parseBlocked()); err != nil {
				return auth.StepFail, err
			}
			// 白名单模式：允许的用户 ∪ 允许的部门，任一命中即通过；两者都为空则不限制
			allowedUsers := parseAllowedUsers()
			depts := parseDepts()
			if len(allowedUsers) > 0 && slices.Contains(allowedUsers, userID) {
				// 在允许的用户名单中，直接放行，跳过部门查询
			} else if len(depts) > 0 {
				ok, err := checkDept(userID, depts)
				if err != nil {
					return auth.StepFail, fmt.Errorf("验证部门失败: %w", err)
				}
				if !ok {
					return auth.StepFail, fmt.Errorf("用户不在允许的部门或用户范围内")
				}
			} else if len(allowedUsers) > 0 {
				return auth.StepFail, fmt.Errorf("用户不在允许的用户范围内")
			}
			ctx.Conn.Username = userID
			ctx.Conn.Nickname = name
			ctx.SetInfo("用户通过" + nickname + "认证登录")
			return auth.StepPass, nil
		}
	}
	ctx.SetInfo("需要通过" + nickname + " SSO 认证")
	return auth.StepPending, nil
}

// 构造 SSO 挑战信息，三个 provider 共用
func ssoChallenge(ssoType string, useDefaultBrowser bool) *auth.ChallengeInfo {
	return &auth.ChallengeInfo{
		Type:     auth.ChallengeSSO,
		Template: "saml",
		Data: map[string]any{
			"sso_type":            ssoType,
			"use_default_browser": useDefaultBrowser,
		},
	}
}

// 将整型部门 ID 列表转为字符串，统一部门名单类型
func intsToStrs(ids []int) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = strconv.Itoa(id)
	}
	return out
}

// 将字符串部门 ID 列表解析为整型，供企微 API 使用
func intsFromStrs(ss []string) ([]int, error) {
	out := make([]int, len(ss))
	for i, s := range ss {
		v, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
