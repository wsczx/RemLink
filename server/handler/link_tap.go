package handler

import (
	"fmt"
	"io"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/songgao/packets/ethernet"
	"github.com/songgao/water"
	"github.com/songgao/water/waterutil"
	"github.com/vishvananda/netlink"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/arpdis"
	"github.com/wsczx/remlink/pkg/ndpdis"
	"github.com/wsczx/remlink/sessdata"
)

const bridgeName = "remlink0"

var (
	// 网关mac地址
	gatewayHw net.HardwareAddr
)

type LinkDriver interface {
	io.ReadWriteCloser
	Name() string
}

func _setGateway() {
	dstAddr := arpdis.Lookup(sessdata.IpPool.Ipv4Gateway, false)
	gatewayHw = dstAddr.HardwareAddr
	// 设置为静态地址映射
	dstAddr.Type = arpdis.TypeStatic
	arpdis.Add(dstAddr)
}

func _checkTapIp(ifName string) {
	iFace, err := net.InterfaceByName(ifName)
	if err != nil {
		base.Fatal("testTap err: ", err)
	}

	var ifIp net.IP

	addrs, err := iFace.Addrs()
	if err != nil {
		base.Fatal("testTap err: ", err)
	}
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil || ip.To4() == nil {
			continue
		}
		ifIp = ip
	}

	if !sessdata.IpPool.Ipv4IPNet.Contains(ifIp) {
		base.Fatal("tapIp or Ip network err")
	}
}

func checkTap() {
	_setGateway()
	_checkTapIp(bridgeName)
}

// 创建tap网卡
func LinkTap(cSess *sessdata.ConnSession) error {
	cfg := water.Config{
		DeviceType: water.TAP,
	}

	ifce, err := water.New(cfg)
	if err != nil {
		base.Error(err)
		return err
	}

	cSess.SetIfName(ifce.Name())

	link, err := netlink.LinkByName(ifce.Name())
	if err != nil {
		base.Error(err)
		_ = ifce.Close()
		return err
	}
	// 设置mtu
	if err = netlink.LinkSetMTU(link, cSess.Mtu); err != nil {
		base.Error(err)
		_ = ifce.Close()
		return err
	}
	// 设置多播
	if err = netlink.LinkSetAllmulticastOn(link); err != nil {
		base.Error(err)
		_ = ifce.Close()
		return err
	}
	// 设置名字
	if err = netlink.LinkSetUp(link); err != nil {
		base.Error(err)
		_ = ifce.Close()
		return err
	}
	bridge, err := netlink.LinkByName(bridgeName)
	if err != nil {
		base.Error(err)
		_ = ifce.Close()
		return err
	}
	// 设置网卡
	if err = netlink.LinkSetMaster(link, bridge); err != nil {
		base.Error(err)
		_ = ifce.Close()
		return err
	}

	// 未分配 v6 地址时禁用 v6，避免内核链路本地地址干扰；双栈时保持开启
	if cSess.IpAddr6 == nil {
		err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", ifce.Name()), "1")
		if err != nil {
			base.Warn(err)
		}
	}

	// IPv6 双栈：将 v6 网关地址赋到桥，供客户端解析网关 MAC 与服务器侧路由。
	// 桥为持久设备，每次建链用 AddrReplace 幂等（避免重复建链时地址已存在报错）。
	if cSess.IpAddr6 != nil {
		// 部分环境新接口继承 disable_ipv6=1，必须先启用桥与 tap 接口的 IPv6，
		// 否则 AddrReplace v6 网关地址会返回 EACCES（permission denied）
		if err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", bridge.Attrs().Name), "0"); err != nil {
			base.Warn("enable ipv6 on bridge failed: ", err)
		}
		if err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", ifce.Name()), "0"); err != nil {
			base.Warn("enable ipv6 on tap failed: ", err)
		}
		// 显式开启桥与 tap 接口的 IPv6 转发（新建接口从 default.forwarding 继承，可能仍为 0，导致 v6 转发不生效）
		if err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.forwarding", bridge.Attrs().Name), "1"); err != nil {
			base.Warn("enable ipv6 forwarding on bridge failed: ", err)
		}
		if err = sysctlSet(fmt.Sprintf("net.ipv6.conf.%s.forwarding", ifce.Name()), "1"); err != nil {
			base.Warn("enable ipv6 forwarding on tap failed: ", err)
		}
		v6GwAddr := &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   cSess.IpPool.V6Gateway(),
				Mask: net.CIDRMask(128, 128),
			},
		}
		if err = netlink.AddrReplace(bridge, v6GwAddr); err != nil {
			base.Warn("assign v6 gateway to bridge failed: ", err)
		}
	}

	// 设置组NAT
	setGroupNAT(cSess)

	go allTapRead(ifce, cSess)
	go allTapWrite(ifce, cSess)
	return nil
}

