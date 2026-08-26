package sessdata

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/coreos/go-iptables/iptables"
	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"github.com/wsczx/remlink/base"
)

const (
	nftGlobalNatTable   = "REMLINK_GLOBAL_NAT"
	nftPostRoutingChain = "REMLINK_GLOBAL_NAT_POSTROUTING"
	nftForwardChain     = "REMLINK_GLOBAL_NAT_FORWARD"

	nftGlobalNatTable6   = "REMLINK_GLOBAL_NAT6"
	nftPostRoutingChain6 = "REMLINK_GLOBAL_NAT6_POSTROUTING"
	nftForwardChain6     = "REMLINK_GLOBAL_NAT6_FORWARD"

	nftTableName             = "REMLINK_FAKEIP"
	nftDnatMapName           = "REMLINK_FAKEIP_DNATMAP"
	nftFakeIPPreRoutingChain = "REMLINK_FAKEIP_PREROUTING"

	nftTableName6             = "REMLINK_FAKEIP6"
	nftDnatMapName6           = "REMLINK_FAKEIP6_DNATMAP"
	nftFakeIPPreRoutingChain6 = "REMLINK_FAKEIP6_PREROUTING"

	iptDnatChain = "REMLINK_FAKEIP_DNAT"

	groupNATV4MagicPrefix = "REMLINK_GNAT_V4:"
	groupNATV6MagicPrefix = "REMLINK_GNAT_V6:"
)

// 防火墙后端接口
type Firewall interface {
	CreateChains(vpnCIDR, fakeIPRange string) error                     // 创建自定义链
	AddNatRule(fakeIP, realIP string) error                             // 添加 DNAT 规则（回包由 conntrack 自动反向）
	DelNatRule(fakeIP, realIP string) error                             // 删除 DNAT 规则
	SetupGlobalNAT(vpnCIDR, masterDev string, inContainer bool) error   // 设置全局 NAT 规则
	SetupGlobalNAT6(vpnCIDR6, masterDev string, inContainer bool) error // 设置全局 IPv6 NAT/转发规则
	AddGroupNAT(groupCIDR, masterDev string, inContainer bool) error    // 为组自定义 CIDR 添加 NAT/转发规则
	AddGroupNAT6(groupCIDR6, masterDev string, inContainer bool) error  // 为组自定义 v6 CIDR 添加 NAT66/stateful FORWARD 规则
	DelGroupNAT(groupCIDR, masterDev string, inContainer bool) error    // 删除组自定义 CIDR 的 NAT/转发规则
	DelGroupNAT6(groupCIDR6, masterDev string, inContainer bool) error  // 删除组自定义 v6 CIDR 的 NAT66/stateful FORWARD 规则
	CleanupFakeIP() error                                               // 清理所有fakeIP规则
	CleanupGlobal() error                                               // 清理全局NAT规则
}

// 全局单例
var (
	GlobalFirewall     Firewall
	GlobalFirewallMu   sync.Mutex
	GlobalFirewallDone bool
)

// 记录组网段已使用的出网网卡，用于去重和配置变更时清理旧规则
var (
	groupNatCIDRs sync.Map
	groupNatMu    sync.Mutex
)

// 获取全局防火墙实例
func GetFirewall() Firewall {
	GlobalFirewallMu.Lock()
	defer GlobalFirewallMu.Unlock()

	if GlobalFirewallDone {
		return GlobalFirewall
	}

	fw, err := newFirewall()
	if err != nil {
		base.Error("Failed to initialize firewall:", err)
		return nil
	}
	GlobalFirewall = fw
	GlobalFirewallDone = true
	return GlobalFirewall
}
func newFirewall() (Firewall, error) {
	cfg := base.GetCfg()
	driver := cfg.FirewallDriver
	if driver == "" {
		driver = "auto"
	}

	switch driver {
	case "iptables":
		ipt, err := NewIPT()
		if err != nil {
			return nil, fmt.Errorf("iptables initialization failed:%v", err)
		}
		base.Info("Firewall driver: using iptables")
		return ipt, nil

	case "nftables":
		nft, err := NewNFT()
		if err != nil {
			return nil, fmt.Errorf("nftables initialization failed: %v", err)
		}
		if err = nft.Probe(); err != nil {
			return nil, fmt.Errorf("nftables Probe failed: %v", err)
		}
		base.Info("Firewall driver: using nftables")
		return nft, nil

	default: // "auto"
		nft, err := NewNFT()
		if err == nil {
			if err = nft.Probe(); err == nil {
				base.Info("Firewall driver: using nftables")
				return nft, nil
			}
			base.Error("nftables initialization failed, falling back to iptables:", err)
		} else {
			base.Error("nftables not available, falling back to iptables:", err)
		}

		ipt, err := NewIPT()
		if err != nil {
			return nil, fmt.Errorf("no supported firewall driver found (iptables error: %v)", err)
		}
		base.Info("Firewall driver: using iptables")
		return ipt, nil
	}
}

// iptables 实现
type IPT struct {
	ipt  *iptables.IPTables
	ipt6 *iptables.IPTables
}

