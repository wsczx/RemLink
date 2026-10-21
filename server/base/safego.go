package base

import (
	"fmt"
	runtimedebug "runtime/debug"
)

// 启动一个带 panic 兜底的 goroutine
// 后台任务（吊销、审计、同步等）内部 panic 无法被调用方recover，会直接拖垮整个进程
func SafeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Error(fmt.Sprintf("goroutine panic: %v\n%s", r, runtimedebug.Stack()))
			}
		}()
		fn()
	}()
}
