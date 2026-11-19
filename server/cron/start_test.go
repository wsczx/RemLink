package cron

import (
	"testing"

	"github.com/wsczx/remlink/base"
)

func TestNewCreatesScheduler(t *testing.T) {
	base.Test()
	c, err := New()
	if err != nil {
		t.Fatalf("New 应成功: %v", err)
	}
	if c == nil || c.scheduler == nil {
		t.Fatal("New 未构造调度器")
	}
}

func TestStartAndStop(t *testing.T) {
	base.Test()
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// 停止后再启不应 panic
	c.Stop()
}

func TestStopOnNotStarted(t *testing.T) {
	base.Test()
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// 未 Start 就 Stop 不应 panic
	c.Stop()
}