func NewIPT() (*IPT, error) {
	ipt, err := iptables.New()
	if err != nil {
		return nil, err
	}
	// v6 FakeDNS 需要 ip6tables；不可用则降级（v4 FakeDNS 不受影响）
	ipt6, err := iptables.NewWithProtocol(iptables.ProtocolIPv6)
	if err != nil {
		base.Warn("ip6tables unavailable, v6 FakeDNS disabled:", err)
		ipt6 = nil
	}
	return &IPT{ipt: ipt, ipt6: ipt6}, nil
}
func (i *IPT) SetupGlobalNAT(vpnCIDR, masterDev string, inContainer bool) error {
	// MASQUERADE 规则
	natRule := []string{"-s", vpnCIDR, "-o", masterDev, "-j", "MASQUERADE"}
	if !inContainer {
		// 添加注释
		natRule = append(natRule, "-m", "comment", "--comment", "RemLink")
	}
	if err := i.ipt.InsertUnique("nat", "POSTROUTING", 1, natRule...); err != nil {
		return err
	}

	// FORWARD 规则：有状态放行（回包 established/related + 来自 VPN 网段的出站），
	// 对齐 NFT 已有的有状态设计，避免无条件 `-j ACCEPT` 把客户端暴露给外部入站
	forwardRules := [][]string{
		{"-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
		{"-s", vpnCIDR, "-j", "ACCEPT"},
	}
	for _, fr := range forwardRules {
		if !inContainer {
			fr = append(fr, "-m", "comment", "--comment", "RemLink")
		}
		if err := i.ipt.InsertUnique("filter", "FORWARD", 1, fr...); err != nil {
			return err
		}
	}
	base.Info("iptables: Setup Global NAT and Forward rules")
	return nil
}
func (i *IPT) AddGroupNAT(groupCIDR, masterDev string, inContainer bool) error {
	natRule := []string{"-s", groupCIDR, "-o", masterDev, "-j", "MASQUERADE"}
	if !inContainer {
		natRule = append(natRule, "-m", "comment", "--comment", "RemLink")
	}
	if err := i.ipt.InsertUnique("nat", "POSTROUTING", 1, natRule...); err != nil {
		return err
	}

	fwdRule := []string{"-s", groupCIDR, "-j", "ACCEPT"}
	if !inContainer {
		fwdRule = append(fwdRule, "-m", "comment", "--comment", "RemLink")
	}
	return i.ipt.InsertUnique("filter", "FORWARD", 1, fwdRule...)
}

// 设置全局 IPv6 NAT/转发规则。
func (i *IPT) SetupGlobalNAT6(vpnCIDR6, masterDev string, inContainer bool) error {
	ip6, err := iptables.NewWithProtocol(iptables.ProtocolIPv6)
	if err != nil {
		return err
	}

	natRule := []string{"-s", vpnCIDR6, "-o", masterDev, "-j", "MASQUERADE"}
	if !inContainer {
		natRule = append(natRule, "-m", "comment", "--comment", "RemLink")
	}
	if err := ip6.InsertUnique("nat", "POSTROUTING", 1, natRule...); err != nil {
		return err
	}

	forwardRules := [][]string{
		{"-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
		{"-s", vpnCIDR6, "-j", "ACCEPT"},
	}
	for _, fr := range forwardRules {
		if !inContainer {
			fr = append(fr, "-m", "comment", "--comment", "RemLink")
		}
		if err := ip6.InsertUnique("filter", "FORWARD", 1, fr...); err != nil {
			return err
		}
	}
	base.Info("iptables: Setup Global NAT6 and Forward rules")
	return nil
}

// 为组 v6 CIDR 添加 NAT66 和转发规则
func (i *IPT) AddGroupNAT6(groupCIDR6, masterDev string, inContainer bool) error {
	ip6, err := iptables.NewWithProtocol(iptables.ProtocolIPv6)
	if err != nil {
		return err
	}
	natRule := []string{"-s", groupCIDR6, "-o", masterDev, "-j", "MASQUERADE"}
	if !inContainer {
		natRule = append(natRule, "-m", "comment", "--comment", "RemLink")
	}
	if err := ip6.InsertUnique("nat", "POSTROUTING", 1, natRule...); err != nil {
		return err
	}
	fwdRule := []string{"-s", groupCIDR6, "-j", "ACCEPT"}
	if !inContainer {
		fwdRule = append(fwdRule, "-m", "comment", "--comment", "RemLink")
	}
	return ip6.InsertUnique("filter", "FORWARD", 1, fwdRule...)
}

// 删除组自定义 v4 CIDR 的 NAT/转发规则（MASQUERADE + 源段 FORWARD ACCEPT）。
func (i *IPT) DelGroupNAT(groupCIDR, masterDev string, inContainer bool) error {
	natRule := []string{"-s", groupCIDR, "-o", masterDev, "-j", "MASQUERADE"}
	fwdRule := []string{"-s", groupCIDR, "-j", "ACCEPT"}
	if !inContainer {
		natRule = append(natRule, "-m", "comment", "--comment", "RemLink")
		fwdRule = append(fwdRule, "-m", "comment", "--comment", "RemLink")
	}
	if err := i.ipt.Delete("nat", "POSTROUTING", natRule...); err != nil && !isIptRuleMissing(err) {
		return err
	}
	if err := i.ipt.Delete("filter", "FORWARD", fwdRule...); err != nil && !isIptRuleMissing(err) {
		return err
	}
	return nil
}

// 删除组 v6 CIDR 的 NAT66 和转发规则
func (i *IPT) DelGroupNAT6(groupCIDR6, masterDev string, inContainer bool) error {
	ip6, err := iptables.NewWithProtocol(iptables.ProtocolIPv6)
	if err != nil {
		return err
	}
	natRule := []string{"-s", groupCIDR6, "-o", masterDev, "-j", "MASQUERADE"}
	if !inContainer {
		natRule = append(natRule, "-m", "comment", "--comment", "RemLink")
	}
	if err := ip6.Delete("nat", "POSTROUTING", natRule...); err != nil && !isIptRuleMissing(err) {
		return err
	}
	fwdRule := []string{"-s", groupCIDR6, "-j", "ACCEPT"}
	if !inContainer {
		fwdRule = append(fwdRule, "-m", "comment", "--comment", "RemLink")
	}
	if err := ip6.Delete("filter", "FORWARD", fwdRule...); err != nil && !isIptRuleMissing(err) {
		return err
	}
	return nil
}

func (i *IPT) CreateChains(vpnCIDR, fakeIPRange string) error {
	// v6 假地址段：用 ip6tables 建独立的 v6 DNAT 链
	if strings.Contains(fakeIPRange, ":") {
		if i.ipt6 == nil {
			return fmt.Errorf("ip6tables not available, cannot create v6 FakeDNS chains")
		}
		err := i.ipt6.NewChain("nat", iptDnatChain)
		if err != nil && !strings.Contains(err.Error(), "Chain already exists") {
			return fmt.Errorf("failed to create v6 DNAT chain: %v", err)
		}
		dnatJumpRule := []string{"-s", vpnCIDR, "-d", fakeIPRange, "-j", iptDnatChain}
		if err := i.ipt6.AppendUnique("nat", "PREROUTING", dnatJumpRule...); err != nil {
			return fmt.Errorf("failed to add v6 DNAT jump rule: %v", err)
		}
		base.Info("Created v6 FakeIP iptables chains")
		return nil
	}

	// 创建 DNAT 自定义链
	err := i.ipt.NewChain("nat", iptDnatChain)
	if err != nil && !strings.Contains(err.Error(), "Chain already exists") {
		return fmt.Errorf("failed to create DNAT chain: %v", err)
	}

	// 在 PREROUTING 链中添加跳转规则
	dnatJumpRule := []string{"-s", vpnCIDR, "-d", fakeIPRange, "-j", iptDnatChain}
	err = i.ipt.AppendUnique("nat", "PREROUTING", dnatJumpRule...)
	if err != nil {
		return fmt.Errorf("failed to add DNAT jump rule: %v", err)
	}

	base.Info("Created FakeIP iptables chains")
	return nil
}

func (i *IPT) AddNatRule(fakeIP, realIP string) error {
	// v6 假地址：走 ip6tables
	if strings.Contains(fakeIP, ":") {
		if i.ipt6 == nil {
			return fmt.Errorf("ip6tables not available, cannot add v6 DNAT rule")
		}
		dnatrule := []string{"-d", fakeIP, "-j", "DNAT", "--to-destination", realIP}
		return i.ipt6.AppendUnique("nat", iptDnatChain, dnatrule...)
	}
	dnatrule := []string{"-d", fakeIP, "-j", "DNAT", "--to-destination", realIP}
	return i.ipt.AppendUnique("nat", iptDnatChain, dnatrule...)
}

func (i *IPT) DelNatRule(fakeIP, realIP string) error {
	// v6 假地址：走 ip6tables
	if strings.Contains(fakeIP, ":") {
		if i.ipt6 == nil {
			return fmt.Errorf("ip6tables not available, cannot delete v6 DNAT rule")
		}
		dnatRule := []string{"-d", fakeIP, "-j", "DNAT", "--to-destination", realIP}
		return i.ipt6.Delete("nat", iptDnatChain, dnatRule...)
	}
	dnatRule := []string{"-d", fakeIP, "-j", "DNAT", "--to-destination", realIP}
	return i.ipt.Delete("nat", iptDnatChain, dnatRule...)
}

func (i *IPT) CleanupFakeIP() error {
	// 清理 v4 DNAT 链
	rules, _ := i.ipt.List("nat", "PREROUTING")
	for _, rule := range rules {
		if strings.Contains(rule, iptDnatChain) {
			parts := strings.Fields(rule)
			if len(parts) > 2 {
				i.ipt.Delete("nat", "PREROUTING", parts[2:]...)
			}
		}
	}
	i.ipt.ClearChain("nat", iptDnatChain)
	i.ipt.DeleteChain("nat", iptDnatChain)

	// 清理 v6 DNAT 链
	if i.ipt6 != nil {
		if v6rules, err := i.ipt6.List("nat", "PREROUTING"); err == nil {
			for _, rule := range v6rules {
				if strings.Contains(rule, iptDnatChain) {
					parts := strings.Fields(rule)
					if len(parts) > 2 {
						i.ipt6.Delete("nat", "PREROUTING", parts[2:]...)
					}
				}
			}
		}
		i.ipt6.ClearChain("nat", iptDnatChain)
		i.ipt6.DeleteChain("nat", iptDnatChain)
	}

	return nil
}
func (i *IPT) CleanupGlobal() error {
	// 清理 全局 NAT 规则
	// 删除 nat 表 POSTROUTING 中的 MASQUERADE 规则
	natRules, _ := i.ipt.List("nat", "POSTROUTING")
	for _, rule := range natRules {
		if strings.Contains(rule, "RemLink") {
			parts := strings.Fields(rule)
			if len(parts) > 2 {
				i.ipt.Delete("nat", "POSTROUTING", parts[2:]...)
			}
		}
	}

	// 删除 filter 表 FORWARD 中的 RemLink ACCEPT 规则
	fwdRules, _ := i.ipt.List("filter", "FORWARD")
	for _, rule := range fwdRules {
		if strings.Contains(rule, "RemLink") {
			parts := strings.Fields(rule)
			if len(parts) > 2 {
				i.ipt.Delete("filter", "FORWARD", parts[2:]...)
			}
		}
	}

	// 清理 IPv6 全局 NAT 规则
	if ip6, err := iptables.NewWithProtocol(iptables.ProtocolIPv6); err == nil {
		if natRules6, err := ip6.List("nat", "POSTROUTING"); err == nil {
			for _, rule := range natRules6 {
				if strings.Contains(rule, "RemLink") {
					if parts := strings.Fields(rule); len(parts) > 2 {
						_ = ip6.Delete("nat", "POSTROUTING", parts[2:]...)
					}
				}
			}
		}
		if fwdRules6, err := ip6.List("filter", "FORWARD"); err == nil {
			for _, rule := range fwdRules6 {
				if strings.Contains(rule, "RemLink") {
					if parts := strings.Fields(rule); len(parts) > 2 {
						_ = ip6.Delete("filter", "FORWARD", parts[2:]...)
					}
				}
			}
		}
	}
	return nil
}

// nftables 实现
type NFT struct {
	mu     sync.Mutex
	conn   *nftables.Conn
	table  *nftables.Table
	table6 *nftables.Table // v6 FakeIP DNAT 表（未开 v6 FakeDNS 时为 nil）
}

func NewNFT() (*NFT, error) {
	c, err := nftables.New()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to nftables: %v", err)
	}
	return &NFT{conn: c}, nil
}
func ifname(name string) []byte {
	b := make([]byte, 16)
	copy(b, name)
	return b
}

// 设置全局 IPv4 NAT 和转发规则
func (n *NFT) SetupGlobalNAT(vpnCIDR, masterDev string, inContainer bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 创建全局NAT表
	globalNatTable := n.conn.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   nftGlobalNatTable,
	})

	_, ipNet, err := net.ParseCIDR(vpnCIDR)
	if err != nil {
		return fmt.Errorf("invalid vpnCIDR: %v", err)
	}
	ones, _ := ipNet.Mask.Size()
	prefixMask := net.CIDRMask(ones, 32)

	// POSTROUTING: MASQUERADE
	postrouting := n.conn.AddChain(&nftables.Chain{
		Name:     nftPostRoutingChain,
		Table:    globalNatTable,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookPostrouting,
		Priority: nftables.ChainPriorityNATSource,
	})
	n.conn.AddRule(&nftables.Rule{
		Table: globalNatTable,
		Chain: postrouting,
		Exprs: n.MasqueradeExprs(ipNet, prefixMask, masterDev),
	})

	// FORWARD: ACCEPT（匹配来自 VPN CIDR 的转发包）
	forwardChain := n.conn.AddChain(&nftables.Chain{
		Name:     nftForwardChain,
		Table:    globalNatTable,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookForward,
		Priority: nftables.ChainPriorityMangle,
	})
	// 回程流量
	n.conn.AddRule(&nftables.Rule{
		Table: globalNatTable,
		Chain: forwardChain,
		Exprs: []expr.Any{
			&expr.Ct{Key: expr.CtKeySTATE, Register: 1},
			// ct_state 以主机字节序存储，用 binaryutil 避免字节序错误
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            4,
				Mask:           binaryutil.NativeEndian.PutUint32(6), // ESTABLISHED|RELATED
				Xor:            binaryutil.NativeEndian.PutUint32(0),
			},
			// ct_state & 6 != 0
			&expr.Cmp{
				Op:       expr.CmpOpNeq,
				Register: 1,
				Data:     []byte{0, 0, 0, 0},
			},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})

	// 来自 VPN CIDR 的出站转发流量
	n.conn.AddRule(&nftables.Rule{
		Table: globalNatTable,
		Chain: forwardChain,
		Exprs: n.ForwardAcceptExprs(ipNet, prefixMask),
	})

	if err := n.conn.Flush(); err != nil {
		return fmt.Errorf("flush nftables failed: %v", err)
	}
	base.Info("nftables: Setup Global NAT and Forward rules")
	return nil
}
func (n *NFT) AddGroupNAT(groupCIDR, masterDev string, inContainer bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	_, ipNet, err := net.ParseCIDR(groupCIDR)
	if err != nil {
		return fmt.Errorf("invalid groupCIDR: %v", err)
	}
	ones, _ := ipNet.Mask.Size()
	prefixMask := net.CIDRMask(ones, 32)
	// 标记组规则，便于按需精准删除（不影响全局规则）
	ud := []byte(groupNATV4MagicPrefix + ipNet.String())

	globalNatTable := &nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   nftGlobalNatTable,
	}
	postroutingChain := &nftables.Chain{
		Name:  nftPostRoutingChain,
		Table: globalNatTable,
	}
	forwardChain := &nftables.Chain{
		Name:  nftForwardChain,
		Table: globalNatTable,
	}

	// POSTROUTING: MASQUERADE（组 CIDR 出站伪装）。
	n.conn.AddRule(&nftables.Rule{
		Table:    globalNatTable,
		Chain:    postroutingChain,
		Exprs:    n.MasqueradeExprs(ipNet, prefixMask, masterDev),
		UserData: ud,
	})

	// FORWARD: ACCEPT（允许组 CIDR 的出站转发流量）
	n.conn.AddRule(&nftables.Rule{
		Table:    globalNatTable,
		Chain:    forwardChain,
		Exprs:    n.ForwardAcceptExprs(ipNet, prefixMask),
		UserData: ud,
	})

	return n.conn.Flush()
}

