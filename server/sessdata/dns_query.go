package sessdata

import (
	"net"
	"strings"

	"github.com/miekg/dns"
	"github.com/songgao/water/waterutil"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// DNS 查询报文解析后的上下文，供 A/AAAA 两条应答路径共用
type dnsQueryCtx struct {
	msg    *dns.Msg
	domain string
	qType  uint16
	rawIP  []byte // 原始 IP 报文，构造响应包时需要
	v6Info V6HeaderInfo
	isV6   bool
	policy *dbdata.Policy
}

// 校验报文、判定目标 DNS 服务器并解析查询，失败时不拦截
// v4/v6 统一提取五元组与 DNS 载荷
func parseDNSQueryPacket(c *ConnSession, pl *Payload) (dnsQueryCtx, bool) {
	var ctx dnsQueryCtx
	rp := c.Policy
	if c.FakeDNS == nil || !rp.EnableFakeDNS {
		return ctx, false
	}
	if pl.LType != LTypeIPData || pl.PType != 0x00 {
		return ctx, false
	}
	if len(pl.Data) < 1 {
		return ctx, false
	}

	var (
		ipDst   net.IP
		ipPort  uint16
		dnsData []byte
	)
	switch (pl.Data[0] & 0xF0) >> 4 {
	case 4:
		ipHeaderLen := int(pl.Data[0]&0x0F) * 4
		// 验证 IHL 值的合法性(5-15,对应 20-60 字节)
		if ipHeaderLen < 20 || ipHeaderLen > 60 {
			if ipHeaderLen != 0 {
				base.Debug("Invalid IP header length:", ipHeaderLen, "bytes, dropping")
			}
			return ctx, false
		}
		if len(pl.Data) < ipHeaderLen+8 {
			base.Debug("Packet too short:", len(pl.Data), "bytes, dropping")
			return ctx, false
		}
		if waterutil.IPv4Protocol(pl.Data) != waterutil.UDP {
			return ctx, false
		}
		ipDst = waterutil.IPv4Destination(pl.Data)
		ipPort = waterutil.IPv4DestinationPort(pl.Data)
		dnsData = pl.Data[ipHeaderLen+8:]
	case 6:
		info, ok := ParseV6Header(pl.Data)
		if !ok || info.Proto != 17 { // 只处理 UDP
			return ctx, false
		}
		if info.PacketEnd < info.L4Off+8 {
			return ctx, false
		}
		ctx.v6Info, ctx.isV6 = info, true
		ipDst = info.Dst
		ipPort = info.DstPort
		dnsData = pl.Data[info.L4Off+8 : info.PacketEnd]
	default:
		return ctx, false
	}

	if ipPort != 53 {
		return ctx, false
	}

	isDNSServer := false
	for _, dnsServer := range rp.ClientDns {
		if dnsAddrEqual(dnsServer.Val, ipDst) {
			isDNSServer = true
			break
		}
	}
	if !isDNSServer && rp.FakeDNSUpstream != "" && dnsAddrEqual(rp.FakeDNSUpstream, ipDst) {
		isDNSServer = true
	}
	if !isDNSServer {
		return ctx, false
	}

	msg := new(dns.Msg)
	if err := msg.Unpack(dnsData); err != nil {
		base.Debug("Failed to parse DNS query:", err)
		return ctx, false
	}
	if len(msg.Question) == 0 {
		return ctx, false
	}

	ctx.msg = msg
	ctx.domain = strings.TrimSuffix(msg.Question[0].Name, ".")
	ctx.qType = msg.Question[0].Qtype
	ctx.rawIP = pl.Data
	ctx.policy = rp
	return ctx, true
}

// 处理命中 FakeDNS 规则的 AAAA 查询
// 双栈未开、或 IPv6 优先关闭、或上游确认无 AAAA 时返回 NODATA 引导客户端走 v4
func (c *ConnSession) answerAAAARecord(ctx dnsQueryCtx) bool {
	msg, domain := ctx.msg, ctx.domain
	if !c.FakeDNS.IsV6Enabled() {
		return false // 未开双栈，不接管 AAAA，放行真实解析
	}
	resp := new(dns.Msg)
	resp.SetReply(msg)
	// IPv6 优先关闭时回 NODATA：双栈下 A/AAAA 同时拿 fakeIP 会让客户端在
	// v4/v6 间竞速，v6 DNAT 路径尚未就绪将形成黑洞
	if !c.Policy.PreferIPv6 {
		dnsResp, err := resp.Pack()
		if err != nil {
			return false
		}
		c.sendDNSResponse(buildDNSResponse(ctx.rawIP, ctx.v6Info, ctx.isV6, dnsResp))
		return true
	}
	upstream := FormatUpstream(c.Policy.GetUpstreamDNS())
	if c.FakeDNS.IsAAAANegative(domain, upstream) {
		// 已确认上游无 AAAA：不回 v6 fakeIP（否则黑洞），回 NODATA 引导走 v4
		dnsResp, err := resp.Pack()
		if err != nil {
			return false
		}
		c.sendDNSResponse(buildDNSResponse(ctx.rawIP, ctx.v6Info, ctx.isV6, dnsResp))
		return true
	}
	if fakeIP6 := c.FakeDNS.AcquireFakeIPv6(domain); fakeIP6 != nil {
		// 映射完成后再返回 AAAA FakeIP，避免客户端立即连接尚未就绪的地址
		if err := c.FakeDNS.ResolveAndMappingSync(fakeIP6.String(), domain, upstream); err == nil {
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
	c.sendDNSResponse(buildDNSResponse(ctx.rawIP, ctx.v6Info, ctx.isV6, dnsResp))
	return true
}

// 在 IPv6 优先且上游已确认 AAAA 可达时，抑制 A 应答回 NODATA
// 首次/未知域名不抑制，回 v4 fakeIP，避免「抑制 A 但 v6 映射建不起来」导致全黑洞
func (c *ConnSession) suppressAForIPv6(ctx dnsQueryCtx) bool {
	if !c.Policy.PreferIPv6 || !c.FakeDNS.IsV6Enabled() {
		return false
	}
	upstream := FormatUpstream(c.Policy.GetUpstreamDNS())
	if !c.FakeDNS.IsAAAAPositive(ctx.domain, upstream) {
		return false
	}
	fakeIP6 := c.FakeDNS.AcquireFakeIPv6(ctx.domain)
	if fakeIP6 == nil {
		return false
	}
	c.FakeDNS.ResolveAndMapping(fakeIP6.String(), ctx.domain, upstream)
	base.Debug("PreferIPv6: AAAA confirmed, suppress A for:", ctx.domain, "->", fakeIP6.String())
	resp := new(dns.Msg)
	resp.SetReply(ctx.msg) // 空 Answer = NODATA
	dnsResp, err := resp.Pack()
	if err != nil {
		return false
	}
	c.sendDNSResponse(buildDNSResponse(ctx.rawIP, ctx.v6Info, ctx.isV6, dnsResp))
	return true
}

// 处理命中 FakeDNS 规则的 A 查询：分配 v4 FakeIP、同步建立 DNAT 后应答
func (c *ConnSession) answerARecord(ctx dnsQueryCtx) bool {
	if c.suppressAForIPv6(ctx) {
		return true
	}

	base.Debug("Intercepting DNS query for:", ctx.domain)

	fakeIP := c.FakeDNS.AcquireFakeIP(ctx.domain)
	if fakeIP == nil {
		base.Error("Failed to acquire FakeIP for:", ctx.domain)
		return false
	}
	base.Debug("Allocated FakeIP:", ctx.domain, "->", fakeIP.String())
	// 先完成真实地址解析和 DNAT，再把 FakeIP 返回给客户端，避免首个连接落入未就绪黑洞
	upstream := FormatUpstream(c.Policy.GetUpstreamDNS())
	if err := c.FakeDNS.ResolveAndMappingSync(fakeIP.String(), ctx.domain, upstream); err != nil {
		base.Debug("Failed to synchronously map FakeIP:", ctx.domain, "error:", err)
		return false // 不向客户端返回永远无法转发的 FakeIP；交由普通 DNS 转发
	}

	resp := new(dns.Msg)
	resp.SetReply(ctx.msg)
	resp.Answer = append(resp.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   ctx.msg.Question[0].Name,
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
	c.sendDNSResponse(buildDNSResponse(ctx.rawIP, ctx.v6Info, ctx.isV6, dnsResp))
	return true
}
