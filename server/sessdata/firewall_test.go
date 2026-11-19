package sessdata

import (
	"errors"
	"net"
	"testing"

	"github.com/google/nftables/expr"
)

// 验证接口名按 nftables 约定零填充到 16 字节
func TestIfname(t *testing.T) {
	b := ifname("eth0")
	if len(b) != 16 {
		t.Fatalf("ifname len = %d, want 16", len(b))
	}
	if string(b[:4]) != "eth0" {
		t.Fatalf("ifname prefix = %q, want eth0", string(b[:4]))
	}
	for _, c := range b[4:] {
		if c != 0 {
			t.Fatalf("ifname tail not zero-padded: %v", b)
		}
	}
}

// 验证 IPv4/IPv6 的 MASQUERADE 表达式链：
// 源地址匹配偏移/长度随协议族变化，末元素为 Masq
func TestMasqueradeExprs(t *testing.T) {
	n := &NFT{}

	_, v4, _ := net.ParseCIDR("10.0.0.0/24")
	v4exprs := n.masqueradeExprs(v4, net.CIDRMask(24, 32), "eth0", false)
	if len(v4exprs) != 6 {
		t.Fatalf("v4 masquerade exprs len = %d, want 6", len(v4exprs))
	}
	p4, ok := v4exprs[0].(*expr.Payload)
	if !ok || p4.Offset != 12 || p4.Len != 4 {
		t.Fatalf("v4 payload offset/len = %d/%d, want 12/4", p4.Offset, p4.Len)
	}
	if _, ok := v4exprs[5].(*expr.Masq); !ok {
		t.Fatalf("v4 last expr not *expr.Masq")
	}

	_, v6, _ := net.ParseCIDR("fd00::/64")
	v6exprs := n.masqueradeExprs(v6, net.CIDRMask(64, 128), "eth0", true)
	p6, ok := v6exprs[0].(*expr.Payload)
	if !ok || p6.Offset != 8 || p6.Len != 16 {
		t.Fatalf("v6 payload offset/len = %d/%d, want 8/16", p6.Offset, p6.Len)
	}
	if _, ok := v6exprs[5].(*expr.Masq); !ok {
		t.Fatalf("v6 last expr not *expr.Masq")
	}
}

// 验证 FORWARD ACCEPT 表达式链：
// 仅源地址匹配 + Verdict(Accept)，无出接口比较，末元素为 VerdictAccept
func TestForwardAcceptExprs(t *testing.T) {
	n := &NFT{}

	_, v4, _ := net.ParseCIDR("10.0.0.0/24")
	v4exprs := n.forwardAcceptExprs(v4, net.CIDRMask(24, 32), false)
	if len(v4exprs) != 4 {
		t.Fatalf("v4 forward exprs len = %d, want 4", len(v4exprs))
	}
	v, ok := v4exprs[3].(*expr.Verdict)
	if !ok || v.Kind != expr.VerdictAccept {
		t.Fatalf("v4 last expr not VerdictAccept")
	}

	_, v6, _ := net.ParseCIDR("fd00::/64")
	v6exprs := n.forwardAcceptExprs(v6, net.CIDRMask(64, 128), true)
	p6, ok := v6exprs[0].(*expr.Payload)
	if !ok || p6.Offset != 8 || p6.Len != 16 {
		t.Fatalf("v6 payload offset/len = %d/%d, want 8/16", p6.Offset, p6.Len)
	}
	if _, ok := v6exprs[3].(*expr.Verdict); !ok {
		t.Fatalf("v6 last expr not *expr.Verdict")
	}
}

// 验证 nftables "file exists" 幂等忽略判断
func TestIsNftDuplicateError(t *testing.T) {
	if !isNftDuplicateError(errors.New("file exists")) {
		t.Fatal("want true for 'file exists'")
	}
	if isNftDuplicateError(errors.New("some other error")) {
		t.Fatal("want false for unrelated error")
	}
}

// 验证 iptables 删除时“规则本就不存在”的错误识别（幂等删除）
func TestIsIptRuleMissing(t *testing.T) {
	if isIptRuleMissing(nil) {
		t.Fatal("nil error should be false")
	}
	for _, msg := range []string{
		"No such rule",
		"does a matching rule exist",
		"Bad rule",
		"No chain/target/match",
	} {
		if !isIptRuleMissing(errors.New(msg)) {
			t.Fatalf("want true for %q", msg)
		}
	}
	if isIptRuleMissing(errors.New("unrelated failure")) {
		t.Fatal("want false for unrelated error")
	}
}