// 按标记删除组 v4 CIDR 的 NAT 和转发规则
func (n *NFT) DelGroupNAT(groupCIDR, masterDev string, inContainer bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	_, ipNet, err := net.ParseCIDR(groupCIDR)
	if err != nil {
		return fmt.Errorf("invalid groupCIDR: %v", err)
	}
	ud := []byte(groupNATV4MagicPrefix + ipNet.String())
	globalNatTable := &nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   nftGlobalNatTable,
	}
	if err := n.delRulesByMagic(globalNatTable, nftPostRoutingChain, ud); err != nil {
		return err
	}
	if err := n.delRulesByMagic(globalNatTable, nftForwardChain, ud); err != nil {
		return err
	}
	return nil
}

// 为组 v6 CIDR 添加 NAT66 和转发规则
func (n *NFT) AddGroupNAT6(groupCIDR6, masterDev string, inContainer bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	_, ipNet, err := net.ParseCIDR(groupCIDR6)
	if err != nil {
		return fmt.Errorf("invalid groupCIDR6: %v", err)
	}
	ones, _ := ipNet.Mask.Size()
	prefixMask := net.CIDRMask(ones, 128)
	ud := []byte(groupNATV6MagicPrefix + ipNet.String())

	globalNatTable6 := &nftables.Table{
		Family: nftables.TableFamilyIPv6,
		Name:   nftGlobalNatTable6,
	}

	n.conn.AddRule(&nftables.Rule{
		Table:    globalNatTable6,
		Chain:    &nftables.Chain{Name: nftPostRoutingChain6, Table: globalNatTable6},
		Exprs:    n.MasqueradeExprs6(ipNet, prefixMask, masterDev),
		UserData: ud,
	})

	n.conn.AddRule(&nftables.Rule{
		Table:    globalNatTable6,
		Chain:    &nftables.Chain{Name: nftForwardChain6, Table: globalNatTable6},
		Exprs:    n.ForwardAcceptExprs6(ipNet, prefixMask),
		UserData: ud,
	})

	return n.conn.Flush()
}

