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

// 非法/非 IPv4 输入必须返回 0 而非 panic
// 脏数据（如 IpMap 中历史非法 ip_addr）进入 IP 池分配时曾导致
// binary.BigEndian.Uint32(nil) panic，使对应客户端登录即崩溃
func TestIp2longInvalid(t *testing.T) {
	// 不应 panic
	if got := Ip2long(nil); got != 0 {
		t.Errorf("Ip2long(nil) = %d, want 0", got)
	}
	// IPv6 地址 To4() 为 nil
	if got := Ip2long(net.ParseIP("2001:db8::1")); got != 0 {
		t.Errorf("Ip2long(2001:db8::1) = %d, want 0", got)
	}
	// 非法字符串 ParseIP 返回 nil
	if got := Ip2long(net.ParseIP("abc")); got != 0 {
		t.Errorf("Ip2long(abc) = %d, want 0", got)
	}
}
