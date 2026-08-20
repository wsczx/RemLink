package handler

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestBuildDNSResponsePacket6_Checksum(t *testing.T) {
	client := net.ParseIP("2001:db8::2") // DNS 查询源（客户端）
	server := net.ParseIP("2001:db8::1") // DNS 查询目的（服务器）
	info := v6HeaderInfo{
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

// dnsAddrEqual 对 v6 文本形态（大小写/零压缩）及 v4 的归一比较。
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