// 按标记删除组 v6 CIDR 的 NAT66 和转发规则
func (n *NFT) DelGroupNAT6(groupCIDR6, masterDev string, inContainer bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	_, ipNet, err := net.ParseCIDR(groupCIDR6)
	if err != nil {
		return fmt.Errorf("invalid groupCIDR6: %v", err)
	}
	ud := []byte(groupNATV6MagicPrefix + ipNet.String())
	globalNatTable6 := &nftables.Table{
		Family: nftables.TableFamilyIPv6,
		Name:   nftGlobalNatTable6,
	}
	if err := n.delRulesByMagic(globalNatTable6, nftPostRoutingChain6, ud); err != nil {
		return err
	}
	if err := n.delRulesByMagic(globalNatTable6, nftForwardChain6, ud); err != nil {
		return err
	}
	return nil
}

// 设置全局 IPv6 NAT/转发规则。
func (n *NFT) SetupGlobalNAT6(vpnCIDR6, masterDev string, inContainer bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	globalNatTable6 := n.conn.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv6,
		Name:   nftGlobalNatTable6,
	})

	_, ipNet, err := net.ParseCIDR(vpnCIDR6)
	if err != nil {
		return fmt.Errorf("invalid vpnCIDR6: %v", err)
	}
	ones, _ := ipNet.Mask.Size()
	prefixMask := net.CIDRMask(ones, 128)

	postrouting := n.conn.AddChain(&nftables.Chain{
		Name:     nftPostRoutingChain6,
		Table:    globalNatTable6,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookPostrouting,
		Priority: nftables.ChainPriorityNATSource,
	})
	n.conn.AddRule(&nftables.Rule{
		Table: globalNatTable6,
		Chain: postrouting,
		Exprs: n.MasqueradeExprs6(ipNet, prefixMask, masterDev),
	})

	forwardChain := n.conn.AddChain(&nftables.Chain{
		Name:     nftForwardChain6,
		Table:    globalNatTable6,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookForward,
		Priority: nftables.ChainPriorityMangle,
	})
	// 回程流量（established/related）
	n.conn.AddRule(&nftables.Rule{
		Table: globalNatTable6,
		Chain: forwardChain,
		Exprs: []expr.Any{
			&expr.Ct{Key: expr.CtKeySTATE, Register: 1},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            4,
				Mask:           binaryutil.NativeEndian.PutUint32(6),
				Xor:            binaryutil.NativeEndian.PutUint32(0),
			},
			&expr.Cmp{
				Op:       expr.CmpOpNeq,
				Register: 1,
				Data:     []byte{0, 0, 0, 0},
			},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
	// 来自 VPN v6 CIDR 的出站转发流量
	n.conn.AddRule(&nftables.Rule{
		Table: globalNatTable6,
		Chain: forwardChain,
		Exprs: n.ForwardAcceptExprs6(ipNet, prefixMask),
	})

	if err := n.conn.Flush(); err != nil {
		return fmt.Errorf("flush nftables v6 failed: %v", err)
	}
	base.Info("nftables: Setup Global NAT6 and Forward rules")
	return nil
}

// MASQUERADE 规则表达式（IPv6，匹配源 CIDR + 出站接口）
func (n *NFT) MasqueradeExprs6(ipNet *net.IPNet, prefixMask net.IPMask, masterDev string) []expr.Any {
	return []expr.Any{
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       8,
			Len:          16,
		},
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            16,
			Mask:           prefixMask,
			Xor:            net.IPv6zero.To16(),
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     ipNet.IP.To16(),
		},
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 2},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 2,
			Data:     ifname(masterDev),
		},
		&expr.Masq{},
	}
}

