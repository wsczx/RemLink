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
	pkt[40] = 0x9c
	pkt[41] = 0x40
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
		if audit.SrcPort != 40000 {
			t.Errorf("audit.SrcPort = %d, want 40000", audit.SrcPort)
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

func TestLogBatchAppendAuditUpgradesRawProtocol(t *testing.T) {
	batch := &LogBatch{}
	baseAudit := dbdata.AccessAudit{
		Username: "user", GroupName: "group", Protocol: uint8(waterutil.TCP),
		Src: "10.0.0.2", Dst: "172.16.34.29", DstPort: 80,
		AccessProto: acc_proto_tcp,
	}
	batch.appendAudit(baseAudit)
	batch.appendAudit(dbdata.AccessAudit{
		Username: "user", GroupName: "group", Protocol: uint8(waterutil.TCP),
		Src: "10.0.0.2", Dst: "172.16.34.29", DstPort: 80,
		AccessProto: acc_proto_http, Info: "example.com",
	})
	if len(batch.Logs) != 1 {
		t.Fatalf("len(batch.Logs) = %d, want 1", len(batch.Logs))
	}
	if batch.Logs[0].AccessProto != acc_proto_http || batch.Logs[0].Info != "example.com" {
		t.Fatalf("merged audit = %#v, want HTTP example.com", batch.Logs[0])
	}
}

func TestLogBatchAppendAuditKeepsDifferentDomains(t *testing.T) {
	batch := &LogBatch{}
	for _, domain := range []string{"one.example", "two.example"} {
		batch.appendAudit(dbdata.AccessAudit{
			Username: "user", GroupName: "group", Protocol: uint8(waterutil.TCP),
			Src: "10.0.0.2", Dst: "172.16.34.29", DstPort: 80,
			AccessProto: acc_proto_http, Info: domain,
		})
	}
	if len(batch.Logs) != 2 {
		t.Fatalf("len(batch.Logs) = %d, want 2", len(batch.Logs))
	}
}
