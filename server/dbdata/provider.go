package dbdata

import (
	"encoding/json"
	"fmt"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
)

// 返回指定 Provider 类型配置中的敏感字段 key。
func ProvSecretKeys(typ string) []string {
	switch typ {
	case "ldap":
		return []string{"bind_pwd"}
	case "radius":
		return []string{"secret"}
	case "wxwork":
		return []string{"secret"}
	case "feishu":
		return []string{"app_secret"}
	case "dingtalk":
		return []string{"client_secret"}
	}
	return nil
}

// 根据名称和类型查找 Provider 并返回其配置 JSON 的 map 形式。
func ResolveProviderConfig(name, typ string) (map[string]any, error) {
	p := &Provider{}
	if err := One("Name", name, p); err != nil {
		return nil, fmt.Errorf("认证源 %q 不存在: %w", name, err)
	}
	if p.Status != 1 {
		return nil, fmt.Errorf("认证源 %q 已停用", name)
	}
	if p.Type != typ {
		return nil, fmt.Errorf("认证源 %q 类型不匹配（期望 %s，实际 %s）", name, typ, p.Type)
	}
	var cfg map[string]any
	if err := json.Unmarshal(p.Config.Data, &cfg); err != nil {
		return nil, fmt.Errorf("解析 Provider 配置失败: %w", err)
	}
	return cfg, nil
}

// 分页查询所有 Provider
func ProviderList(pageSize, page int) ([]Provider, int, error) {
	var datas []Provider
	count := CountAll(&Provider{})
	err := Find(&datas, pageSize, page)
	return datas, count, err
}

// 返回指定类型且启用状态的 Provider 名称列表。typ 为空时返回全部。
func ProviderNamesByType(typ string) []string {
	var datas []Provider
	where := "status=1"
	args := []any{}
	if typ != "" {
		where += " AND type=?"
		args = append(args, typ)
	}
	if err := FindWhere(&datas, 0, 0, where, args...); err != nil {
		base.Error(err)
		return nil
	}
	names := make([]string, 0, len(datas))
	for _, v := range datas {
		names = append(names, v.Name)
	}
	return names
}
func (p *Provider) NewConfig() (auth.ProviderConfig, error) {
	switch p.Type {
	case "ldap":
		return &auth.LDAPConfig{}, nil
	case "radius":
		return &auth.RADIUSConfig{}, nil
	case "wxwork":
		return &auth.WXWorkConfig{}, nil
	case "feishu":
		return &auth.FeishuConfig{}, nil
	case "dingtalk":
		return &auth.DingtalkConfig{}, nil
	default:
		return nil, fmt.Errorf("不支持的 Provider 类型: %s", p.Type)
	}
}

// 根据类型验证 Provider 配置
func ValidateProviderConfig(p *Provider) error {
	if p.Name == "" {
		return fmt.Errorf("Provider 名称不能为空")
	}
	cfg, err := p.NewConfig()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(p.Config.Data, cfg); err != nil {
		return fmt.Errorf("%s 配置格式无效: %w", p.Type, err)
	}
	return cfg.ValidateConfig()
}

// 新增或更新 Provider。
func SetProvider(p *Provider) error {
	if p.Name == "" {
		return fmt.Errorf("Provider 名称不能为空")
	}
	if p.Type == "" {
		return fmt.Errorf("Provider 类型不能为空")
	}
	if len(p.Config.Data) == 0 {
		return fmt.Errorf("Provider 配置不能为空")
	}
	if err := ValidateProviderConfig(p); err != nil {
		return err
	}
	if p.Status == 0 {
		p.Status = 1
	}

	if p.Id > 0 {
		return Set(p)
	}
	return Add(p)
}

// 删除 Provider
func DelProvider(id int) error {
	if id < 1 {
		return fmt.Errorf("无效的 ID")
	}
	return Del(&Provider{Id: id})
}

// 检查 Provider 是否被任何 Group 的 Pipeline 引用
func CheckProviderInUse(name string) bool {
	var groups []Group
	if err := Find(&groups, 0, 0); err != nil {
		base.Error(err)
		return false
	}
	for _, g := range groups {
		profile, err := auth.ParseAuthProfile(g.AuthProfile)
		if err != nil {
			continue
		}
		for _, step := range profile.Step {
			if step.Provider == name {
				return true
			}
		}
	}
	return false
}
