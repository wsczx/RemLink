package authsrv

import (
	"fmt"

	"github.com/wsczx/remlink/auth"
)

type ForcePwd struct {
	ctx *auth.Context
}

func init() {
	auth.Registry.Register("forcepwd", func() auth.Authenticator {
		return &ForcePwd{}
	})
}

func (f *ForcePwd) Name() string { return "forcepwd" }

func (f *ForcePwd) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	f.ctx = ctx
	if ctx.UserInfo == nil {
		LoadUserInfo(ctx)
	}
	if ctx.UserInfo == nil {
		return auth.StepFail, fmt.Errorf("查询用户失败")
	}
	// 仅本地用户参与强制改密；外部认证用户密码在外部系统，透明放行。
	if ctx.UserInfo.Type != "" && ctx.UserInfo.Type != "local" {
		return auth.StepPass, nil
	}
	if !ctx.UserInfo.ForcePwd {
		return auth.StepPass, nil
	}
	return auth.StepPending, nil
}

func (f *ForcePwd) Challenge() *auth.ChallengeInfo {
	if f.ctx == nil {
		return nil
	}
	return &auth.ChallengeInfo{
		Type: auth.ChallengeForcePwd,
		Data: map[string]any{"username": f.ctx.Conn.Username},
	}
}

// 在管道首个 local 步骤之后插入强制改密步骤。
func insertForcePwd(p *auth.Pipeline) {
	for i, step := range p.Steps {
		if step.Name() == "local" {
			factory, ok := auth.Registry.GetFactory("forcepwd")
			if !ok {
				return
			}
			inst := factory()
			p.Steps = append(p.Steps[:i+1], append([]auth.Authenticator{inst}, p.Steps[i+1:]...)...)
			return
		}
	}
}
