package auth

import (
	"fmt"
	"sync"
)

// 管理认证器工厂
type ProviderRegistry struct {
	mu      sync.RWMutex
	factory map[string]func() Authenticator
}

var Registry = NewProviderRegistry()

func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		factory: make(map[string]func() Authenticator),
	}
}

// 注册认证器工厂，重复注册会 panic
func (r *ProviderRegistry) Register(name string, factory func() Authenticator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factory[name]; exists {
		panic(fmt.Sprintf("auth: 重复注册 %q", name))
	}
	r.factory[name] = factory
}

// 返回指定名称的认证器工厂
func (r *ProviderRegistry) GetFactory(name string) (func() Authenticator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factory[name]
	return f, ok
}

// 判断认证器是否已注册
func (r *ProviderRegistry) IsRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factory[name]
	return ok
}

// 返回已注册的认证器名称
func (r *ProviderRegistry) RegisteredNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factory))
	for name := range r.factory {
		names = append(names, name)
	}
	return names
}

// 判断认证器是否为 SSO 类型
func (r *ProviderRegistry) IsSSOType(name string) bool {
	factory, ok := r.GetFactory(name)
	if !ok {
		return false
	}
	c, ok := factory().(Challenger)
	if !ok {
		return false
	}
	info := c.Challenge()
	return info != nil && info.Type == ChallengeSSO
}

// 从注册表中移除指定名称，仅用于测试
func unregister(name string) {
	Registry.mu.Lock()
	delete(Registry.factory, name)
	Registry.mu.Unlock()
}
