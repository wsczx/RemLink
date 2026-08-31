package handler

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/vishvananda/netlink"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

// link vtap
const vTapPrefix = "lvtap"

type Vtap struct {
	*os.File
	ifName    string
	closeOnce sync.Once
}

func (v *Vtap) Close() error {
	// 读/写两个协程都会 defer Close，用 Once 保证 fd 关闭与 LinkDel 只执行一次，
	// 避免第二次 LinkByName 因网卡已删而报无谓错误
	var err error
	v.closeOnce.Do(func() {
		v.File.Close()
		link, e := netlink.LinkByName(v.ifName)
		if e != nil {
			err = e
			return
		}
		err = netlink.LinkDel(link)
	})
	return err
}

func checkMacvtap() {
	_setGateway()
	_checkTapIp(base.GetCfg().MasterDev)

	ifName := "remlinkMacvtap"

	// 开启主网卡混杂模式
	masterLink, err := netlink.LinkByName(base.GetCfg().MasterDev)
	if err != nil {
		base.Fatal(err)
	}
	if err = netlink.SetPromiscOn(masterLink); err != nil {
		base.Fatal(err)
	}

	// 测试 macvtap 功能
	macvtap := &netlink.Macvtap{
		Macvlan: netlink.Macvlan{
			LinkAttrs: netlink.LinkAttrs{
				Name:        ifName,
				ParentIndex: masterLink.Attrs().Index,
			},
			Mode: netlink.MACVLAN_MODE_BRIDGE,
		},
	}
	if err = netlink.LinkAdd(macvtap); err != nil {
		base.Fatal(err)
	}

	testLink, err := netlink.LinkByName(ifName)
	if err != nil {
		base.Fatal(err)
	}
	if err = netlink.LinkDel(testLink); err != nil {
		base.Fatal(err)
	}
}

// 创建 Macvtap 网卡
func LinkMacvtap(cSess *sessdata.ConnSession) error {
	capL := sessdata.IpPool.IpLongMax - sessdata.IpPool.IpLongMin
	if capL <= 0 {
		// IP 池配置异常（Max<=Min）时避免取模除零 panic
		capL = 1
	}
	ipN := utils.Ip2long(cSess.IpAddr) % capL
	ifName := fmt.Sprintf("%s%d", vTapPrefix, ipN)

	cSess.SetIfName(ifName)

	alias := utils.ParseName(cSess.Group.Name + "." + cSess.Username)
	// 创建 macvtap 网卡
	masterLink, err := netlink.LinkByName(base.GetCfg().MasterDev)
	if err != nil {
		base.Error(err)
		return err
	}
	macvtap := &netlink.Macvtap{
		Macvlan: netlink.Macvlan{
			LinkAttrs: netlink.LinkAttrs{
				Name:        ifName,
				ParentIndex: masterLink.Attrs().Index,
				Alias:       alias,
			},
			Mode: netlink.MACVLAN_MODE_BRIDGE,
		},
	}
	if err = netlink.LinkAdd(macvtap); err != nil {
		base.Error(err)
		return err
	}
	//
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		base.Error(err)
		return err
	}
	if err = netlink.LinkSetMTU(link, cSess.Mtu); err != nil {
		base.Error(err)
		return err
	}
	// 设置 mac
	mac, err := net.ParseMAC(cSess.MacHw.String())
	if err != nil {
		base.Error(err)
		return err
	}
	if err = netlink.LinkSetHardwareAddr(link, mac); err != nil {
		base.Error(err)
		return err
	}
	// 启动网卡
	if err = netlink.LinkSetUp(link); err != nil {
		base.Error(err)
		return err
	}
	// 未分配 v6 地址时禁用 v6，避免内核链路本地地址干扰；双栈时显式启用
	if cSess.IpAddr6 == nil {
		err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", ifName), "1")
		if err != nil {
			base.Warn(err)
		}
	} else {
		// 双栈：显式启用接口的 IPv6，否则新接口继承 disable_ipv6=1 会导致 v6 数据面 EACCES
		if err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", ifName), "0"); err != nil {
			base.Warn(err)
		}
		// 显式开启该接口的 IPv6 转发（新建接口从 default.forwarding 继承，可能仍为 0，导致 v6 转发不生效）
		if err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.forwarding", ifName), "1"); err != nil {
			base.Warn(err)
		}
		// 将 v6 网关地址赋到 macvtap 接口，供客户端解析网关 MAC 与服务器侧路由。
		// macvtap 与主网卡同处二层广播域，内核在本接口应答 NDP；每个 lvtapN 独立接口可各自持有 /128 网关地址。
		v6GwAddr := &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   cSess.IpPool.V6Gateway(),
				Mask: net.CIDRMask(128, 128),
			},
		}
		if err = netlink.AddrReplace(link, v6GwAddr); err != nil {
			base.Warn("assign v6 gateway to vtap failed: ", err)
		}
	}

	// 设置组NAT
	setGroupNAT(cSess)

	return createVtap(cSess, ifName)
}

