package handler

import (
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/songgao/water/waterutil"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

// 构造 IPv6 报文，用于审计路径的地址族解析
func buildV6Packet(nextHeader uint8, src, dst net.IP, payloadLen int) []byte {
	data := make([]byte, 40+payloadLen)
	data[0] = 0x60 // 版本=6
	binary.BigEndian.PutUint16(data[4:6], uint16(payloadLen))
	data[6] = nextHeader
	copy(data[8:24], src.To16())
	copy(data[24:40], dst.To16())
	return data
}

// 需最小化初始化审计全局状态（否则 nil panic / channel 阻塞）
func TestLogAudit_v6_UDP(t *testing.T) {
	aa := NewAccessAudit(sessdata.NewSessionManager())
	aa.Payload = &AuditPayload{
		Pool:       utils.NewWorkerPool(1, 16),
		IpAuditMap: &sync.Map{},
	}
	aa.Batch = &LogBatch{LogChan: make(chan dbdata.AccessAudit, 16)}

	dst := net.ParseIP("2001:db8::1")
	pkt := buildV6Packet(17, net.ParseIP("2001:db8::2"), dst, 20)
	pkt[40] = 0x9c
	pkt[41] = 0x40
	pkt[42] = 0
	pkt[43] = 53
	// 用 cap==sessdata.PayloadBufferSize 的 buf 包裹，避免 PutPayload 触发 base.Warn（测试环境日志未初始化）
	buf := make([]byte, len(pkt), sessdata.PayloadBufferSize)
	copy(buf, pkt)
	pl := &sessdata.Payload{LType: sessdata.LTypeIPData, PType: 0x00, Data: buf}

	aa.log("user", "group", pl)

	select {
	case audit := <-aa.Batch.LogChan:
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

// 解帧后的长度由客户端控制；超短报文不得触发越界读取
func TestParseAuditPacketRejectsShortPackets(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		{0x45},
		make([]byte, 19),
		{0x45, 0, 0, 20, 0, 0, 0, 0, 64, 17, 0, 0}, // 12 字节，IHL 合法但不足 20
	}
	for i, data := range cases {
		pl := &sessdata.Payload{LType: sessdata.LTypeIPData, PType: 0x00, Data: data}
		if _, ok := parseAuditPacket(pl); ok {
			t.Errorf("case %d (len=%d) 应被拒绝", i, len(data))
		}
	}
}
