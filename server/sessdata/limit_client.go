package sessdata

import (
	"sync"

	"github.com/wsczx/remlink/base"
)

const limitAllKey = "__ALL__"

var (
	limitClient = map[string]int{limitAllKey: 0}
	limitMux    = sync.Mutex{}
)

// 返回当前并发连接数与上限，供节点总览展示节点容量
func GetConcurrency() (current, maxClient, maxUserClient int, perUser map[string]int) {
	limitMux.Lock()
	defer limitMux.Unlock()
	c := limitClient[limitAllKey]
	perUser = make(map[string]int, len(limitClient))
	for k, v := range limitClient {
		if k == limitAllKey {
			continue
		}
		perUser[k] = v
	}
	cfg := base.GetCfg()
	return c, cfg.MaxClient, cfg.MaxUserClient, perUser
}

func LimitClient(user string, close bool) bool {
	limitMux.Lock()
	defer limitMux.Unlock()

	_all := limitClient[limitAllKey]
	c, ok := limitClient[user]
	if !ok { // 不存在用户
		limitClient[user] = 0
	}

	if close {
		if c > 0 {
			limitClient[user] = c - 1
		}
		if _all > 0 {
			limitClient[limitAllKey] = _all - 1
		}
		return true
	}

	// 全局判断
	if _all >= base.GetCfg().MaxClient {
		return false
	}

	// 超出同一个用户限制
	if c >= base.GetCfg().MaxUserClient {
		return false
	}

	limitClient[user] = c + 1
	limitClient[limitAllKey] = _all + 1
	return true
}
