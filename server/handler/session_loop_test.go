package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/sessdata"
)

func TestWriteLoopDoesNotCountDownlink(t *testing.T) {
	ast := assert.New(t)
	out := make(chan *sessdata.Payload, 1)
	done := make(chan struct{})
	written := make(chan struct{}, 1)
	recycled := make(chan struct{}, 1)
	cSess := &sessdata.ConnSession{CloseChan: make(chan struct{})}
	pl := &sessdata.Payload{Data: make([]byte, 64)}

	writeFn := func(p *sessdata.Payload) (int, error) {
		written <- struct{}{}
		return len(p.Data), nil
	}
	go runSessionWriteLoop(cSess, out, done, writeFn, func(*sessdata.Payload) { recycled <- struct{}{} }, false)

	out <- pl
	ast.Eventually(func() bool {
		select {
		case <-written:
			return true
		default:
			return false
		}
	}, time.Second, 5*time.Millisecond, "payload 应被写循环取出")
	ast.Eventually(func() bool {
		select {
		case <-recycled:
			return true
		default:
			return false
		}
	}, time.Second, 5*time.Millisecond, "写循环应回收 payload")
	close(done)

	ast.Zero(cSess.BandwidthDown.Load(), "公共写循环不应累计下行流量")
}
