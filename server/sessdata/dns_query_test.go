package sessdata

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/dbdata"
)

// 构造一个完整的 IPv4 + UDP + DNS 查询报文
func buildV4DNSQuery(t *testing.T, dstIP string, dstPort uint16, qname string, qtype uint16) []byte {
	t.Helper()
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(qname), qtype)
	payload, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	pkt := make([]byte, 20+8+len(payload))
	pkt[0] = 0x45 // IPv4, IHL=5
	binary.BigEndian.PutUint16(pkt[2:4], uint16(len(pkt)))
	binary.BigEndian.PutUint16(pkt[6:8], 0) // 非分片
	pkt[9] = 17                             // UDP
	copy(pkt[12:16], net.ParseIP("100.64.0.2").To4())
	copy(pkt[16:20], net.ParseIP(dstIP).To4())
	binary.BigEndian.PutUint16(pkt[20:22], 53000)
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	binary.BigEndian.PutUint16(pkt[24:26], uint16(8+len(payload)))
	copy(pkt[28:], payload)
	return pkt
}

// 构造一个只用于解析/拦截判定的会话（不触发上游解析与发送）
func newDNSQuerySession(t *testing.T) *ConnSession {
	t.Helper()
	// 只做解析与拦截判定，不分配 FakeIP，故用空管理器绕开 iptables 链初始化
	return &ConnSession{
		FakeDNS: &FakeDNSManager{},
		Policy: &dbdata.Policy{
			EnableFakeDNS:     true,
			ClientDns:         []dbdata.ValData{{Val: "8.8.8.8"}},
			FakeDNSIncludeSet: map[string]struct{}{"example.com": {}},
		},
	}
}

func dnsPayload(data []byte) *Payload {
	return &Payload{LType: LTypeIPData, PType: 0x00, Data: data}
}

func TestParseDNSQueryPacket_AcceptsValidV4Query(t *testing.T) {
	ast := assert.New(t)
	c := newDNSQuerySession(t)
	pkt := buildV4DNSQuery(t, "8.8.8.8", 53, "www.example.com", dns.TypeA)

	ctx, ok := parseDNSQueryPacket(c, dnsPayload(pkt))
	ast.True(ok)
	ast.Equal("www.example.com", ctx.domain)
	ast.Equal(uint16(dns.TypeA), ctx.qType)
	ast.False(ctx.isV6)
	ast.Equal(pkt, ctx.rawIP)
	ast.NotNil(ctx.msg)
}

func TestParseDNSQueryPacket_RejectsMalformed(t *testing.T) {
	c := newDNSQuerySession(t)
	valid := buildV4DNSQuery(t, "8.8.8.8", 53, "www.example.com", dns.TypeA)

	withByte := func(b []byte, idx int, val byte) []byte {
		out := append([]byte{}, b...)
		out[idx] = val
		return out
	}

	cases := []struct {
		name string
		pl   *Payload
	}{
		{"空载荷", dnsPayload(nil)},
		{"非 IPData", &Payload{LType: LTypeIPData, PType: 0x01, Data: valid}},
		{"非法 IHL", dnsPayload(withByte(valid, 0, 0x40))},
		{"报文过短", dnsPayload(valid[:24])},
		{"非 UDP 协议", dnsPayload(withByte(valid, 9, 6))},
		{"非 53 端口", dnsPayload(buildV4DNSQuery(t, "8.8.8.8", 5353, "www.example.com", dns.TypeA))},
		{"非配置 DNS 服务器", dnsPayload(buildV4DNSQuery(t, "1.1.1.1", 53, "www.example.com", dns.TypeA))},
		{"DNS 载荷非法", dnsPayload(append(append([]byte{}, valid[:28]...), 0x00, 0x01))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := parseDNSQueryPacket(c, tc.pl)
			assert.False(t, ok, "应拒绝该报文")
		})
	}
}

func TestParseDNSQueryPacket_RespectsEnableFakeDNS(t *testing.T) {
	ast := assert.New(t)
	c := newDNSQuerySession(t)
	pkt := buildV4DNSQuery(t, "8.8.8.8", 53, "www.example.com", dns.TypeA)

	c.Policy.EnableFakeDNS = false
	_, ok := parseDNSQueryPacket(c, dnsPayload(pkt))
	ast.False(ok, "未启用 FakeDNS 不解析")

	c.Policy.EnableFakeDNS = true
	c.FakeDNS = nil
	_, ok = parseDNSQueryPacket(c, dnsPayload(pkt))
	ast.False(ok, "FakeDNS 实例未注入不解析")
}

func TestInterceptDNS_DoesNotIntercept(t *testing.T) {
	cases := []struct {
		name   string
		qname  string
		qtype  uint16
		mutate func(*ConnSession)
	}{
		{"域名不匹配规则", "www.other.com", dns.TypeA, nil},
		{"非 A/AAAA 查询", "www.example.com", dns.TypeTXT, nil},
		{"AAAA 但双栈未开", "www.example.com", dns.TypeAAAA, nil},
		{"未启用 FakeDNS", "www.example.com", dns.TypeA, func(c *ConnSession) {
			c.Policy.EnableFakeDNS = false
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ast := assert.New(t)
			c := newDNSQuerySession(t)
			if tc.mutate != nil {
				tc.mutate(c)
			}
			pkt := buildV4DNSQuery(t, "8.8.8.8", 53, tc.qname, tc.qtype)
			ast.False(c.InterceptDNS(dnsPayload(pkt)), "不应拦截")
		})
	}
}

