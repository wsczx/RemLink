package handler

import (
	"encoding/binary"
	"net"
)

// 保存 IPv6 基础头和扩展头解析结果
type v6HeaderInfo struct {
	Proto          uint8  // 最终上层协议号 (TCP=6 / UDP=17 / ICMPv6=58 / ...)
	Src            net.IP // 16 字节源地址
	Dst            net.IP // 16 字节目的地址
	SrcPort        uint16
	DstPort        uint16
	L4Off          int // 上层协议头（TCP/UDP/ICMPv6...）在报文中的偏移，用于定位载荷
	PacketEnd      int // IPv6 Payload Length 对应的有效包尾（不含尾部填充）
	FragmentID     uint32
	FragmentOffset uint16 // 以字节为单位
	MoreFragments  bool
	IsFragment     bool
}

// 解析 IPv6 包，跳过扩展头链，定位上层协议与端口。
// 返回 ok=false 表示报文长度非法、版本非 v6 或扩展头链无法收敛（畸形报文），调用方应安全拒绝。
func parseV6Header(data []byte) (v6HeaderInfo, bool) {
	var info v6HeaderInfo
	if len(data) < 40 {
		return info, false
	}
	if data[0]>>4 != 6 {
		return info, false
	}

	payloadLen := int(binary.BigEndian.Uint16(data[4:6]))
	packetEnd := 40 + payloadLen
	if packetEnd > len(data) {
		return info, false
	}
	nextHeader := data[6]
	info.Src = make(net.IP, 16)
	info.Dst = make(net.IP, 16)
	copy(info.Src, data[8:24])
	copy(info.Dst, data[24:40])
	offset := 40

	// 逐跳跳过扩展头；上限防止畸形报文导致死循环
	const maxExtHdrs = 12
	for range maxExtHdrs {
		switch nextHeader {
		case 0, 43, 60, 135: // Hop-by-Hop / Routing / Destination Options / Mobility
			if offset+2 > packetEnd {
				return info, false
			}
			// Hdr Ext Len 字段以 8 字节为单位，不含前 8 字节 → 头长 = (HdrExtLen+1)*8
			hdrLen := int(data[offset+1]) + 1
			nextHeader = data[offset]
			offset += hdrLen * 8
		case 51: // AH: 头长 = (Payload Len + 2) * 4
			if offset+2 > packetEnd {
				return info, false
			}
			hdrLen := (int(data[offset+1]) + 2) * 4
			nextHeader = data[offset]
			offset += hdrLen
		case 44: // Fragment: 固定 8 字节，无 Hdr Ext Len 字段
			if offset+8 > packetEnd {
				return info, false
			}
			fragmentField := binary.BigEndian.Uint16(data[offset+2 : offset+4])
			info.IsFragment = true
			info.FragmentOffset = (fragmentField >> 3) * 8
			info.MoreFragments = fragmentField&1 != 0
			info.FragmentID = binary.BigEndian.Uint32(data[offset+4 : offset+8])
			nextHeader = data[offset]
			offset += 8
		case 50: // ESP: 加密载荷，无法继续解析上层，到此为止
			info.Proto = 50
			info.L4Off = offset
			info.PacketEnd = packetEnd
			return info, true
		default: // TCP(6) / UDP(17) / ICMPv6(58) / 其他上层协议
			info.Proto = nextHeader
			info.L4Off = offset
			info.PacketEnd = packetEnd
			if info.IsFragment && info.FragmentOffset != 0 {
				return info, true
			}
			if info.IsFragment && info.MoreFragments {
				if offset+4 <= packetEnd && (nextHeader == 6 || nextHeader == 17) {
					info.SrcPort = binary.BigEndian.Uint16(data[offset : offset+2])
					info.DstPort = binary.BigEndian.Uint16(data[offset+2 : offset+4])
				}
				return info, true
			}
			if nextHeader == 6 && offset+20 > packetEnd {
				return info, false
			}
			if nextHeader == 17 && offset+8 > packetEnd {
				return info, false
			}
			if nextHeader == 6 || nextHeader == 17 {
				info.SrcPort = binary.BigEndian.Uint16(data[offset : offset+2])
				info.DstPort = binary.BigEndian.Uint16(data[offset+2 : offset+4])
			}
			return info, true
		}
		if offset > packetEnd {
			return info, false
		}
	}
	return info, false
}