// FORWARD ACCEPT 规则的表达式（IPv6，匹配源 CIDR）
func (n *NFT) ForwardAcceptExprs6(ipNet *net.IPNet, prefixMask net.IPMask) []expr.Any {
	return []expr.Any{
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       8,
			Len:          16,
		},
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            16,
			Mask:           prefixMask,
			Xor:            net.IPv6zero.To16(),
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     ipNet.IP.To16(),
		},
		&expr.Verdict{Kind: expr.VerdictAccept},
	}
}

// MASQUERADE 规则表达式（匹配源 CIDR + 出站接口）
func (n *NFT) MasqueradeExprs(ipNet *net.IPNet, prefixMask net.IPMask, masterDev string) []expr.Any {
	return []expr.Any{
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       12,
			Len:          4,
		},
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            4,
			Mask:           prefixMask,
			Xor:            net.IPv4zero.To4(),
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     ipNet.IP.To4(),
		},
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 2},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 2,
			Data:     ifname(masterDev),
		},
		&expr.Masq{},
	}
}

// FORWARD ACCEPT 规则的表达式（匹配源 CIDR）
func (n *NFT) ForwardAcceptExprs(ipNet *net.IPNet, prefixMask net.IPMask) []expr.Any {
	return []expr.Any{
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       12,
			Len:          4,
		},
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            4,
			Mask:           prefixMask,
			Xor:            net.IPv4zero.To4(),
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     ipNet.IP.To4(),
		},
		&expr.Verdict{Kind: expr.VerdictAccept},
	}
}

