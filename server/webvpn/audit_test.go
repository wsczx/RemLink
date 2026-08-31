package webvpn

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

func TestMain(m *testing.M) {
	base.ReinitLog()
	os.Exit(m.Run())
}

func TestAuditBatcherStopDrainsQueue(t *testing.T) {
	var mu sync.Mutex
	var written []dbdata.WebVpnAudit
	flushed := make(chan struct{}, 1)

	batcher := NewAuditBatcher()
	batcher.write = func(records []dbdata.WebVpnAudit) error {
		mu.Lock()
		written = append(written, records...)
		mu.Unlock()
		select {
		case flushed <- struct{}{}:
		default:
		}
		return nil
	}
	batcher.Start()
	batcher.Log(dbdata.WebVpnAudit{Username: "drain-test"})

	batcher.Stop()

	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("Stop did not flush queued audit record")
	}
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, written, 1)
	assert.Equal(t, "drain-test", written[0].Username)
}

func TestAuditBatcherConcurrentStopAndRestart(t *testing.T) {
	var mu sync.Mutex
	var written []dbdata.WebVpnAudit
	batcher := NewAuditBatcher()
	batcher.write = func(records []dbdata.WebVpnAudit) error {
		mu.Lock()
		written = append(written, records...)
		mu.Unlock()
		return nil
	}
	batcher.Start()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		batcher.Stop()
	}()
	go func() {
		defer wg.Done()
		batcher.Stop()
	}()
	wg.Wait()

	batcher.Start()
	batcher.Log(dbdata.WebVpnAudit{Username: "restart-test"})
	batcher.Stop()

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, written, 1)
	assert.Equal(t, "restart-test", written[0].Username)
}
