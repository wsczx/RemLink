package base

import (
	"sync"
	"testing"
)

func TestSafeGo(t *testing.T) {
	var wg sync.WaitGroup
	done := make(chan struct{})

	// panic 的 goroutine 不应影响其它 goroutine 与测试进程
	wg.Add(2)
	SafeGo(func() {
		defer wg.Done()
		panic("test panic")
	})
	SafeGo(func() {
		defer wg.Done()
		close(done)
	})

	wg.Wait()
	select {
	case <-done:
	default:
		t.Error("SafeGo 中正常任务未执行")
	}
}
