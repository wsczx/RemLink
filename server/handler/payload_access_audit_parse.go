package handler

import (
	"encoding/binary"
	"net"
	"time"

	"github.com/songgao/water/waterutil"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/sessdata"
)

// 从 IP 报文提取的审计要素，v4/v6 统一口径
type auditPacket struct {
	proto            waterutil.IPProtocol
	src, dst         net.IP
	srcPort, dstPort uint16
	tcpSeg, udpSeg   []byte // L4 段（含头），供域名提取使用
}

// 解析 IP 报文的五元组与 L4 段；分片非首片返回 false
// v6 在此过滤非 TCP/UDP，v4 的协议过滤由调用方完成
func parseAuditPacket(pl *sessdata.Payload) (auditPacket, bool) {
	var pkt auditPacket
	// 最短 IPv4 头 20 字节；更短的报文无法取版本字段，直接跳过
	if len(pl.Data) < 20 {
		return pkt, false
	}
	switch (pl.Data[0] & 0xF0) >> 4 {
	case 4:
		pkt.proto = waterutil.IPv4Protocol(pl.Data)
		pkt.src = waterutil.IPv4Source(pl.Data)
		pkt.dst = waterutil.IPv4Destination(pl.Data)
		ipPl := waterutil.IPv4Payload(pl.Data)
		if len(ipPl) < 4 {
			base.Error("ipPl len < 4", ipPl, pl.Data)
			return pkt, false
		}
		// IPv4 非首片不包含 TCP/UDP 头，不能把载荷前四字节误当作端口
		fragmentOffset := binary.BigEndian.Uint16(pl.Data[6:8]) & 0x1fff
		if fragmentOffset != 0 {
			return pkt, false
		}
		pkt.srcPort = binary.BigEndian.Uint16(ipPl[:2])
		pkt.dstPort = binary.BigEndian.Uint16(ipPl[2:4])
		// 用 IP 总长度字段更稳妥地定位上层负载
		ipTotalLen := min(int(binary.BigEndian.Uint16(pl.Data[2:4])), len(pl.Data))
		switch pkt.proto {
		case waterutil.TCP, waterutil.UDP:
			ihl := int(pl.Data[0]&0x0f) << 2 // IPv4 头长度（IHL）
			if ihl >= 20 && ihl < ipTotalLen {
				if pkt.proto == waterutil.TCP {
					pkt.tcpSeg = pl.Data[ihl:ipTotalLen]
				} else {
					pkt.udpSeg = pl.Data[ihl:ipTotalLen]
				}
			}
		}
	case 6:
		info, ok := sessdata.ParseV6Header(pl.Data)
		if !ok {
			return pkt, false // 无法解析的 v6 包不审计（安全跳过）
		}
		pkt.proto = waterutil.IPProtocol(info.Proto)
		pkt.src = info.Src
		pkt.dst = info.Dst
		if info.Proto != 6 && info.Proto != 17 {
			return pkt, false // 非 TCP/UDP 不审计（与 v4 一致）
		}
		pkt.srcPort = info.SrcPort
		pkt.dstPort = info.DstPort
		if info.IsFragment && info.FragmentOffset != 0 {
			return pkt, false // 非首片没有端口信息，不生成虚假五元组
		}
		if info.L4Off < len(pl.Data) {
			l4Data := pl.Data[info.L4Off:info.PacketEnd]
			switch info.Proto {
			case 6:
				pkt.tcpSeg = l4Data
			case 17:
				pkt.udpSeg = l4Data
			}
		}
	default:
		return pkt, false
	}
	return pkt, true
}

// 识别访问协议与域名：TCP 走 SNI/HTTP，53 端口 UDP 走 DNS Question，
// 报文本身无域名时再用 FakeDNS 映射兜底
func (a *AccessAudit) resolveDomain(userName, groupName string, pkt auditPacket, accessProto uint8) (uint8, string) {
	info := ""
	if accessProto == acc_proto_tcp && len(pkt.tcpSeg) >= 20 {
		if a.Payload.TCPStreams != nil {
			streamKey := tcpStreamKey{
				Username: userName, GroupName: groupName, Protocol: uint8(pkt.proto),
				Src: pkt.src.String(), Dst: pkt.dst.String(), SrcPort: pkt.srcPort, DstPort: pkt.dstPort,
			}
			accessProto, info, _ = a.Payload.TCPStreams.add(streamKey, pkt.tcpSeg, time.Now())
			if pkt.tcpSeg[13]&0x05 != 0 { // FIN 或 RST：连接结束后立即释放正反向缓存
				a.Payload.TCPStreams.close(streamKey)
			}
		} else {
			accessProto, info = onTCP(pkt.tcpSeg)
		}
	} else if accessProto == acc_proto_udp && pkt.dstPort == 53 && len(pkt.udpSeg) >= 12 {
		if name := parseDNSQuery(pkt.udpSeg); name != "" {
			accessProto = acc_proto_dns
			info = name
		}
	}

	// FakeDNS 兜底：目的地址是 FakeIP 时，通过映射反查域名
	if info == "" {
		if domain := a.sessions.FakeDNSDomain(pkt.dst.String()); domain != "" {
			info = domain
		}
	}
	return accessProto, info
}