// 初始化 Table, Map 和跳转规则
func (n *NFT) CreateChains(vpnCIDR, fakeIPRange string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// v6 假地址段：建独立的 IPv6 family 表 + map + prerouting DNAT 链
	if strings.Contains(fakeIPRange, ":") {
		n.table6 = n.conn.AddTable(&nftables.Table{
			Family: nftables.TableFamilyIPv6,
			Name:   nftTableName6,
		})

		dnatMap6 := &nftables.Set{
			Table:    n.table6,
			Name:     nftDnatMapName6,
			IsMap:    true,
			KeyType:  nftables.TypeIP6Addr,
			DataType: nftables.TypeIP6Addr,
		}
		if err := n.conn.AddSet(dnatMap6, nil); err != nil {
			return err
		}

		prerouting6 := n.conn.AddChain(&nftables.Chain{
			Name:     nftFakeIPPreRoutingChain6,
			Table:    n.table6,
			Type:     nftables.ChainTypeNAT,
			Hooknum:  nftables.ChainHookPrerouting,
			Priority: nftables.ChainPriorityNATDest,
		})
		n.conn.AddRule(&nftables.Rule{
			Table: n.table6,
			Chain: prerouting6,
			Exprs: []expr.Any{
				// v6 头目的地址在偏移 24，16 字节
				&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 24, Len: 16},
				&expr.Lookup{SourceRegister: 1, DestRegister: 1, IsDestRegSet: true, SetName: nftDnatMapName6},
				&expr.NAT{Type: expr.NATTypeDestNAT, Family: uint32(nftables.TableFamilyIPv6), RegAddrMin: 1},
			},
		})

		base.Info("nftables v6 FakeIP initialized with map-based rules")
		return n.conn.Flush()
	}

	// 创建 IPv4 表
	n.table = n.conn.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   nftTableName,
	})

	// 创建 DNAT Map (Key: FakeIP -> Val: RealIP)
	dnatMap := &nftables.Set{
		Table:    n.table,
		Name:     nftDnatMapName,
		IsMap:    true,
		KeyType:  nftables.TypeIPAddr,
		DataType: nftables.TypeIPAddr,
	}
	if err := n.conn.AddSet(dnatMap, nil); err != nil {
		return err
	}

	// 创建 PREROUTING 规则: 如果目的地址在 dnat_map 中，执行 DNAT
	prerouting := n.conn.AddChain(&nftables.Chain{
		Name:     nftFakeIPPreRoutingChain,
		Table:    n.table,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookPrerouting,
		Priority: nftables.ChainPriorityNATDest,
	})

	n.conn.AddRule(&nftables.Rule{
		Table: n.table,
		Chain: prerouting,
		Exprs: []expr.Any{
			// 从 IP 头提取目的地址（DAddr）到寄存器 1
			&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 16, Len: 4},
			// 用寄存器 1 的值去 dnat_map 查找，结果覆盖回寄存器 1
			&expr.Lookup{SourceRegister: 1, DestRegister: 1, IsDestRegSet: true, SetName: nftDnatMapName},
			// 用寄存器 1 的值执行 DNAT
			&expr.NAT{Type: expr.NATTypeDestNAT, Family: uint32(nftables.TableFamilyIPv4), RegAddrMin: 1},
		},
	})

	base.Info("nftables initialized with map-based rules")
	return n.conn.Flush()
}
func (n *NFT) AddNatRule(fakeIP, realIP string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// v6 假地址：写入 v6 表的 DNAT map
	if strings.Contains(fakeIP, ":") {
		if n.table6 == nil {
			return fmt.Errorf("nftables v6 FakeIP table not initialized, cannot add v6 DNAT rule")
		}
		fake6 := net.ParseIP(fakeIP).To16()
		real6 := net.ParseIP(realIP).To16()
		if fake6 == nil || real6 == nil || real6.To4() != nil {
			return fmt.Errorf("invalid v6 DNAT pair: %s -> %s", fakeIP, realIP)
		}
		dnatSet6 := &nftables.Set{Table: n.table6, Name: nftDnatMapName6}
		n.conn.SetAddElements(dnatSet6, []nftables.SetElement{
			{Key: fake6, Val: real6},
		})
		if err := n.conn.Flush(); err != nil && !isNftDuplicateError(err) {
			return err
		}
		return nil
	}

	fakeIPParsed := net.ParseIP(fakeIP).To4()
	realIPParsed := net.ParseIP(realIP).To4()

	if fakeIPParsed == nil {
		return fmt.Errorf("invalid fake IP address: %s", fakeIP)
	}
	if realIPParsed == nil {
		return fmt.Errorf("invalid real IP address: %s", realIP)
	}
	dnatSet := &nftables.Set{Table: n.table, Name: nftDnatMapName}
	n.conn.SetAddElements(dnatSet, []nftables.SetElement{
		{Key: fakeIPParsed, Val: realIPParsed},
	})
	if err := n.conn.Flush(); err != nil && !isNftDuplicateError(err) {
		return err
	}
	return nil
}