func allTapWrite(ifce LinkDriver, cSess *sessdata.ConnSession) {
	// 注册 v6 地址→客户端MAC，供 NDP 代答和池内互访寻址
	if cSess.IpAddr6 != nil {
		ndpdis.Add(cSess.IpAddr6, cSess.MacHw)
	}
	defer func() {
		base.Debug("LinkTap return", cSess.IpAddr)
		if cSess.IpAddr6 != nil {
			ndpdis.Delete(cSess.IpAddr6)
		}
		cSess.Close()
		ifce.Close()
	}()

	var (
		err   error
		dstHw net.HardwareAddr
		pl    *sessdata.Payload
		frame = make(ethernet.Frame, BufferSize)
		ipDst = net.IPv4(1, 2, 3, 4)
	)

	for {
		frame.Resize(BufferSize)

		select {
		case pl = <-cSess.PayloadIn:
		case <-cSess.CloseChan:
			return
		}

		switch pl.LType {
		default:
		case sessdata.LTypeEthernet:
			copy(frame, pl.Data)
			frame = frame[:len(pl.Data)]

		case sessdata.LTypeIPData: // 需要转换成 Ethernet 数据
			if waterutil.IsIPv6(pl.Data) {
				if cSess.IpAddr6 == nil || len(pl.Data) < 40 {
					continue
				}
				// 校验源地址为本会话分配的 v6 地址，防伪造
				if !net.IP(pl.Data[8:24]).Equal(cSess.IpAddr6) {
					continue
				}
				ipDst6 := net.IP(pl.Data[24:40])
				dstHw = gatewayHw
				if ipDst6[0] == 0xff {
					// 组播地址按 RFC2464 映射以太网组播MAC 33:33:xxxx
					dstHw = net.HardwareAddr{0x33, 0x33, ipDst6[12], ipDst6[13], ipDst6[14], ipDst6[15]}
				} else if sessdata.IpPool.Ipv6IPNet.Contains(ipDst6) {
					if hw := ndpdis.Lookup(ipDst6); hw != nil {
						dstHw = hw
					}
				}
				frame.Prepare(dstHw, cSess.MacHw, ethernet.NotTagged, ethernet.IPv6, len(pl.Data))
				copy(frame[12+2:], pl.Data)
				break
			}

			ipSrc := waterutil.IPv4Source(pl.Data)
			if !ipSrc.Equal(cSess.IpAddr) {
				// 非分配给客户端ip，直接丢弃
				continue
			}

			// 手动设置ipv4地址
			ipDst[12] = pl.Data[16]
			ipDst[13] = pl.Data[17]
			ipDst[14] = pl.Data[18]
			ipDst[15] = pl.Data[19]

			dstHw = gatewayHw
			if sessdata.IpPool.Ipv4IPNet.Contains(ipDst) {
				dstAddr := arpdis.Lookup(ipDst, true)
				if dstAddr != nil {
					dstHw = dstAddr.HardwareAddr
				}
			}

			frame.Prepare(dstHw, cSess.MacHw, ethernet.NotTagged, ethernet.IPv4, len(pl.Data))
			copy(frame[12+2:], pl.Data)
		}

		_, err = ifce.Write(frame)
		if err != nil {
			base.Error("tap Write err", err)
			return
		}

		putPayloadInBefore(cSess, pl)
	}
}

