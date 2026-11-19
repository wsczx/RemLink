package sessdata

import (
	"sync"

	"github.com/wsczx/remlink/base"
)

const limitAllKey = "__ALL__"

// 管理在线客户端并发数计数（全局 + 单用户）
type ClientLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func NewClientLimiter() *ClientLimiter {
	return &ClientLimiter{counts: map[string]int{limitAllKey: 0}}
}

func (l *ClientLimiter) LimitClient(user string, close bool) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	all := l.counts[limitAllKey]
	c, ok := l.counts[user]
	if !ok { // 不存在用户
		l.counts[user] = 0
	}

	if close {
		if c > 0 {
			l.counts[user] = c - 1
		}
		if all > 0 {
			l.counts[limitAllKey] = all - 1
		}
		return true
	}

	// 全局判断
	if all >= base.GetCfg().MaxClient {
		return false
	}

	// 超出同一个用户限制
	if c >= base.GetCfg().MaxUserClient {
		return false
	}

	l.counts[user] = c + 1
	l.counts[limitAllKey] = all + 1
	return true
}
