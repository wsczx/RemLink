package auth

import (
	"fmt"
)

// ProviderResolverFunc 将 Provider 名称解析为配置 map。
type ProviderResolverFunc func(name, typ string) (map[string]any, error)

type Pipeline struct {
	Steps       []Authenticator
	pendingStep int // 返回 StepPending 的步骤索引
}

// 根据 GroupAuthProfile 获取认证管道。
func GetPipeline(profile GroupAuthProfile, resolver ProviderResolverFunc) (*Pipeline, error) {
	if len(profile.Step) == 0 {
		return nil, fmt.Errorf("认证配置未包含任何步骤")
	}

	steps := make([]Authenticator, 0, len(profile.Step))
	for i, cfg := range profile.Step {
		factory, ok := Registry.GetFactory(cfg.Type)
		if !ok {
			return nil, fmt.Errorf("未知认证类型 %q（步骤 %d，可用: %v）", cfg.Type, i, Registry.RegisteredNames())
		}

		authInst := factory()

		// 注入 Provider 配置
		if cfg.Provider != "" {
			if resolver == nil {
				return nil, fmt.Errorf("步骤 %d 引用了 Provider %q，但未提供解析器", i, cfg.Provider)
			}
			configMap, err := resolver(cfg.Provider, cfg.Type)
			if err != nil {
				return nil, fmt.Errorf("解析 Provider %q（步骤 %d）失败: %w", cfg.Provider, i, err)
			}
			if len(configMap) > 0 {
				if err := GetProviderConfigFromMap(configMap, authInst); err != nil {
					return nil, fmt.Errorf("解析 %q 配置（步骤 %d）失败: %w", cfg.Type, i, err)
				}
			}
		}

		steps = append(steps, authInst)
	}

	return &Pipeline{Steps: steps}, nil
}

func (p *Pipeline) Run(ctx *Context) (StepResult, error) {
	return p.runFrom(ctx, 0)
}

func (p *Pipeline) Resume(ctx *Context, fromStep int) (StepResult, error) {
	if fromStep < 0 || fromStep >= len(p.Steps) {
		return StepFail, fmt.Errorf("无效的恢复步骤: %d", fromStep)
	}
	return p.runFrom(ctx, fromStep)
}

func (p *Pipeline) PendingStep() int {
	return p.pendingStep
}

func (p *Pipeline) GetChallenger() Challenger {
	if p.pendingStep >= 0 && p.pendingStep < len(p.Steps) {
		if c, ok := p.Steps[p.pendingStep].(Challenger); ok {
			return c
		}
	}
	return nil
}

func (p *Pipeline) runFrom(ctx *Context, start int) (StepResult, error) {
	p.pendingStep = -1

	for i := start; i < len(p.Steps); i++ {
		step := p.Steps[i]

		result, err := step.Authenticate(ctx)
		if err != nil {
			return StepFail, fmt.Errorf("%s: %w", step.Name(), err)
		}

		switch result {
		case StepFail:
			return StepFail, nil
		case StepPending:
			p.pendingStep = i
			return StepPending, nil
		case StepPass:
			if step.Name() != "forcepwd" {
				ctx.AddPassedStep(step.Name())
			}
			continue
		default:
			return StepFail, fmt.Errorf("%s: 未知认证结果 %d", step.Name(), result)
		}
	}

	// 所有步骤通过后，校验证书身份与最终身份一致性
	if ctx.Identity != "" && ctx.Conn.Username != ctx.Identity {
		return StepFail, fmt.Errorf("证书身份(%s)与认证身份(%s)不一致", ctx.Identity, ctx.Conn.Username)
	}

	return StepPass, nil
}