func (n *NFT) DelNatRule(fakeIP, realIP string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// v6 假地址：从 v6 表的 DNAT map 删除
	if strings.Contains(fakeIP, ":") {
		if n.table6 == nil {
			return nil // v6 表未建，无规则可删
		}
		fake6 := net.ParseIP(fakeIP).To16()
		if fake6 == nil {
			return fmt.Errorf("invalid fake IP address: %s", fakeIP)
		}
		dnatSet6 := &nftables.Set{Table: n.table6, Name: nftDnatMapName6}
		n.conn.SetDeleteElements(dnatSet6, []nftables.SetElement{
			{Key: fake6},
		})
		if err := n.conn.Flush(); err != nil && !strings.Contains(err.Error(), "no such file") {
			return err
		}
		return nil
	}

	fakeIPParsed := net.ParseIP(fakeIP).To4()
	if fakeIPParsed == nil {
		return fmt.Errorf("invalid fake IP address: %s", fakeIP)
	}
	dnatSet := &nftables.Set{Table: n.table, Name: nftDnatMapName}
	n.conn.SetDeleteElements(dnatSet, []nftables.SetElement{
		{Key: fakeIPParsed},
	})
	if err := n.conn.Flush(); err != nil && !strings.Contains(err.Error(), "no such file") {
		return err
	}
	return nil
}

// 探测 nftables 是否可用
func (n *NFT) Probe() error {
	testTable := n.conn.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   "REMLINK_PROBE_TEST",
	})
	n.conn.AddChain(&nftables.Chain{
		Name:     "PROBE_CHAIN",
		Table:    testTable,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookPostrouting,
		Priority: nftables.ChainPriorityNATSource,
	})
	err := n.conn.Flush()
	n.conn.DelTable(testTable)
	n.conn.Flush()
	return err
}
func (n *NFT) CleanupFakeIP() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.table != nil {
		n.conn.DelTable(n.table)
	} else {
		n.conn.DelTable(&nftables.Table{Name: nftTableName, Family: nftables.TableFamilyIPv4})
	}
	err := n.conn.Flush()

	// v6 FakeIP 表单独 Flush：表不存在时的失败不应连带 v4 清理
	if n.table6 != nil {
		n.conn.DelTable(n.table6)
	} else {
		n.conn.DelTable(&nftables.Table{Name: nftTableName6, Family: nftables.TableFamilyIPv6})
	}
	if err6 := n.conn.Flush(); err6 != nil && n.table6 != nil {
		base.Warn("cleanup nftables v6 FakeIP table failed:", err6)
	}
	return err
}
func (n *NFT) CleanupGlobal() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 清理全局 NAT 表（IPv4 + IPv6）
	n.conn.DelTable(&nftables.Table{Name: nftGlobalNatTable, Family: nftables.TableFamilyIPv4})
	n.conn.DelTable(&nftables.Table{Name: nftGlobalNatTable6, Family: nftables.TableFamilyIPv6})
	return n.conn.Flush()
}

