package handler

import (
	"encoding/binary"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/songgao/water/waterutil"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/sessdata"
)

// DNS 拦截 - 返回 FakeIP 或透明转发
func interceptDNS(cSess *sessdata.ConnSession, pl *sessdata.Payload) bool {
	rp := cSess.Policy
	if cSess.FakeDNS == nil || !rp.EnableFakeDNS {
		return false
	}

	if pl.LType != sessdata.LTypeIPData || pl.PType != 0x00 {
		return false
	}

	if len(pl.Data) < 1 {
		return false
	}

	// 按传输层 IP 版本提取目的地址/端口/DNS 载荷（v6 DNS 报文同样拦截）
	var (
		ipDst   net.IP
		ipPort  uint16
		dnsData []byte
		v6Info  v6HeaderInfo // 仅 isV6 时有效，构造 v6 响应包需要
		isV6    bool
	)
	switch (pl.Data[0] & 0xF0) >> 4 {
	case 4:
		ipHeaderLen := int(pl.Data[0]&0x0F) * 4
		// 验证 IHL 值的合法性(5-15,对应 20-60 字节)
		if ipHeaderLen < 20 || ipHeaderLen > 60 {
			if ipHeaderLen != 0 {
				base.Debug("Invalid IP header length:", ipHeaderLen, "bytes, dropping")
			}
			return false
		}
		// 检查数据包是否包含完整的 IP 头 + UDP 头
		if len(pl.Data) < ipHeaderLen+8 {
			base.Debug("Packet too short:", len(pl.Data), "bytes, dropping")
			return false
		}
		// 只处理 UDP 协议
		if waterutil.IPv4Protocol(pl.Data) != waterutil.UDP {
			return false
		}
		ipDst = waterutil.IPv4Destination(pl.Data)
		ipPort = waterutil.IPv4DestinationPort(pl.Data)
		dnsData = pl.Data[ipHeaderLen+8:]
	case 6:
		info, ok := parseV6Header(pl.Data)
		if !ok || info.Proto != 17 { // 只处理 UDP
			return false
		}
		if info.PacketEnd < info.L4Off+8 {
			return false
		}
		v6Info, isV6 = info, true
		ipDst = info.Dst
		ipPort = info.DstPort
		dnsData = pl.Data[info.L4Off+8 : info.PacketEnd]
	default:
		return false
	}

	// 判断是否是DNS请求
	if ipPort != 53 {
		return false
	}

	// 检查是否是配置的ClientDns服务器
	isDNSServer := false
	for _, dnsServer := range rp.ClientDns {
		if dnsAddrEqual(dnsServer.Val, ipDst) {
			isDNSServer = true
			break
		}
	}
	// 检查是否是 FakeDNS 上游服务器
	if !isDNSServer && rp.FakeDNSUpstream != "" {
		if dnsAddrEqual(rp.FakeDNSUpstream, ipDst) {
			isDNSServer = true
		}
	}

	if !isDNSServer {
		return false
	}

	// 解析 DNS 请求
	msg := new(dns.Msg)
	if err := msg.Unpack(dnsData); err != nil {
		base.Debug("Failed to parse DNS query:", err)
		return false
	}

	if len(msg.Question) == 0 {
		return false
	}

	domain := strings.TrimSuffix(msg.Question[0].Name, ".")
	qType := msg.Question[0].Qtype

	// 命中 FakeDNS 规则的 AAAA 查询
	if qType == dns.TypeAAAA && shouldUseFakeIP(domain, rp) {
		if !cSess.FakeDNS.IsV6Enabled() {
			return false // 未开双栈，不接管 AAAA，放行真实解析
		}
		resp := new(dns.Msg)
		resp.SetReply(msg)
		// IPv6 优先关闭：不回 v6 fakeIP，回 NODATA 强制客户端走 v4 fakeIP。
		// 否则双栈下 A/AAAA 同时拿到 fakeIP，客户端在 v4/v6 间竞速，v6 fakeIP→DNAT 路径
		if !cSess.Policy.PreferIPv6 {
			dnsResp, err := resp.Pack()
			if err != nil {
				return false
			}
			sendDNSResponse(cSess, buildDNSResponse(pl.Data, v6Info, isV6, dnsResp))
			return true
		}
		upstream := sessdata.FormatUpstream(cSess.Policy.GetUpstreamDNS())
		if cSess.FakeDNS.IsAAAANegative(domain, upstream) {
			// 已确认上游无 AAAA：不回 v6 fakeIP（否则黑洞），回 NODATA 引导走 v4
			dnsResp, err := resp.Pack()
			if err != nil {
				return false
			}
			sendDNSResponse(cSess, buildDNSResponse(pl.Data, v6Info, isV6, dnsResp))
			return true
		}
		if fakeIP6 := cSess.FakeDNS.AcquireFakeIPv6(domain); fakeIP6 != nil {
			// 映射完成后再返回 AAAA FakeIP，避免客户端立即连接尚未就绪的地址
			if err := cSess.FakeDNS.ResolveAndMappingSync(fakeIP6.String(), domain, upstream); err == nil {
				resp.Answer = append(resp.Answer, &dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   msg.Question[0].Name,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					AAAA: fakeIP6,
				})
			} else {
				base.Debug("Failed to synchronously map AAAA FakeIP:", domain, "error:", err)
				resp.Rcode = dns.RcodeServerFailure
			}
		}
		dnsResp, err := resp.Pack()
		if err != nil {
			return false
		}
		sendDNSResponse(cSess, buildDNSResponse(pl.Data, v6Info, isV6, dnsResp))
		return true
	}

	// 只处理 A 记录查询
	if qType != dns.TypeA {
		return false
	}

	// 检查是否需要返回 FakeIP
	if !shouldUseFakeIP(domain, rp) {
		base.Debug("Domain not matched FakeDNS rules, Not Intercepting:", domain)
		return false // 不拦截，正常转发
	}

	// IPv6 优先：仅当该域名上游 AAAA 已确认可达（异步解析成功后写入的正缓存）才抑制 A 回 NODATA，
	// 首次/未知域名不抑制，回 v4 fakeIP，由客户端 Happy Eyeballs 竞争 v6，
	// 避免「抑制 A 但 v6 映射建不起来」导致的全黑洞（转发失效）。
	if cSess.Policy.PreferIPv6 && cSess.FakeDNS.IsV6Enabled() {
		upstream := sessdata.FormatUpstream(cSess.Policy.GetUpstreamDNS())
		if cSess.FakeDNS.IsAAAAPositive(domain, upstream) {
			if fakeIP6 := cSess.FakeDNS.AcquireFakeIPv6(domain); fakeIP6 != nil {
				cSess.FakeDNS.ResolveAndMapping(fakeIP6.String(), domain, upstream)
				base.Debug("PreferIPv6: AAAA confirmed, suppress A for:", domain, "->", fakeIP6.String())
				resp := new(dns.Msg)
				resp.SetReply(msg) // 空 Answer = NODATA
				dnsResp, err := resp.Pack()
				if err != nil {
					return false
				}
				sendDNSResponse(cSess, buildDNSResponse(pl.Data, v6Info, isV6, dnsResp))
				return true
			}
		}
		// AAAA 未确认：回退 v4 fakeIP
	}

	base.Debug("Intercepting DNS query for:", domain)

	// 分配 FakeIP
	fakeIP := cSess.FakeDNS.AcquireFakeIP(domain)
	if fakeIP == nil {
		base.Error("Failed to acquire FakeIP for:", domain)
		return false
	}

	base.Debug("Allocated FakeIP:", domain, "->", fakeIP.String())
	// 先完成真实地址解析和 DNAT，再把 FakeIP 返回给客户端，避免首个连接落入未就绪黑洞。
	upstream := sessdata.FormatUpstream(cSess.Policy.GetUpstreamDNS())
	if err := cSess.FakeDNS.ResolveAndMappingSync(fakeIP.String(), domain, upstream); err != nil {
		base.Debug("Failed to synchronously map FakeIP:", domain, "error:", err)
		// 不向客户端返回永远无法转发的 FakeIP；交由普通 DNS 转发。
		return false
	}

	// 构造 DNS 响应
	resp := new(dns.Msg)
	resp.SetReply(msg)
	resp.Answer = append(resp.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   msg.Question[0].Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		A: fakeIP,
	})

	dnsResp, err := resp.Pack()
	if err != nil {
		base.Error("Failed to pack DNS response:", err)
		return false
	}

	// 构造完整的响应包并发送回客户端
	sendDNSResponse(cSess, buildDNSResponse(pl.Data, v6Info, isV6, dnsResp))

	return true
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
func buildDNSResponse(queryPacket []byte, v6Info v6HeaderInfo, isV6 bool, dnsResp []byte) []byte {
	if isV6 {
		return buildDNSResponsePacket6(v6Info, dnsResp)
	}
	return buildDNSResponsePacket(queryPacket, dnsResp)
}