// 创建 Ipvtap 网卡。
// 基于 ipvlan 子系统，工作在三层路由模式：子设备共享母网卡 MAC，
// 内核按 IP 分发流量（而非二层桥接）。适合客户端规模大、母网卡所在网络对 MAC 数量敏感的场景。
func LinkIpvtap(cSess *sessdata.ConnSession) error {
	capL := sessdata.IpPool.IpLongMax - sessdata.IpPool.IpLongMin
	if capL <= 0 {
		// IP 池配置异常（Max<=Min）时兜底，避免取模除零 panic
		capL = 1
	}
	ipN := utils.Ip2long(cSess.IpAddr) % capL
	ifName := fmt.Sprintf("%s%d", vTapPrefix, ipN)

	cSess.SetIfName(ifName)

	alias := utils.ParseName(cSess.Group.Name + "." + cSess.Username)
	// 创建 ipvtap 网卡（Ipvlan 三层模式，与母网卡共享 MAC，不设置独立硬件地址）
	masterLink, err := netlink.LinkByName(base.GetCfg().MasterDev)
	if err != nil {
		base.Error(err)
		return err
	}
	ipvtap := &netlink.IPVtap{
		IPVlan: netlink.IPVlan{
			LinkAttrs: netlink.LinkAttrs{
				Name:        ifName,
				ParentIndex: masterLink.Attrs().Index,
				Alias:       alias,
			},
			Mode: netlink.IPVLAN_MODE_L3,
		},
	}
	if err = netlink.LinkAdd(ipvtap); err != nil {
		base.Error(err)
		return err
	}
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		base.Error(err)
		return err
	}
	if err = netlink.LinkSetMTU(link, cSess.Mtu); err != nil {
		base.Error(err)
		return err
	}
	// 启动网卡（ipvtap 共享母网卡 MAC，无需 LinkSetHardwareAddr）
	if err = netlink.LinkSetUp(link); err != nil {
		base.Error(err)
		return err
	}
	// 未分配 v6 地址时禁用 v6，避免内核链路本地地址干扰；双栈时显式启用
	if cSess.IpAddr6 == nil {
		err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", ifName), "1")
		if err != nil {
			base.Warn(err)
		}
	} else {
		// 双栈：显式启用接口的 IPv6，否则新接口继承 disable_ipv6=1 会导致 v6 数据面 EACCES
		if err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", ifName), "0"); err != nil {
			base.Warn(err)
		}
		// 显式开启该接口的 IPv6 转发（新建接口从 default.forwarding 继承，可能仍为 0，导致 v6 转发不生效）
		if err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.forwarding", ifName), "1"); err != nil {
			base.Warn(err)
		}
		// 将 v6 网关地址赋到 ipvtap 接口。ipvtap 为三层路由，v6 流量按 IP 分发，
		// 不受共享 MAC 影响；每个 lvtapN 独立接口可各自持有 /128 网关地址。
		v6GwAddr := &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   cSess.IpPool.V6Gateway(),
				Mask: net.CIDRMask(128, 128),
			},
		}
		if err = netlink.AddrReplace(link, v6GwAddr); err != nil {
			base.Warn("assign v6 gateway to vtap failed: ", err)
		}
	}

	// 设置组NAT
	setGroupNAT(cSess)

	return createVtap(cSess, ifName)
}

type ifReq struct {
	Name  [0x10]byte
	Flags uint16
	pad   [0x28 - 0x10 - 2]byte
}

func createVtap(cSess *sessdata.ConnSession, ifName string) error {
	// 初始化 ifName
	inf, err := net.InterfaceByName(ifName)
	if err != nil {
		base.Error(err)
		return err
	}

	tName := fmt.Sprintf("/dev/tap%d", inf.Index)

	var fdInt int

	fdInt, err = syscall.Open(tName, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}

	var flags uint16 = syscall.IFF_TAP | syscall.IFF_NO_PI
	var req ifReq
	req.Flags = flags

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fdInt),
		uintptr(syscall.TUNSETIFF),
		uintptr(unsafe.Pointer(&req)),
	)
	if errno != 0 {
		return os.NewSyscallError("ioctl", errno)
	}

	file := os.NewFile(uintptr(fdInt), tName)
	ifce := &Vtap{File: file, ifName: ifName}

	go allTapRead(ifce, cSess)
	go allTapWrite(ifce, cSess)
	return nil
}

// 销毁未关闭的vtap
func destroyVtap() {
	its, err := net.Interfaces()
	if err != nil {
		base.Error(err)
		return
	}
	for _, v := range its {
		if strings.HasPrefix(v.Name, vTapPrefix) {
			// 删除原来的网卡
			link, err := netlink.LinkByName(v.Name)
			if err != nil {
				base.Error(err)
				continue
			}
			netlink.LinkDel(link)
		}
	}
}