// 确保组自定义 v4/v6 网段的 NAT/转发规则已按指定出网网卡(egress)下发。
// 若此前已按不同 egress 下发过，则先清理旧规则再下发新规则（无需重启服务即可切换出网网卡）。
// v4CIDR / v6CIDR 可单独为空；入参 egress 应为已回退后的真实出网网卡（空=全局 master_dev）。
func SyncGroupNAT(v4CIDR, v6CIDR, egress string) {
	if v4CIDR == "" && v6CIDR == "" {
		return
	}
	fw := GetFirewall()
	if fw == nil {
		return
	}
	groupNatMu.Lock()
	defer groupNatMu.Unlock()
	if v4CIDR != "" {
		syncGroupNAT(fw, v4CIDR, egress, false)
	}
	if v6CIDR != "" {
		syncGroupNAT(fw, v6CIDR, egress, true)
	}
}

func syncGroupNAT(fw Firewall, cidr, egress string, isV6 bool) {
	if old, loaded := groupNatCIDRs.Load(cidr); !loaded || old != egress {
		if loaded {
			// egress 变化：先清理旧规则，避免旧出网网卡规则残留
			delGroupNATRule(fw, cidr, old.(string), isV6)
		}
		var err error
		if isV6 {
			err = fw.AddGroupNAT6(cidr, egress, base.InContainer)
		} else {
			err = fw.AddGroupNAT(cidr, egress, base.InContainer)
		}
		if err != nil {
			base.Warn("为组自定义网段", cidr, "下发 NAT 规则失败(egress=", egress, "):", err)
			return
		}
		base.Info("为组自定义网段", cidr, "下发 NAT 规则, egress=", egress)
		groupNatCIDRs.Store(cidr, egress)
	}
}

// 删除组自定义网段的 NAT/转发规则（组配置变更网段或删除组时调用），并清除去重跟踪。
// 仅删除与 oldOutDev（空则回退 master_dev）匹配的规则，避免误删其他出网网卡的规则。
func RemoveGroupNAT(oldV4, oldV6, oldOutDev string) {
	if oldV4 == "" && oldV6 == "" {
		return
	}
	fw := GetFirewall()
	if fw == nil {
		return
	}
	groupNatMu.Lock()
	defer groupNatMu.Unlock()
	remove := func(cidr string, isV6 bool) {
		if cidr == "" {
			return
		}
		egress := oldOutDev
		if tracked, ok := groupNatCIDRs.Load(cidr); ok {
			egress = tracked.(string)
		}
		if egress == "" {
			egress = base.GetCfg().MasterDev
		}
		delGroupNATRule(fw, cidr, egress, isV6)
		groupNatCIDRs.Delete(cidr)
	}
	remove(oldV4, false)
	remove(oldV6, true)
}

func delGroupNATRule(fw Firewall, cidr, egress string, isV6 bool) {
	var err error
	if isV6 {
		err = fw.DelGroupNAT6(cidr, egress, base.InContainer)
	} else {
		err = fw.DelGroupNAT(cidr, egress, base.InContainer)
	}
	if err != nil {
		// 规则可能本就不存在（如之前下发失败），忽略
		base.Debug("清理组自定义网段 NAT 规则失败(可忽略):", cidr, err)
	}
}

// 删除指定 nftables 链中 UserData 标记等于 magic 的所有规则（按 handle 精准删除）。
// 仅删除组规则，不影响全局规则；表/链不存在时忽略。
func (n *NFT) delRulesByMagic(table *nftables.Table, chainName string, magic []byte) error {
	chain := &nftables.Chain{Name: chainName, Table: table}
	rules, err := n.conn.GetRules(table, chain)
	if err != nil {
		// 表/链不存在（如 GlobalNat 关闭、未建全局 NAT 表）则无规则可删，忽略
		base.Debug("获取 nftables 规则失败(忽略):", err)
		return nil
	}
	deleted := false
	for _, r := range rules {
		if string(r.UserData) == string(magic) {
			if err := n.conn.DelRule(&nftables.Rule{Table: table, Chain: chain, Handle: r.Handle}); err != nil {
				return err
			}
			deleted = true
		}
	}
	if deleted {
		if err := n.conn.Flush(); err != nil {
			return err
		}
	}
	return nil
}

// 清理所有防火墙后端规则
func CleanupAllNatRules() {
	groupNatMu.Lock()
	defer groupNatMu.Unlock()
	if ipt, err := NewIPT(); err == nil {
		ipt.CleanupGlobal()
		ipt.CleanupFakeIP()
	}
	if nft, err := NewNFT(); err == nil {
		nft.CleanupGlobal()
		nft.CleanupFakeIP()
	}
	groupNatCIDRs.Range(func(key, _ any) bool {
		groupNatCIDRs.Delete(key)
		return true
	})
}

// 判断 nftables 规则已存在的错误
func isNftDuplicateError(err error) bool {
	return strings.Contains(err.Error(), "file exists")
}

// 判断 iptables 删除时“规则本就不存在”的错误
func isIptRuleMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "No such rule") ||
		strings.Contains(msg, "does a matching rule exist") ||
		strings.Contains(msg, "Bad rule") ||
		strings.Contains(msg, "No chain/target/match")
}
