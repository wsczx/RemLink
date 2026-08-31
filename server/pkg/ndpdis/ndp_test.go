package ndpdis

import (
	"net"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
)

func TestTable(t *testing.T) {
	ast := assert.New(t)
	ip := net.ParseIP("2001:db8:1::2")
	hw := net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}

	ast.Nil(Lookup(ip))
	Add(ip, hw)
	ast.Equal(hw, Lookup(ip))
	Delete(ip)
	ast.Nil(Lookup(ip))
}

func TestNewNAReply(t *testing.T) {
	ast := assert.New(t)
	targetIP := net.ParseIP("2001:db8:1::2")
	targetHw := net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}
	dstIP := net.ParseIP("2001:db8:1::1")
	dstHw := net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x02}

	data, err := NewNAReply(targetIP, targetHw, dstIP, dstHw)
	ast.Nil(err)

	packet := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.NoCopy)

	eth := packet.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
	ast.Equal(targetHw, eth.SrcMAC)
	ast.Equal(dstHw, eth.DstMAC)
	ast.Equal(layers.EthernetTypeIPv6, eth.EthernetType)

	ip6 := packet.Layer(layers.LayerTypeIPv6).(*layers.IPv6)
	ast.True(ip6.SrcIP.Equal(targetIP))
	ast.True(ip6.DstIP.Equal(dstIP))
	ast.Equal(uint8(255), ip6.HopLimit)

	naLayer := packet.Layer(layers.LayerTypeICMPv6NeighborAdvertisement)
	ast.NotNil(naLayer)
	na := naLayer.(*layers.ICMPv6NeighborAdvertisement)
	ast.True(na.TargetAddress.Equal(targetIP))
	ast.Equal(uint8(0x60), na.Flags) // Solicited + Override
	ast.Equal(1, len(na.Options))
	ast.Equal(layers.ICMPv6OptTargetAddress, na.Options[0].Type)
	ast.Equal([]byte(targetHw), na.Options[0].Data)
}
