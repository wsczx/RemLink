package auth

import (
	"encoding/json"
	"fmt"
)

// 定义认证步骤及执行顺序
type GroupAuthProfile struct {
	Step []AuthMethodConfig `json:"step"`
}

// 定义单个认证步骤
type AuthMethodConfig struct {
	Type     string `json:"type"`               // 认证器名称："local","ldap","radius","cert","otp","wxwork"
	Provider string `json:"provider,omitempty"` // 引用已命名的 Provider（如"北京LDAP"），配置由 ProviderResolver 解析
}

// 判断是否包含指定类型的步骤
func (p *GroupAuthProfile) HasStep(typ string) bool {
	for _, s := range p.Step {
		if s.Type == typ {
			return true
		}
	}
	return false
}

// Provider 配置的统一接口
type ProviderConfig interface {
	ValidateConfig() error
}

// 解析认证配置
func ParseAuthProfile(raw json.RawMessage) (*GroupAuthProfile, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("认证配置为空")
	}
	var profile GroupAuthProfile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return nil, fmt.Errorf("解析认证配置失败: %w", err)
	}
	if len(profile.Step) == 0 {
		return nil, fmt.Errorf("认证步骤为空")
	}
	if profile.Step[0].Type == "otp" {
		return nil, fmt.Errorf("第一步不能是 OTP 认证，OTP 依赖前置步骤提供用户名")
	}
	if hasRadiusAndOTP(profile.Step) {
		return nil, fmt.Errorf("radius 和 otp 不能同时配置: Radius 服务端自带 TOTP 时会与 remlink 本地 OTP 冲突")
	}
	if hasSSOAndCredential(profile.Step) {
		return nil, fmt.Errorf("SSO 认证(企业微信/飞书)不能与本地密码、LDAP、RADIUS 等凭据认证同时使用：SSO 由第三方身份提供，不产生登录密码，无法与需要密码的步骤组合")
	}
	return &profile, nil
}

// 将配置映射到目标结构体
func GetProviderConfigFromMap(cfg map[string]any, target any) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	return json.Unmarshal(b, target)
}

// 检查是否同时配置 RADIUS 和 OTP
func hasRadiusAndOTP(steps []AuthMethodConfig) bool {
	hasRadius, hasOtp := false, false
	for _, s := range steps {
		if s.Type == "radius" {
			hasRadius = true
		}
		if s.Type == "otp" {
			hasOtp = true
		}
	}
	return hasRadius && hasOtp
}

// 检查认证管道是否同时包含 SSO 类型与需要密码的凭据类型。
// SSO 由第三方身份提供、不产生登录密码，无法与需要密码的步骤(local/ldap/radius)组合。
func hasSSOAndCredential(steps []AuthMethodConfig) bool {
	hasSSO, hasCred := false, false
	for _, s := range steps {
		if Registry.IsSSOType(s.Type) {
			hasSSO = true
		}
		if s.Type == "local" || s.Type == "ldap" || s.Type == "radius" {
			hasCred = true
		}
	}
	return hasSSO && hasCred
}
