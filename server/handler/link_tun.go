package handler

import (
	"fmt"
	"net"
	"strings"

	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

func checkTun() {
	// 测试tun
	cfg := water.Config{
		DeviceType: water.TUN,
	}

	ifce, err := water.New(cfg)
	if err != nil {
		base.Fatal("open tun err: ", err)
	}
	defer ifce.Close()

	link, err := netlink.LinkByName(ifce.Name())
	if err != nil {
		base.Fatal("testTun err: ", err)
	}
	if err = netlink.LinkSetMTU(link, 1399); err != nil {
		base.Fatal("testTun err: ", err)
	}
	if err = netlink.LinkSetAllmulticastOff(link); err != nil {
		base.Fatal("testTun err: ", err)
	}
	if err = netlink.LinkSetUp(link); err != nil {
		base.Fatal("testTun err: ", err)
	}
	// 未启用 NAT 时不初始化防火墙后端
	if !base.GetCfg().GlobalNat && !base.GetCfg().GlobalNat6 {
		return
	}

	fw := sessdata.GetFirewall()
	if fw == nil {
		base.Error("初始化防火墙失败: firewall is nil, 请检查防火墙后端配置")
		return
	}

	// IPv4 全局 NAT：仅由 GlobalNat 控制，行为不变
	if base.GetCfg().GlobalNat {
		// 校验主网卡是否存在
		masterDev := base.GetCfg().MasterDev
		if _, err := netlink.LinkByName(masterDev); err != nil {
			ifaces := utils.GetPhysicalInterfaces()
			base.Warn("========================================")
			base.Warn("NAT 配置错误：主网卡未正确配置!")
			base.Warn("当前可用物理网卡: " + strings.Join(ifaces, ", "))
			base.Warn("NAT 转发规则将无法生效, 请在web后台更新配置")
			base.Warn("========================================")
		}
		if err := fw.SetupGlobalNAT(base.GetCfg().Ipv4CIDR, base.GetCfg().MasterDev, base.InContainer); err != nil {
			if _, ok := fw.(*sessdata.IPT); ok {
				base.Error("设置NAT转发失败:", err)
				base.Error("请确认内核已加载 iptable_nat/iptable_filter 模块：lsmod | grep iptable")
				base.Error("或执行 modprobe iptable_nat iptable_filter 后，在 web 后台更新配置并重启服务")
			} else {
				base.Error("设置NAT转发失败:", err)
				base.Error("请在 web 后台更新配置后重启服务")
			}
		}
	}

	// IPv6 NAT/转发由 GlobalNat6 独立控制。
	if base.GetCfg().Ipv6CIDR != "" && base.GetCfg().GlobalNat6 {
		if err := fw.SetupGlobalNAT6(base.GetCfg().Ipv6CIDR, base.GetCfg().MasterDev, base.InContainer); err != nil {
			if _, ok := fw.(*sessdata.IPT); ok {
				base.Error("设置 IPv6 NAT/转发失败:", err)
				base.Error("请确认内核已加载 ip6table_nat/ip6table_filter 模块：lsmod | grep ip6table")
				base.Error("或执行 modprobe ip6table_nat ip6table_filter 后，在 web 后台更新配置并重启服务")
			} else {
				base.Error("设置 IPv6 NAT/转发失败:", err)
				base.Error("请在 web 后台更新配置后重启服务")
			}
		}
	}
}

// 创建tun网卡
func LinkTun(cSess *sessdata.ConnSession) error {
	cfg := water.Config{
		DeviceType: water.TUN,
	}

	ifce, err := water.New(cfg)
	if err != nil {
		base.Error(err)
		return err
	}
	cSess.SetIfName(ifce.Name())

	alias := utils.ParseName(cSess.Group.Name + "." + cSess.Username)
	link, err := netlink.LinkByName(ifce.Name())
	if err != nil {
		base.Error(err)
		_ = ifce.Close()
		return err
	}

	//	设置mtu
	if err = netlink.LinkSetMTU(link, cSess.Mtu); err != nil {
		base.Error(err)
		_ = ifce.Close()
		return err
	}
	// 禁用广播
	if err = netlink.LinkSetAllmulticastOff(link); err != nil {
		base.Error(err)
		_ = ifce.Close()
		return err
	}
	if err = netlink.LinkSetUp(link); err != nil {
		base.Error(err)
		_ = ifce.Close()
		return err
	}
	// 设置 alias
	if err = netlink.LinkSetAlias(link, alias); err != nil {
		base.Warn("set alias err: ", err)
	}

	localIP := cSess.IpPool.Ipv4Gateway
	peerIP := cSess.IpAddr
	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   localIP,
			Mask: net.CIDRMask(32, 32),
		},
		Peer: &net.IPNet{
			IP:   peerIP,
			Mask: net.CIDRMask(32, 32),
		},
	}
	if err = netlink.AddrAdd(link, addr); err != nil {
		base.Error(err)
		_ = ifce.Close()
		return err
	}

	// IPv6 双栈：客户端 /128，网关 /128（点对点）；v6 地址池为全局单池，网关取全局池
	if cSess.IpAddr6 != nil {
		// 部分环境（容器/默认 sysctl）新接口继承 disable_ipv6=1，必须先启用，
		// 否则给 TUN 加 v6 地址会返回 EACCES（permission denied）
		if err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", ifce.Name()), "0"); err != nil {
			base.Warn("enable ipv6 on tun failed: ", err)
		}
		// 显式开启该 TUN 接口的 IPv6 转发（新建接口从 default.forwarding 继承，可能仍为 0，导致 v6 转发不生效）
		if err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.forwarding", ifce.Name()), "1"); err != nil {
			base.Warn("enable ipv6 forwarding on tun failed: ", err)
		}
		v6Addr := &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   cSess.IpPool.V6Gateway(),
				Mask: net.CIDRMask(128, 128),
			},
			Peer: &net.IPNet{
				IP:   cSess.IpAddr6,
				Mask: net.CIDRMask(128, 128),
			},
		}
		if err = netlink.AddrAdd(link, v6Addr); err != nil {
			base.Error(err)
			_ = ifce.Close()
			return err
		}
	}

	// 设置组NAT
	setGroupNAT(cSess)
	// 仅纯 v4 时禁用 TUN 接口的 IPv6，避免无地址的 v6 流量；启用 v6 双栈时不禁用
	if cSess.IpAddr6 == nil {
		if err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", ifce.Name()), "1"); err != nil {
			base.Warn(err)
		}
	}

	go tunRead(ifce, cSess)
	go tunWrite(ifce, cSess)
	return nil
}

