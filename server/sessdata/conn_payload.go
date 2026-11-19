package sessdata

import (
	"encoding/binary"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/songgao/water/waterutil"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

// 保存 IPv6 基础头和扩展头解析结果
type V6HeaderInfo struct {
	Proto          uint8
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

// 解析 IPv6 包，跳过扩展头链，定位上层协议与端口
// 返回 ok=false 表示报文长度非法、版本非 v6 或扩展头链无法收敛（畸形报文），调用方应安全拒绝
func ParseV6Header(data []byte) (V6HeaderInfo, bool) {
	var info V6HeaderInfo
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

// 处理入方向数据包：DNS 拦截、FakeIP 还原、LinkAcl 校验，最后送入 PayloadIn 通道
func (c *ConnSession) PayloadInbound(pl *Payload) bool {
	// DNS 拦截
	if c.InterceptDNS(pl) {
		return false // DNS 已处理,不继续转发
	}
	// FakeIP 还原
	if c.RestoreFakeIP(pl) {
		return false // 已处理,不继续转发
	}

	if pl.LType == LTypeIPData && pl.PType == 0x00 {
		// FakeIP 目的段不受 LinkAcl 限制：fakeIP 是 FakeDNS 接管域名的占位地址
		// 真实目的在内核 PREROUTING DNAT 后才确定（v4/v6 统一），此处按 v4/v6 放行
		if !c.IsFakeIPDst(pl) {
			// 进行Acl规则判断
			if !c.CheckLinkAcl(pl) {
				// 校验不通过直接丢弃
				return false
			}
		}
	}

	select {
	case <-c.CloseChan:
		return true
	default:
	}
	select {
	case c.PayloadIn <- pl:
	case <-c.CloseChan:
		return true
	}
	return false
}

// 按当前会话是否有 DTLS 通道，路由到 CSTP 或 DTLS 出方向
func (c *ConnSession) PayloadOut(pl *Payload) bool {
	dSess := c.GetDtlsSession()
	if dSess == nil {
		return c.CstpPayloadOut(pl)
	}
	return c.payloadOutDtls(dSess, pl)
}

// 直接送入 CSTP 出方向通道（不按 DTLS 状态路由），供 CSTP 控制报文原通道应答
func (c *ConnSession) CstpPayloadOut(pl *Payload) bool {
	select {
	case <-c.CloseChan:
		return true
	default:
	}
	select {
	case c.PayloadOutCstp <- pl:
	case <-c.CloseChan:
		return true
	}
	return false
}

// 与 CstpPayloadOut 对称：任一关闭信号触发时返回 true，让 DPD 读循环及时退出
func (c *ConnSession) payloadOutDtls(dSess *DtlsSession, pl *Payload) bool {
	select {
	case <-dSess.CloseChan:
		return true
	case <-c.CloseChan:
		return true
	default:
	}
	select {
	case c.PayloadOutDtls <- pl:
	case <-dSess.CloseChan:
		return true
	case <-c.CloseChan:
		return true
	}
	return false
}

// 判断包的目的地址是否落在 FakeDNS 地址段内（v4/v6 双池）
// FakeIP 是域名占位地址，真实目的由内核 DNAT 决定，ACL 不应基于占位地址做拦截
func (c *ConnSession) IsFakeIPDst(pl *Payload) bool {
	if c.FakeDNS == nil || !c.Policy.EnableFakeDNS || len(pl.Data) < 20 {
		return false
	}
	var ipDst net.IP
	switch (pl.Data[0] & 0xF0) >> 4 {
	case 4:
		ipDst = waterutil.IPv4Destination(pl.Data)
	case 6:
		info, ok := ParseV6Header(pl.Data)
		if !ok {
			return false
		}
		ipDst = info.Dst
	default:
		return false
	}
	return c.FakeDNS.IsFakeIP(ipDst)
}

// Acl规则校验（v4/v6 统一；v6 复用 LinkAcl 的 *net.IPNet，兼容 v6 CIDR 规则）
func (c *ConnSession) CheckLinkAcl(pl *Payload) bool {
	rp := c.Policy
	if !(pl.LType == LTypeIPData && pl.PType == 0x00) {
		return true
	}
	if len(rp.LinkAcl) == 0 {
		return true // 无 ACL 规则，v4/v6 一致放行
	}

	// 最短 IPv4 头 20 字节；畸形短包无法解析，交给后续路径处理
	if len(pl.Data) < 20 {
		return true
	}
	// 按 IP 版本分流，提取五元组
	var ipDst net.IP
	var ipPort uint16
	var ipProto waterutil.IPProtocol
	isICMP := false

	switch (pl.Data[0] & 0xF0) >> 4 {
	case 4:
		ipProto = waterutil.IPv4Protocol(pl.Data)
		ipDst = waterutil.IPv4Destination(pl.Data)
		ipPort = waterutil.IPv4DestinationPort(pl.Data)
		isICMP = ipProto == waterutil.ICMP
	case 6:
		info, ok := ParseV6Header(pl.Data)
		if !ok {
			// 无法解析的 v6 包：安全拒绝，避免畸形报文绕过 ACL
			return false
		}
		ipDst = info.Dst
		ipPort = info.DstPort
		// ICMPv6(58) 与 v4 ICMP(1) 同语义：一条 icmp allow 规则同时放行 ping 与 ping6
		if info.Proto == 58 {
			ipProto = waterutil.ICMP
			isICMP = true
		} else {
			ipProto = waterutil.IPProtocol(info.Proto)
		}
	default:
		// 非 IPv4/IPv6 的畸形包：安全默认拒绝
		return false
	}

	// 优先放行 dns 端口
	for _, v := range rp.ClientDns {
		if v.Val == ipDst.String() && ipPort == 53 {
			return true
		}
	}

	for _, v := range rp.LinkAcl {
		if v.Protocol == "" || v.Protocol == dbdata.ALL || v.IpProto == ipProto {
			if v.IpNet.Contains(ipDst) {
				// icmp 不判断端口
				if isICMP {
					if v.Action == dbdata.Allow {
						return true
					} else {
						return false
					}
				}

				if utils.ContainsPortInStr(v.Port, ipPort) {
					if v.Action == dbdata.Allow {
						return true
					} else {
						return false
					}
				}
			}
		}
	}

	return false
}

// DNS 拦截 - 返回 FakeIP 或透明转发
func (c *ConnSession) InterceptDNS(pl *Payload) bool {
	ctx, ok := parseDNSQueryPacket(c, pl)
	if !ok {
		return false
	}

	// 命中 FakeDNS 规则的 AAAA 查询
	if ctx.qType == dns.TypeAAAA && shouldUseFakeIP(ctx.domain, ctx.policy) {
		return c.answerAAAARecord(ctx)
	}

	// 只处理 A 记录查询
	if ctx.qType != dns.TypeA {
		return false
	}
	if !shouldUseFakeIP(ctx.domain, ctx.policy) {
		base.Debug("Domain not matched FakeDNS rules, Not Intercepting:", ctx.domain)
		return false // 不拦截，正常转发
	}
	return c.answerARecord(ctx)
}

// 比较配置的 DNS 服务器地址与报文目的地址
func dnsAddrEqual(cfgVal string, dst net.IP) bool {
	if cfgVal == dst.String() {
		return true
	}
	if ip := net.ParseIP(cfgVal); ip != nil {
		return ip.Equal(dst)
	}
	return false
}

// 按传输层版本构造 DNS 响应包
func buildDNSResponse(queryPacket []byte, v6Info V6HeaderInfo, isV6 bool, dnsResp []byte) []byte {
	if isV6 {
		return buildDNSResponsePacket6(v6Info, dnsResp)
	}
	return buildDNSResponsePacket(queryPacket, dnsResp)
}

// 还原 FakeIP 为真实域名并解析
func (c *ConnSession) RestoreFakeIP(pl *Payload) bool {
	rp := c.Policy
	if c.FakeDNS == nil || !rp.EnableFakeDNS || len(pl.Data) < 20 {
		return false
	}

	// 按 IP 版本取目的地址；v6 复用共享头解析
	var ipDst net.IP
	switch (pl.Data[0] & 0xF0) >> 4 {
	case 4:
		ipDst = waterutil.IPv4Destination(pl.Data)
	case 6:
		info, ok := ParseV6Header(pl.Data)
		if !ok {
			return false // 畸形 v6 包交给后续 ACL 处理
		}
		ipDst = info.Dst
	default:
		return false
	}

	if !c.FakeDNS.IsFakeIP(ipDst) {
		return false
	}

	fakeIPStr := ipDst.String()

	// 查 realIP、更新访问时间、判断是否需要刷新、取 domain
	realIP, domain, needRefresh := c.FakeDNS.LookupAndTouch(fakeIPStr)

	// 已有映射: 命中转发, 到期则异步换节点
	if realIP != "" {
		if needRefresh {
			c.FakeDNS.RenewMapping(fakeIPStr, domain, FormatUpstream(c.Policy.GetUpstreamDNS()))
		}
		return false
	}

	// fakeIP 在池范围内但不在映射表中，是客户端缓存的旧 IP
	if domain == "" {
		base.Trace("FakeIP not in map (stale cache), dropping:", ipDst)
		return true
	}

	// 首次访问: 异步解析并写入 NAT 规则, 丢包让客户端 TCP 重传后恢复
	upstreamDNS := FormatUpstream(c.Policy.GetUpstreamDNS())
	c.FakeDNS.ResolveAndMapping(fakeIPStr, domain, upstreamDNS)

	base.Debug("FakeIP mapping not ready, dropping packet, resolving async:", domain)
	return true
}

// 检查域名是否应该返回 FakeIP
func shouldUseFakeIP(domain string, p *dbdata.Policy) bool {
	domain = strings.ToLower(domain)

	// 有 include 规则：仅匹配 include 列表的域名返回 FakeIP
	if len(p.FakeDNSIncludeSet) > 0 {
		if len(p.FakeDNSExcludeSet) > 0 {
			base.Warn("FakeDNS: include and exclude both set, exclude will be ignored")
		}
		return matchDomainSet(domain, p.FakeDNSIncludeSet)
	}

	// 有 exclude 规则但没有 include：默认全部拦截，但排除 exclude 列表
	if len(p.FakeDNSExcludeSet) > 0 {
		return !matchDomainSet(domain, p.FakeDNSExcludeSet)
	}

	// 没有任何规则时不做拦截，避免误接管所有 DNS 流量
	return false
}

// 匹配域名集合
func matchDomainSet(domain string, set map[string]struct{}) bool {
	d := domain
	for {
		if _, ok := set[d]; ok {
			return true // 精确匹配或父域名匹配
		}
		idx := strings.IndexByte(d, '.')
		if idx < 0 {
			break
		}
		d = d[idx+1:] // 取子域名
	}
	return false
}

// 构建 DNS 响应包
func buildDNSResponsePacket(queryPacket []byte, dnsResp []byte) []byte {
	// 从原始查询包的 IHL 字段动态获取 IP 头长度
	ipHeaderLen := int(queryPacket[0]&0x0F) * 4
	udpHeaderLen := 8
	totalLen := ipHeaderLen + udpHeaderLen + len(dnsResp)

	respPacket := make([]byte, totalLen)

	// 复制并修改 IP 头
	copy(respPacket[:ipHeaderLen], queryPacket[:ipHeaderLen])

	// 交换源和目标IP
	copy(respPacket[12:16], queryPacket[16:20])
	copy(respPacket[16:20], queryPacket[12:16])

	binary.BigEndian.PutUint16(respPacket[2:4], uint16(totalLen))

	respPacket[10] = 0
	respPacket[11] = 0
	ipChecksum := calculateChecksum(respPacket[:ipHeaderLen])
	binary.BigEndian.PutUint16(respPacket[10:12], ipChecksum)

	copy(respPacket[ipHeaderLen:ipHeaderLen+udpHeaderLen], queryPacket[ipHeaderLen:ipHeaderLen+udpHeaderLen])

	// 交换源和目标端口
	copy(respPacket[ipHeaderLen:ipHeaderLen+2], queryPacket[ipHeaderLen+2:ipHeaderLen+4])
	copy(respPacket[ipHeaderLen+2:ipHeaderLen+4], queryPacket[ipHeaderLen:ipHeaderLen+2])

	udpLen := udpHeaderLen + len(dnsResp)
	binary.BigEndian.PutUint16(respPacket[ipHeaderLen+4:ipHeaderLen+6], uint16(udpLen))

	copy(respPacket[ipHeaderLen+udpHeaderLen:], dnsResp)

	// UDP 校验和（IPv4 伪头）
	respPacket[ipHeaderLen+6] = 0
	respPacket[ipHeaderLen+7] = 0
	pseudo := make([]byte, 0, 12+udpLen)
	pseudo = append(pseudo, respPacket[12:20]...) // 源 + 目的 IP
	pseudo = append(pseudo, 0, 17)                // 零 + 协议号 UDP
	pseudo = binary.BigEndian.AppendUint16(pseudo, uint16(udpLen))
	pseudo = append(pseudo, respPacket[ipHeaderLen:]...)
	udpCsum := calculateChecksum(pseudo)
	if udpCsum == 0 {
		udpCsum = 0xffff // RFC 768：计算结果 0 需翻转为全 1
	}
	binary.BigEndian.PutUint16(respPacket[ipHeaderLen+6:ipHeaderLen+8], udpCsum)

	return respPacket
}

// 构建 IPv6 DNS 响应包：全新 40 字节基础头（不携带查询包的扩展头）+ UDP + DNS 载荷
// IPv6 的 UDP 校验和为强制项，必须按含伪头计算，否则客户端内核直接丢包
func buildDNSResponsePacket6(info V6HeaderInfo, dnsResp []byte) []byte {
	udpLen := 8 + len(dnsResp)
	respPacket := make([]byte, 40+udpLen)

	// IPv6 基础头
	respPacket[0] = 0x60 // Version=6
	binary.BigEndian.PutUint16(respPacket[4:6], uint16(udpLen))
	respPacket[6] = 17                       // Next Header: UDP
	respPacket[7] = 64                       // Hop Limit
	copy(respPacket[8:24], info.Dst.To16())  // 源 = 查询包目的（DNS 服务器）
	copy(respPacket[24:40], info.Src.To16()) // 目的 = 查询包源（客户端）

	// UDP 头（源/目的端口对调）
	binary.BigEndian.PutUint16(respPacket[40:42], info.DstPort)
	binary.BigEndian.PutUint16(respPacket[42:44], info.SrcPort)
	binary.BigEndian.PutUint16(respPacket[44:46], uint16(udpLen))
	copy(respPacket[48:], dnsResp)

	// UDP 校验和（IPv6 伪头: 源 + 目的 + uint32 UDP 长度 + 3 字节零 + Next Header）
	pseudo := make([]byte, 0, 40+udpLen)
	pseudo = append(pseudo, respPacket[8:40]...)
	pseudo = binary.BigEndian.AppendUint32(pseudo, uint32(udpLen))
	pseudo = append(pseudo, 0, 0, 0, 17)
	pseudo = append(pseudo, respPacket[40:]...)
	csum := calculateChecksum(pseudo)
	if csum == 0 {
		csum = 0xffff // RFC 768/8200：0 表示无校验和，需翻转为全 1
	}
	binary.BigEndian.PutUint16(respPacket[46:48], csum)

	return respPacket
}

func calculateChecksum(data []byte) uint16 {
	sum := uint32(0)
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(data[i])<<8 | uint32(data[i+1])
	}
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}
	for sum > 0xffff {
		sum = (sum >> 16) + (sum & 0xffff)
	}
	return ^uint16(sum)
}

// 发送 DNS 响应包到客户端
func (c *ConnSession) sendDNSResponse(respPacket []byte) {
	respPl := Payloads.GetPayload()
	// 超长响应不能静默截断，直接丢弃并告警
	if len(respPacket) > len(respPl.Data) {
		Payloads.PutPayload(respPl)
		base.Warn("DNS response exceeds buffer, dropped:", len(respPacket))
		return
	}
	respPl.LType = LTypeIPData
	respPl.PType = 0x00
	copy(respPl.Data, respPacket)
	respPl.Data = respPl.Data[:len(respPacket)]

	dSess := c.GetDtlsSession()
	var ch chan *Payload
	if dSess != nil {
		ch = c.PayloadOutDtls
	} else {
		ch = c.PayloadOutCstp
	}

	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()

	select {
	case ch <- respPl:
	case <-c.CloseChan:
		Payloads.PutPayload(respPl)
	case <-timer.C:
		Payloads.PutPayload(respPl)
		base.Debug("DNS response dropped: outbound channel full")
	}
}
