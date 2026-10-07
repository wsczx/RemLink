// Package ticket 实现 WebVPN 长连接（WebSocket）建立用的一次性短时票据。
//
// 票据为 256bit 随机值，仅在内存中保存，默认 30s 过期、兑换一次即作废；
// 绑定 sid / 用户 / 应用 / 客户端 IP，兑换时逐项比对，防止票据被转移使用。
package ticket

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

const DefaultTTL = 30 * time.Second

var (
	ErrInvalid  = errors.New("票据无效或已使用")
	ErrExpired  = errors.New("票据已过期")
	ErrMismatch = errors.New("票据与当前请求不匹配")
)

// 票据绑定的上下文
type Claims struct {
	Sid      string
	Username string
	App      string
	ClientIP string
	Purpose  string // 如 "term"
}

type entry struct {
	c   Claims
	exp time.Time
}

type Store struct {
	mu  sync.Mutex
	m   map[string]entry
	ttl time.Duration
	now func() time.Time
	max int
}

func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Store{m: make(map[string]entry), ttl: ttl, now: time.Now, max: 100000}
}

// 签发票据
func (s *Store) Issue(c Claims) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := base64.RawURLEncoding.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if len(s.m) >= s.max {
		s.gcLocked(now)
		if len(s.m) >= s.max {
			return "", errors.New("票据过多，请稍后重试")
		}
	}
	s.m[t] = entry{c: c, exp: now.Add(s.ttl)}
	return t, nil
}

// 兑换票据：无论成功与否，票据均被删除（单次）；expect 中非空字段逐项比对
func (s *Store) Consume(t string, expect Claims) (Claims, error) {
	if t == "" {
		return Claims{}, ErrInvalid
	}
	s.mu.Lock()
	e, ok := s.m[t]
	delete(s.m, t)
	now := s.now()
	if len(s.m) > 256 {
		s.gcLocked(now)
	}
	s.mu.Unlock()
	if !ok {
		return Claims{}, ErrInvalid
	}
	if now.After(e.exp) {
		return Claims{}, ErrExpired
	}
	eq := func(want, got string) bool {
		return want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
	}
	if !eq(expect.Sid, e.c.Sid) || !eq(expect.Username, e.c.Username) || !eq(expect.App, e.c.App) ||
		!eq(expect.ClientIP, e.c.ClientIP) || !eq(expect.Purpose, e.c.Purpose) {
		return Claims{}, ErrMismatch
	}
	return e.c, nil
}

// 当前未过期票据数（测试/监控用）
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(s.now())
	return len(s.m)
}

func (s *Store) gcLocked(now time.Time) {
	for k, e := range s.m {
		if now.After(e.exp) {
			delete(s.m, k)
		}
	}
}