// 还原 FakeIP 为真实域名并解析
func restoreFakeIP(cSess *sessdata.ConnSession, pl *sessdata.Payload) bool {
	rp := cSess.Policy
	if cSess.FakeDNS == nil || !rp.EnableFakeDNS {
		return false
	}

	// 按 IP 版本取目的地址；v6 复用共享头解析
	var ipDst net.IP
	switch (pl.Data[0] & 0xF0) >> 4 {
	case 4:
		ipDst = waterutil.IPv4Destination(pl.Data)
	case 6:
		info, ok := parseV6Header(pl.Data)
		if !ok {
			return false // 畸形 v6 包交给后续 ACL 处理
		}
		ipDst = info.Dst
	default:
		return false
	}

	if !cSess.FakeDNS.IsFakeIP(ipDst) {
		return false
	}

	fakeIPStr := ipDst.String()

	// 查 realIP、更新访问时间、判断是否需要刷新、取 domain
	realIP, domain, needRefresh := cSess.FakeDNS.LookupAndTouch(fakeIPStr)

	// 已有映射: 命中转发, 到期则异步换节点
	if realIP != "" {
		if needRefresh {
			cSess.FakeDNS.RenewMapping(fakeIPStr, domain, sessdata.FormatUpstream(cSess.Policy.GetUpstreamDNS()))
		}
		return false
	}

	// fakeIP 在池范围内但不在映射表中，是客户端缓存的旧 IP
	if domain == "" {
		base.Trace("FakeIP not in map (stale cache), dropping:", ipDst)
		return true
	}

	// 首次访问: 异步解析并写入 NAT 规则, 丢包让客户端 TCP 重传后恢复
	upstreamDNS := sessdata.FormatUpstream(cSess.Policy.GetUpstreamDNS())
	cSess.FakeDNS.ResolveAndMapping(fakeIPStr, domain, upstreamDNS)

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

// 构建 IPv6 DNS 响应包：全新 40 字节基础头（不携带查询包的扩展头）+ UDP + DNS 载荷。
// IPv6 的 UDP 校验和为强制项，必须按含伪头计算，否则客户端内核直接丢包。
func buildDNSResponsePacket6(info v6HeaderInfo, dnsResp []byte) []byte {
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
func sendDNSResponse(cSess *sessdata.ConnSession, respPacket []byte) {
	respPl := getPayload()
	// 超长响应不能静默截断，直接丢弃并告警
	if len(respPacket) > len(respPl.Data) {
		putPayload(respPl)
		base.Warn("DNS response exceeds buffer, dropped:", len(respPacket))
		return
	}
	respPl.LType = sessdata.LTypeIPData
	respPl.PType = 0x00
	copy(respPl.Data, respPacket)
	respPl.Data = respPl.Data[:len(respPacket)]

	dSess := cSess.GetDtlsSession()
	var ch chan *sessdata.Payload
	if dSess != nil {
		ch = cSess.PayloadOutDtls
	} else {
		ch = cSess.PayloadOutCstp
	}

	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()

	select {
	case ch <- respPl:
	case <-cSess.CloseChan:
		putPayload(respPl)
	case <-timer.C:
		putPayload(respPl)
		base.Debug("DNS response dropped: outbound channel full")
	}
}