func TestShouldUseFakeIPRules(t *testing.T) {
	ast := assert.New(t)

	include := &dbdata.Policy{FakeDNSIncludeSet: map[string]struct{}{"example.com": {}}}
	ast.True(shouldUseFakeIP("www.example.com", include))
	ast.False(shouldUseFakeIP("example.org", include))

	exclude := &dbdata.Policy{FakeDNSExcludeSet: map[string]struct{}{"internal.test": {}}}
	ast.True(shouldUseFakeIP("public.test", exclude))
	ast.False(shouldUseFakeIP("a.internal.test", exclude))

	ast.False(shouldUseFakeIP("example.com", &dbdata.Policy{}), "无规则时不得默认接管")
}

// 超短 IP 报文不得让 FakeIP/ACL/FakeDNS 还原路径越界读取
func TestShortIPPacketsDoNotPanic(t *testing.T) {
	pl := &Payload{LType: LTypeIPData, PType: 0x00, Data: []byte{0x45}}
	c := &ConnSession{
		FakeDNS: &FakeDNSManager{},
		Policy: &dbdata.Policy{
			EnableFakeDNS: true,
			LinkAcl:       []dbdata.GroupLinkAcl{{Val: "10.0.0.0/8", Action: "allow"}},
		},
	}

	if c.IsFakeIPDst(pl) {
		t.Error("超短包不应判定为 FakeIP")
	}
	if !c.CheckLinkAcl(pl) {
		t.Error("超短包在 ACL 路径应放行，交由后续处理")
	}
	if c.RestoreFakeIP(pl) {
		t.Error("超短包不应触发 FakeIP 还原")
	}
}

func TestBuildDNSResponsePacket6_Checksum(t *testing.T) {
	client := net.ParseIP("2001:db8::2") // DNS 查询源（客户端）
	server := net.ParseIP("2001:db8::1") // DNS 查询目的（服务器）
	info := V6HeaderInfo{
		Proto:   17,
		Src:     client,
		Dst:     server,
		SrcPort: 53000,
		DstPort: 53,
		L4Off:   40,
	}
	dnsResp := []byte{0x00, 0x01, 0x81, 0x80, 0x00, 0x01, 0x00, 0x01}
	pkt := buildDNSResponsePacket6(info, dnsResp)

	if len(pkt) != 40+8+len(dnsResp) {
		t.Fatalf("packet len = %d, want %d", len(pkt), 40+8+len(dnsResp))
	}
	if pkt[0]>>4 != 6 {
		t.Errorf("version = %d, want 6", pkt[0]>>4)
	}
	if pkt[6] != 17 {
		t.Errorf("next header = %d, want 17", pkt[6])
	}
	if !net.IP(pkt[8:24]).Equal(server) {
		t.Errorf("src = %v, want %v", net.IP(pkt[8:24]), server)
	}
	if !net.IP(pkt[24:40]).Equal(client) {
		t.Errorf("dst = %v, want %v", net.IP(pkt[24:40]), client)
	}
	if sp, dp := binary.BigEndian.Uint16(pkt[40:42]), binary.BigEndian.Uint16(pkt[42:44]); sp != 53 || dp != 53000 {
		t.Errorf("udp ports = %d/%d, want 53/53000", sp, dp)
	}
	// IPv6 的 UDP 校验和为强制项：伪头 + UDP 载荷的互联网校验和应补码为 0
	cs := uint32(0)
	udp := pkt[40:]
	data := append(append([]byte{}, server...), client...)
	data = append(data, byte(len(udp)>>8), byte(len(udp)), 0, 0x11) // UDP 长度(2) + 0 + next(0x11)
	data = append(data, udp...)
	for i := 0; i+1 < len(data); i += 2 {
		cs += uint32(data[i])<<8 | uint32(data[i+1])
	}
	if len(data)%2 == 1 {
		cs += uint32(data[len(data)-1]) << 8
	}
	for cs>>16 != 0 {
		cs = (cs & 0xffff) + (cs >> 16)
	}
	if ^uint16(cs) != 0 {
		t.Errorf("UDP checksum invalid: complement = 0x%04x, want 0x0000", ^uint16(cs))
	}
}

// 对 v6 文本形态（大小写/零压缩）及 v4 做归一比较
func TestDnsAddrEqual_v6(t *testing.T) {
	cases := []struct {
		cfg  string
		dst  net.IP
		want bool
	}{
		{"2001:DB8::1", net.ParseIP("2001:db8::1"), true},
		{"2001:db8:0:0:0:0:0:1", net.ParseIP("2001:db8::1"), true},
		{"2001:db8::1", net.ParseIP("2001:db8::2"), false},
		{"8.8.8.8", net.ParseIP("8.8.8.8"), true},
		{"8.8.8.8", net.ParseIP("8.8.4.4"), false},
	}
	for _, c := range cases {
		if got := dnsAddrEqual(c.cfg, c.dst); got != c.want {
			t.Errorf("dnsAddrEqual(%q, %v) = %v, want %v", c.cfg, c.dst, got, c.want)
		}
	}
}