func allTapRead(ifce LinkDriver, cSess *sessdata.ConnSession) {
	defer func() {
		// 解析畸形二层帧等异常不应带崩整个进程
		if err := recover(); err != nil {
			base.Error("tapRead panic", err)
		}
		base.Debug("tapRead return", cSess.IpAddr)
		ifce.Close()
	}()

	var (
		err   error
		n     int
		data  []byte
		frame = make(ethernet.Frame, BufferSize)
	)

	for {
		frame.Resize(BufferSize)

		n, err = ifce.Read(frame)
		if err != nil {
			base.Error("tap Read err", n, err)
			return
		}
		frame = frame[:n]

		switch frame.Ethertype() {
		default:
			continue
		case ethernet.IPv6:
			if cSess.IpAddr6 == nil {
				continue
			}
			data = frame.Payload()
			if len(data) < 40 {
				continue
			}

			// NS(邻居请求) 代答（类比 ARP 代理）：目标为网关、本会话、或池内其他客户端地址时分别代答
			// data[6]=NextHeader(58=ICMPv6) data[40]=ICMPv6 Type(135=NS)
			if data[6] == 58 && len(data) > 40 && data[40] == 135 {
				packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.NoCopy)
				nsLayer := packet.Layer(layers.LayerTypeICMPv6NeighborSolicitation)
				if nsLayer == nil {
					continue
				}
				// 畸形包各层可能缺失，逐一判防强制断言 panic
				ns, ok := nsLayer.(*layers.ICMPv6NeighborSolicitation)
				if !ok {
					continue
				}
				ethLayer, ok := packet.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
				if !ok {
					continue
				}
				ip6Layer, ok := packet.Layer(layers.LayerTypeIPv6).(*layers.IPv6)
				if !ok {
					continue
				}
				if ip6Layer.SrcIP.IsUnspecified() {
					// DAD 探测(源为::)不代答，/128 池分配不存在地址冲突
					continue
				}
				// 决定代答 MAC：本会话地址→本 MAC；网关→gatewayHw；池内其他客户端→查 ndpdis
				var replyMac net.HardwareAddr
				switch {
				case cSess.IpAddr6.Equal(ns.TargetAddress):
					replyMac = cSess.MacHw
				case cSess.IpPool.V6Gateway() != nil && cSess.IpPool.V6Gateway().Equal(ns.TargetAddress):
					replyMac = gatewayHw
				default:
					replyMac = ndpdis.Lookup(ns.TargetAddress)
				}
				if replyMac == nil {
					continue
				}
				data, err = ndpdis.NewNAReply(ns.TargetAddress, replyMac, ip6Layer.SrcIP, ethLayer.SrcMAC)
				if err != nil {
					base.Error(err)
					return
				}
				pl := getPayload()
				pl.LType = sessdata.LTypeEthernet
				copy(pl.Data, data)
				pl.Data = pl.Data[:len(data)]
				if payloadIn(cSess, pl) {
					return
				}
				continue
			}

			// 普通 v6 数据：仅投递目的为本会话地址的包
			if !net.IP(data[24:40]).Equal(cSess.IpAddr6) {
				continue
			}
			pl := getPayload()
			copy(pl.Data, data)
			pl.Data = pl.Data[:len(data)]
			if payloadOut(cSess, pl) {
				return
			}

		case ethernet.IPv4:
			// 发送IP数据
			data = frame.Payload()

			ip_dst := waterutil.IPv4Destination(data)
			if !ip_dst.Equal(cSess.IpAddr) {
				// 过滤非本机地址
				continue
			}

			pl := getPayload()
			// 拷贝数据到pl
			copy(pl.Data, data)
			// 更新切片长度
			pl.Data = pl.Data[:len(data)]
			if payloadOut(cSess, pl) {
				return
			}

		case ethernet.ARP:
			// 暂时仅实现了ARP协议
			packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.NoCopy)
			layer := packet.Layer(layers.LayerTypeARP)
			// 畸形 ARP 帧解析失败时 layer 为 nil，必须判 ok 否则强制断言 panic 崩进程
			arpReq, ok := layer.(*layers.ARP)
			if !ok {
				continue
			}

			if !cSess.IpAddr.Equal(arpReq.DstProtAddress) {
				// 过滤非本机地址
				continue
			}

			// 返回ARP数据
			src := &arpdis.Addr{IP: cSess.IpAddr, HardwareAddr: cSess.MacHw}
			dst := &arpdis.Addr{IP: arpReq.SourceProtAddress, HardwareAddr: arpReq.SourceHwAddress}
			data, err = arpdis.NewARPReply(src, dst)
			if err != nil {
				base.Error(err)
				return
			}

			// 从接受的arp信息添加arp地址
			addr := &arpdis.Addr{
				IP:           append([]byte{}, dst.IP...),
				HardwareAddr: append([]byte{}, dst.HardwareAddr...),
			}
			arpdis.Add(addr)

			pl := getPayload()
			// 设置为二层数据类型
			pl.LType = sessdata.LTypeEthernet
			// 拷贝数据到pl
			copy(pl.Data, data)
			// 更新切片长度
			pl.Data = pl.Data[:len(data)]

			if payloadIn(cSess, pl) {
				return
			}

		}
	}
}
