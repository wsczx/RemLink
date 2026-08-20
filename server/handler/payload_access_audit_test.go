package handler

import (
	"net"
	"testing"
	"time"

	"github.com/songgao/water/waterutil"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

// 需最小化初始化审计全局状态（否则 nil panic / channel 阻塞）。
func TestLogAudit_v6_UDP(t *testing.T) {
	auditPayload = &AuditPayload{
		Pool:       utils.NewWorkerPool(1, 16),
		IpAuditMap: utils.NewMap("cmap", 0),
	}
	logBatch = &LogBatch{LogChan: make(chan dbdata.AccessAudit, 16)}

	dst := net.ParseIP("2001:db8::1")
	pkt := buildV6Packet(17, net.ParseIP("2001:db8::2"), dst, 20)
	pkt[40] = 0
	pkt[41] = 0
	pkt[42] = 0
	pkt[43] = 53
	// 用 cap==BufferSize 的 buf 包裹，避免 putPayload 触发 base.Warn（测试环境日志未初始化）
	buf := make([]byte, len(pkt), BufferSize)
	copy(buf, pkt)
	pl := &sessdata.Payload{LType: sessdata.LTypeIPData, PType: 0x00, Data: buf}

	logAudit("user", "group", pl)

	select {
	case audit := <-logBatch.LogChan:
		if audit.Dst != dst.String() {
			t.Errorf("audit.Dst = %q, want %q", audit.Dst, dst.String())
		}
		if audit.Src != "2001:db8::2" {
			t.Errorf("audit.Src = %q, want 2001:db8::2", audit.Src)
		}
		if audit.DstPort != 53 {
			t.Errorf("audit.DstPort = %d, want 53", audit.DstPort)
		}
		if audit.Protocol != uint8(waterutil.UDP) {
			t.Errorf("audit.Protocol = %d, want %d", audit.Protocol, waterutil.UDP)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for v6 audit record")
	}
}
