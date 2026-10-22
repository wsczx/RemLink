package utils

import (
	"net"
	"testing"
)

func TestIp2long(t *testing.T) {
	if got := Ip2long(net.ParseIP("192.168.1.1")); got != 0xC0A80101 {
		t.Errorf("Ip2long(192.168.1.1) = %x, want c0a80101", got)
	}
	if got := Ip2long(net.ParseIP("0.0.0.0")); got != 0 {
		t.Errorf("Ip2long(0.0.0.0) = %d, want 0", got)
	}
}

// 非法/非 IPv4 输入必须返回 0：IpMap 历史脏数据曾致 IP 池分配 panic
func TestIp2longInvalid(t *testing.T) {
	if got := Ip2long(nil); got != 0 {
		t.Errorf("Ip2long(nil) = %d, want 0", got)
	}
	if got := Ip2long(net.ParseIP("2001:db8::1")); got != 0 {
		t.Errorf("Ip2long(2001:db8::1) = %d, want 0", got)
	}
	if got := Ip2long(net.ParseIP("abc")); got != 0 {
		t.Errorf("Ip2long(abc) = %d, want 0", got)
	}
}
