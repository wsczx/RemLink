package handler

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

// buildTLSClientHello 构造一个最小 TLS ClientHello（仅含 SNI 扩展）。
func buildTLSClientHello(host string) []byte {
	name := []byte(host)
	// ServerName（host_name 类型）
	serverName := append([]byte{0x00}, byte(len(name)>>8), byte(len(name)))
	serverName = append(serverName, name...)
	// server_name_list = 列表长度(2) + ServerName
	sniList := append([]byte{byte(len(serverName) >> 8), byte(len(serverName))}, serverName...)
	// SNI 扩展 = type(2)=0 + 长度(2) + server_name_list
	ext := append([]byte{0x00, 0x00, byte(len(sniList) >> 8), byte(len(sniList))}, sniList...)
	// ClientHello 主体
	body := []byte{0x03, 0x03}
	body = append(body, make([]byte, 32)...)               // random
	body = append(body, 0x00)                              // session_id 长度 0
	body = append(body, 0x00, 0x02, 0xc0, 0x2b)            // 密码套件长度(2) + 一个套件
	body = append(body, 0x01, 0x00)                        // 压缩方法长度(1) + null
	body = append(body, byte(len(ext)>>8), byte(len(ext))) // 扩展段总长
	body = append(body, ext...)
	// Handshake 消息
	hs := append([]byte{0x01, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body...)
	// TLS 记录层
	rec := append([]byte{0x16, 0x03, 0x01, byte(len(hs) >> 8), byte(len(hs))}, hs...)
	return rec
}

// buildTCPSegment 构造一段 TCP 段（20 字节 TCP 头 + 负载）。
func buildTCPSegment(payload []byte) []byte {
	seg := make([]byte, 20+len(payload))
	seg[12] = 0x50 // Data Offset=5 -> 20 字节头
	seg[13] = 0x18 // PSH+ACK
	copy(seg[20:], payload)
	return seg
}

// buildDNSQuery 构造 DNS 查询报文（仅第一个 Question）。
func buildDNSQuery(host string) []byte {
	dns := []byte{0x12, 0x34, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	for l := range strings.SplitSeq(host, ".") {
		dns = append(dns, byte(len(l)))
		dns = append(dns, []byte(l)...)
	}
	dns = append(dns, 0x00)
	dns = append(dns, 0x00, 0x01, 0x00, 0x01)
	return dns
}

// buildV4Raw 构造 IPv4 包：20 字节头 + 上层负载。
func buildV4Raw(proto uint8, src, dst net.IP, payload []byte) []byte {
	pkt := make([]byte, 20+len(payload))
	pkt[0] = 0x45 // version=4, ihl=5
	pkt[9] = proto
	copy(pkt[12:16], src.To4())
	copy(pkt[16:20], dst.To4())
	binary.BigEndian.PutUint16(pkt[2:4], uint16(20+len(payload)))
	copy(pkt[20:], payload)
	return pkt
}

func TestOnTCPExtractsSNI(t *testing.T) {
	rec := buildTLSClientHello("example.com")
	seg := buildTCPSegment(rec)
	proto, info := onTCP(seg)
	if proto != acc_proto_https {
		t.Fatalf("proto = %d, want https(%d)", proto, acc_proto_https)
	}
	if info != "example.com" {
		t.Fatalf("info = %q, want example.com", info)
	}
}

func TestParseDNSQuery(t *testing.T) {
	name := parseDNSQuery(buildDNSQuery("api.weixin.qq.com"))
	if name != "api.weixin.qq.com" {
		t.Fatalf("name = %q, want api.weixin.qq.com", name)
	}
	// 响应报文（QR=1）应忽略
	resp := buildDNSQuery("example.com")
	resp[2] |= 0x80
	if parseDNSQuery(resp) != "" {
		t.Fatal("response packet should not be parsed")
	}
}

// 端到端：v4 HTTPS 访问应提取域名。
func TestLogAudit_v4_HTTPS_Domain(t *testing.T) {
	auditPayload = &AuditPayload{Pool: utils.NewWorkerPool(1, 16), IpAuditMap: utils.NewMap("cmap", 0)}
	logBatch = &LogBatch{LogChan: make(chan dbdata.AccessAudit, 16)}

	pkt := buildV4Raw(6, net.ParseIP("203.0.113.5"), net.ParseIP("203.0.113.1"),
		buildTCPSegment(buildTLSClientHello("www.example.com")))
	buf := make([]byte, len(pkt), BufferSize)
	copy(buf, pkt)
	logAudit("user", "group", &sessdata.Payload{LType: sessdata.LTypeIPData, PType: 0x00, Data: buf})

	select {
	case audit := <-logBatch.LogChan:
		if audit.Info != "www.example.com" {
			t.Fatalf("Info = %q, want www.example.com", audit.Info)
		}
		if audit.AccessProto != acc_proto_https {
			t.Fatalf("AccessProto = %d, want https", audit.AccessProto)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for v4 https audit record")
	}
}

// 端到端：v6 HTTPS 访问应提取域名（此前 v6 不解析域名）。
func TestLogAudit_v6_HTTPS_Domain(t *testing.T) {
	auditPayload = &AuditPayload{Pool: utils.NewWorkerPool(1, 16), IpAuditMap: utils.NewMap("cmap", 0)}
	logBatch = &LogBatch{LogChan: make(chan dbdata.AccessAudit, 16)}

	pkt := buildV6Packet(6, net.ParseIP("2001:db8::2"), net.ParseIP("2001:db8::1"), 20+len(buildTLSClientHello("v6.example.com")))
	copy(pkt[40:], buildTCPSegment(buildTLSClientHello("v6.example.com")))
	buf := make([]byte, len(pkt), BufferSize)
	copy(buf, pkt)
	logAudit("user", "group", &sessdata.Payload{LType: sessdata.LTypeIPData, PType: 0x00, Data: buf})

	select {
	case audit := <-logBatch.LogChan:
		if audit.Info != "v6.example.com" {
			t.Fatalf("Info = %q, want v6.example.com", audit.Info)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for v6 https audit record")
	}
}

// 端到端：v4 DNS 查询应提取被查询域名。
func TestLogAudit_v4_DNS_Domain(t *testing.T) {
	auditPayload = &AuditPayload{Pool: utils.NewWorkerPool(1, 16), IpAuditMap: utils.NewMap("cmap", 0)}
	logBatch = &LogBatch{LogChan: make(chan dbdata.AccessAudit, 16)}

	dns := buildDNSQuery("dns.google.com")
	pkt := buildV4Raw(17, net.ParseIP("203.0.113.5"), net.ParseIP("8.8.8.8"), dns)
	binary.BigEndian.PutUint16(pkt[20:22], 12345)
	binary.BigEndian.PutUint16(pkt[22:24], 53) // 目的端口 53
	buf := make([]byte, len(pkt), BufferSize)
	copy(buf, pkt)
	logAudit("user", "group", &sessdata.Payload{LType: sessdata.LTypeIPData, PType: 0x00, Data: buf})

	select {
	case audit := <-logBatch.LogChan:
		if audit.Info != "dns.google.com" {
			t.Fatalf("Info = %q, want dns.google.com", audit.Info)
		}
		if audit.AccessProto != acc_proto_dns {
			t.Fatalf("AccessProto = %d, want dns", audit.AccessProto)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dns audit record")
	}
}
