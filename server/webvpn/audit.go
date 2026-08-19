package webvpn

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 异步批量审计：proxy 投递记录到 channel，后台 goroutine 定时（1s）批量写库。
type AuditBatcher struct {
	queue   chan dbdata.WebVpnAudit
	quit    chan struct{}
	done    chan struct{}
	started atomic.Bool
	doneMu  sync.Mutex
}

func NewAuditBatcher() *AuditBatcher {
	return &AuditBatcher{
		queue: make(chan dbdata.WebVpnAudit, 1000),
		quit:  make(chan struct{}),
	}
}

// 非阻塞投递：队列满时普通记录丢弃，risk>=1 降级为本地日志，避免审计阻塞代理主路径。
func (b *AuditBatcher) Log(rec dbdata.WebVpnAudit) {
	select {
	case b.queue <- rec:
	default:
		if rec.RiskLevel >= 1 {
			base.Warn("WebVPN 审计队列已满，降级本地记录:",
				rec.Username, rec.Method, rec.Host, rec.Path, rec.StatusCode, rec.RiskLevel)
		}
	}
}

// 启动后台批处理。
func (b *AuditBatcher) Start() {
	if !b.started.CompareAndSwap(false, true) {
		return
	}
	b.doneMu.Lock()
	b.done = make(chan struct{})
	b.doneMu.Unlock()
	go b.batchWriter()
}

// 停止批处理并等待在途记录落库。
func (b *AuditBatcher) Stop() {
	if !b.started.Load() {
		return
	}
	b.quit <- struct{}{}
	b.doneMu.Lock()
	done := b.done
	b.doneMu.Unlock()
	<-done
	b.started.Store(false)
}

func (b *AuditBatcher) batchWriter() {
	base.Debug("WebVPN 审计批处理已启动")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var batch []dbdata.WebVpnAudit
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := dbdata.AddBatchWebVpnAudit(batch); err != nil {
			base.Warn("WebVPN 审计批量写入失败:", err)
		}
		batch = nil
	}
	for {
		select {
		case rec := <-b.queue:
			batch = append(batch, rec)
			if len(batch) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-b.quit:
			flush()
			base.Debug("WebVPN 审计批处理已退出")
			close(b.done)
			return
		}
	}
}

// 依据状态码给审计风险等级：4xx=1(可疑) 5xx=2(高危) 其余=0。
func RiskOf(statusCode int) int8 {
	switch {
	case statusCode >= 500:
		return 2
	case statusCode >= 400:
		return 1
	default:
		return 0
	}
}