func tunWrite(ifce *water.Interface, cSess *sessdata.ConnSession) {
	defer func() {
		base.Debug("LinkTun return", cSess.IpAddr)
		cSess.Close()
		_ = ifce.Close()
	}()

	var (
		err error
		pl  *sessdata.Payload
	)

	for {
		select {
		case pl = <-cSess.PayloadIn:
		case <-cSess.CloseChan:
			return
		}

		_, err = ifce.Write(pl.Data)
		if err != nil {
			base.Error("tun Write err", err)
			return
		}

		putPayloadInBefore(cSess, pl)
	}
}

func tunRead(ifce *water.Interface, cSess *sessdata.ConnSession) {
	defer func() {
		base.Debug("tunRead return", cSess.IpAddr)
		_ = ifce.Close()
	}()
	var (
		err error
		n   int
	)

	for {
		// data := make([]byte, BufferSize)
		pl := getPayload()
		n, err = ifce.Read(pl.Data)
		if err != nil {
			base.Error("tun Read err", n, err)
			return
		}

		// 更新数据长度
		pl.Data = (pl.Data)[:n]

		if payloadOut(cSess, pl) {
			return
		}
	}
}
func setGroupNAT(cSess *sessdata.ConnSession) {
	if cSess.IpPool == nil || cSess.IpPool.Ipv4IPNet == nil {
		return
	}
	// 回退到全局池则
	if cSess.IpPool == sessdata.IpPool {
		return
	}

	cidr := cSess.IpPool.Ipv4IPNet.String()
	v6cidr := ""
	if cSess.IpPool.Ipv6IPNet != nil {
		v6cidr = cSess.IpPool.Ipv6IPNet.String()
	}

	// 按协议族开关过滤组 NAT 规则，关闭后由用户自行负责路由和转发。
	if !base.GetCfg().GlobalNat {
		cidr = ""
	}
	if !base.GetCfg().GlobalNat6 {
		v6cidr = ""
	}
	if cidr == "" && v6cidr == "" {
		return
	}

	// 出网网卡：组级 out_dev 优先，空则沿用全局 master_dev。
	egress := cSess.Group.OutDev
	if egress == "" {
		egress = base.GetCfg().MasterDev
	}
	// 出网网卡不存在时跳过下发，待下次连接重试
	if _, err := net.InterfaceByName(egress); err != nil {
		base.Warn("组", cSess.Group.Name, "出网网卡", egress, "不存在，跳过 NAT 下发:", err)
		return
	}

	sessdata.SyncGroupNAT(cidr, v6cidr, egress)

	// 组指定了独立的出网网卡且不同于全局 master_dev 时，确保该接口 IPv6 可用
	if v6cidr != "" && egress != base.GetCfg().MasterDev {
		if err := sysctlSet("net.ipv6.conf."+egress+".disable_ipv6", "0"); err != nil {
			base.Warn(err)
		}
		if err := sysctlSet("net.ipv6.conf."+egress+".accept_ra", "2"); err != nil {
			base.Warn(err)
		}
	}
}
