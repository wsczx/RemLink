package authsrv

import (
	"fmt"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/dbdata"
)

// 加载组配置并构建管道，执行首次认证。
func Authenticate(ctx *auth.Context) *auth.PipelineResult {
	profile, err := loadAndParseProfile(ctx.Conn.GroupName)
	if err != nil {
		return &auth.PipelineResult{Result: auth.StepFail, Err: err}
	}

	pipeline, err := auth.GetPipeline(*profile, dbdata.ResolveProviderConfig)
	if err != nil {
		return &auth.PipelineResult{Result: auth.StepFail, Err: err}
	}
	insertForcePwd(pipeline)

	// 管道含 otp 且首步非 local 时，提前加载用户信息供后续步骤共享。
	if profile.HasStep("otp") && profile.Step[0].Type != "local" {
		LoadUserInfo(ctx)
	}

	result, err := pipeline.Run(ctx)
	pr := getPipelineResult(ctx, result, err, pipeline)
	pr.PrevStepIdx = -1
	return pr
}

// 从挂起状态恢复管道，从指定步骤继续。
func Resume(ctx *auth.Context, state auth.PipelineState) *auth.PipelineResult {
	profile, err := loadAndParseProfile(ctx.Conn.GroupName)
	if err != nil {
		return &auth.PipelineResult{Result: auth.StepFail, Err: err}
	}

	pipeline, err := auth.GetPipeline(*profile, dbdata.ResolveProviderConfig)
	if err != nil {
		return &auth.PipelineResult{Result: auth.StepFail, Err: err}
	}
	insertForcePwd(pipeline)

	ctx.SetPassedSteps(state.PassedSteps)
	result, err := pipeline.Resume(ctx, state.StepIdx)
	pr := getPipelineResult(ctx, result, err, pipeline)
	pr.PrevStepIdx = state.StepIdx
	return pr
}

// 将管道执行结果封装为 PipelineResult。
func getPipelineResult(ctx *auth.Context, result auth.StepResult, err error, pipeline *auth.Pipeline) *auth.PipelineResult {
	pr := &auth.PipelineResult{
		Result:    result,
		Err:       err,
		Username:  ctx.Conn.Username,
		GroupName: ctx.Conn.GroupName,
		Info:      ctx.LogInfo(),
	}

	if ctx.UserInfo != nil && ctx.UserInfo.LimitTime != nil {
		pr.LimitTime = ctx.UserInfo.LimitTime
	}

	if result == auth.StepPending {
		if c := pipeline.GetChallenger(); c != nil {
			pr.Challenge = c.Challenge()
		}
		pr.State = auth.PipelineState{
			StepIdx:     pipeline.PendingStep(),
			PassedSteps: ctx.PassedSteps(),
		}
	}

	return pr
}

// 加载用户信息
func loadUser(username string) (*auth.UserInfo, error) {
	u := &dbdata.User{}
	if err := dbdata.One("Username", username, u); err != nil {
		return nil, err
	}
	return u.ToAuthInfo(), nil
}

// 将用户信息写入 ctx.UserInfo（已加载或用户名为空则跳过，避免重复查库；查库失败静默忽略）。
func LoadUserInfo(ctx *auth.Context) {
	if ctx.Conn.Username == "" || ctx.UserInfo != nil {
		return
	}
	if u, err := loadUser(ctx.Conn.Username); err == nil {
		ctx.SetUserInfo(u)
	}
}

// 强制从数据库重新加载用户信息到 ctx.UserInfo
// 用于 resume 场景：用户在断点期间可能已改密或清除强制改密标记，必须以数据库为准
func ReloadUserInfo(ctx *auth.Context) {
	if ctx.Conn.Username == "" {
		return
	}
	if u, err := loadUser(ctx.Conn.Username); err == nil {
		ctx.SetUserInfo(u)
	}
}

// 返回组的 SSO 类型：只要认证管道中包含 SSO 步骤即返回该步骤类型
func GetSSOType(groupName string) string {
	profile, err := loadAndParseProfile(groupName)
	if err != nil || len(profile.Step) == 0 {
		return ""
	}
	for _, step := range profile.Step {
		if auth.Registry.IsSSOType(step.Type) {
			return step.Type
		}
	}
	return ""
}

// 判断证书自动认证是否可行：首步必须为 cert，且后续不能出现需要交互的步骤。
func CertAutoAuth(groupName string) bool {
	profile, err := loadAndParseProfile(groupName)
	if err != nil || len(profile.Step) == 0 {
		return false
	}
	if profile.Step[0].Type != "cert" {
		return false
	}
	for _, step := range profile.Step {
		switch step.Type {
		case "ldap", "local", "radius":
			return false
		}
	}
	return true
}

// 返回 SSO 浏览器模式（"external" 表示使用默认浏览器，否则为空）。
func GetSSOBrowserMode(ssoType, groupName string) string {
	useDefaultBrowser, err := loadSSOConfig(groupName, ssoType)
	if err != nil {
		return ""
	}
	if useDefaultBrowser {
		return "external"
	}
	return ""
}

func loadAndParseProfile(groupName string) (*auth.GroupAuthProfile, error) {
	g, err := loadGroup(groupName)
	if err != nil {
		return nil, err
	}
	return auth.ParseAuthProfile(g.AuthProfile)
}

func loadGroup(name string) (*auth.GroupInfo, error) {
	g := &dbdata.Group{}
	if err := dbdata.One("Name", name, g); err != nil {
		return nil, err
	}
	if g.Status != 1 {
		return nil, fmt.Errorf("用户组(%s)已禁用", name)
	}
	return &auth.GroupInfo{Name: g.Name, AuthProfile: g.AuthProfile}, nil
}

func loadSSOConfig(groupName, ssoType string) (bool, error) {
	switch ssoType {
	case "wxwork":
		cfg, err := dbdata.GetAuthWework(groupName)
		if err != nil {
			return false, err
		}
		return cfg.UseDefaultBrowser, nil
	case "feishu":
		cfg, err := dbdata.GetAuthFeishu(groupName)
		if err != nil {
			return false, err
		}
		return cfg.UseDefaultBrowser, nil
	case "dingtalk":
		cfg, err := dbdata.GetAuthDingtalk(groupName)
		if err != nil {
			return false, err
		}
		return cfg.UseDefaultBrowser, nil
	}
	return false, fmt.Errorf("不支持的 SSO 类型: %s", ssoType)
}
