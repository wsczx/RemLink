package ndpdis

// 提供 IPv6 邻居发现代答

import (
	"net"
	"sync"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var (
	table   = make(map[string]net.HardwareAddr, 128)
	tableMu sync.RWMutex
)

func Add(ip net.IP, hw net.HardwareAddr) {
	if ip == nil || hw == nil {
		return
	}
	tableMu.Lock()
	defer tableMu.Unlock()
	table[ip.String()] = hw
}

func Delete(ip net.IP) {
	if ip == nil {
		return
	}
	tableMu.Lock()
	defer tableMu.Unlock()
	delete(table, ip.String())
}

func Lookup(ip net.IP) net.HardwareAddr {
	tableMu.RLock()
	defer tableMu.RUnlock()
	return table[ip.String()]
}

var serializeOpts = gopacket.SerializeOptions{
	FixLengths:       true,
	ComputeChecksums: true,
}

// 构造邻居通告帧
func NewNAReply(targetIP net.IP, targetHw net.HardwareAddr, dstIP net.IP, dstHw net.HardwareAddr) ([]byte, error) {
	eth := layers.Ethernet{
		SrcMAC:       targetHw,
		DstMAC:       dstHw,
		EthernetType: layers.EthernetTypeIPv6,
	}
	ip6 := layers.IPv6{
		Version:    6,
		HopLimit:   255, // NDP 要求 HopLimit 必须为 255，否则接收方丢弃
		NextHeader: layers.IPProtocolICMPv6,
		SrcIP:      targetIP.To16(),
		DstIP:      dstIP.To16(),
	}
	icmp := layers.ICMPv6{
		TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeNeighborAdvertisement, 0),
	}
	if err := icmp.SetNetworkLayerForChecksum(&ip6); err != nil {
		return nil, err
	}
	na := layers.ICMPv6NeighborAdvertisement{
		Flags:         0x60, // Solicited + Override
		TargetAddress: targetIP.To16(),
		Options: layers.ICMPv6Options{
			{Type: layers.ICMPv6OptTargetAddress, Data: targetHw},
		},
	}

	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, serializeOpts, &eth, &ip6, &icmp, &na); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
