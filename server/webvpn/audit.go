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
	queue    chan dbdata.WebVpnAudit
	quit     chan struct{}
	done     chan struct{}
	stateMu  sync.Mutex
	started  atomic.Bool
	stopping bool
	write    func([]dbdata.WebVpnAudit) error
}

func NewAuditBatcher() *AuditBatcher {
	return &AuditBatcher{
		queue: make(chan dbdata.WebVpnAudit, 1000),
		quit:  make(chan struct{}),
		write: dbdata.AddBatchWebVpnAudit,
	}
}

// 非阻塞投递：队列满时普通记录丢弃，risk>=1 降级为本地日志，避免审计阻塞代理主路径。
func (b *AuditBatcher) Log(rec dbdata.WebVpnAudit) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if !b.started.Load() || b.stopping {
		return
	}
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
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.started.Load() || b.stopping {
		return
	}
	b.quit = make(chan struct{})
	b.done = make(chan struct{})
	b.started.Store(true)
	go b.batchWriter(b.quit, b.done)
}

// 停止批处理并等待在途记录落库。
func (b *AuditBatcher) Stop() {
	b.stateMu.Lock()
	if !b.started.Load() {
		b.stateMu.Unlock()
		return
	}
	done := b.done
	if b.stopping {
		b.stateMu.Unlock()
		<-done
		return
	}
	b.stopping = true
	quit := b.quit
	b.stateMu.Unlock()
	close(quit)
	<-done
	b.stateMu.Lock()
	b.started.Store(false)
	b.stopping = false
	b.stateMu.Unlock()
}

func (b *AuditBatcher) batchWriter(quit <-chan struct{}, done chan<- struct{}) {
	base.Debug("WebVPN 审计批处理已启动")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var batch []dbdata.WebVpnAudit
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := b.write(batch); err != nil {
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
		case <-quit:
			for {
				select {
				case rec := <-b.queue:
					batch = append(batch, rec)
				default:
					flush()
					base.Debug("WebVPN 审计批处理已退出")
					close(done)
					return
				}
			}
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
