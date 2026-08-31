package utils

import (
	"maps"
	"sync"
)

type IMaps interface {
	Set(key string, val any)
	Get(key string) (any, bool)
	Del(key string)
	// Items 返回所有条目，用于清理等批量操作
	Items() map[string]any
}

/**
 * 基础的Map结构
 *
 */
type BaseMap struct {
	m  map[string]any
	mu sync.Mutex
}

func (m *BaseMap) Set(key string, value any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = value
}
func (m *BaseMap) Get(key string) (any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.m[key]
	return v, ok
}
func (m *BaseMap) Del(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, key)
}
func (m *BaseMap) Items() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]any, len(m.m))
	maps.Copy(result, m.m)
	return result
}

/**
 * Map 读写结构
 *
 */
type RWLockMap struct {
	m    map[string]any
	lock sync.RWMutex
}

func (m *RWLockMap) Set(key string, value any) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.m[key] = value
}

func (m *RWLockMap) Get(key string) (any, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	v, ok := m.m[key]
	return v, ok
}

func (m *RWLockMap) Del(key string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.m, key)
}

func (m *RWLockMap) Items() map[string]any {
	m.lock.RLock()
	defer m.lock.RUnlock()
	result := make(map[string]any, len(m.m))
	maps.Copy(result, m.m)
	return result
}

/**
 * sync.Map 结构
 *
 */
type SyncMap struct {
	m sync.Map
}

func (m *SyncMap) Set(key string, val any) {
	m.m.Store(key, val)
}

func (m *SyncMap) Get(key string) (any, bool) {
	return m.m.Load(key)
}

func (m *SyncMap) Del(key string) {
	m.m.Delete(key)
}

func (m *SyncMap) Items() map[string]any {
	result := make(map[string]any)
	m.m.Range(func(k, v any) bool {
		result[k.(string)] = v
		return true
	})
	return result
}

func NewMap(name string, len int) IMaps {
	switch name {
	case "cmap":
		return &SyncMap{}
	case "rwmap":
		m := make(map[string]any, len)
		return &RWLockMap{m: m}
	case "syncmap":
		return &SyncMap{}
	default:
		m := make(map[string]any, len)
		return &BaseMap{m: m}
	}
}
