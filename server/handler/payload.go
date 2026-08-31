package handler

import (
	"net"

	"github.com/songgao/water/waterutil"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/sessdata"
)

func payloadIn(cSess *sessdata.ConnSession, pl *sessdata.Payload) bool {
	// DNS 拦截
	if interceptDNS(cSess, pl) {
		return false // DNS 已处理,不继续转发
	}
	// FakeIP 还原
	if restoreFakeIP(cSess, pl) {
		return false // FakeIP 已处理,不继续转发
	}

	if pl.LType == sessdata.LTypeIPData && pl.PType == 0x00 {
		// FakeIP 目的段不受 LinkAcl 限制：fakeIP 是 FakeDNS 接管域名的占位地址，
		// 真实目的在内核 PREROUTING DNAT 后才确定（v4/v6 统一），此处按 v4/v6 放行，
		if !isFakeIPDst(cSess, pl) {
			// 进行Acl规则判断
			check := checkLinkAcl(cSess.Policy, pl)
			if !check {
				// 校验不通过直接丢弃
				return false
			}
		}
	}

	closed := false
	select {
	case cSess.PayloadIn <- pl:
	case <-cSess.CloseChan:
		closed = true
	}

	return closed
}

func putPayloadInBefore(cSess *sessdata.ConnSession, pl *sessdata.Payload) {
	// 异步审计日志
	if base.GetCfg().AuditInterval >= 0 {
		auditPayload.Add(cSess.Username, cSess.Group.Name, pl)
		return
	}
	putPayload(pl)
}

func payloadOut(cSess *sessdata.ConnSession, pl *sessdata.Payload) bool {
	dSess := cSess.GetDtlsSession()
	if dSess == nil {
		return payloadOutCstp(cSess, pl)
	} else {
		return payloadOutDtls(cSess, dSess, pl)
	}
}

func payloadOutCstp(cSess *sessdata.ConnSession, pl *sessdata.Payload) bool {
	closed := false

	select {
	case cSess.PayloadOutCstp <- pl:
	case <-cSess.CloseChan:
		closed = true
	}

	return closed
}

func payloadOutDtls(cSess *sessdata.ConnSession, dSess *sessdata.DtlsSession, pl *sessdata.Payload) bool {
	select {
	case cSess.PayloadOutDtls <- pl:
	case <-dSess.CloseChan:
	}

	return false
}

// 判断包的目的地址是否落在 FakeDNS 假地址段内（v4/v6 双池）。
// FakeIP 是域名占位地址，真实目的由内核 DNAT 决定，ACL 不应基于占位地址做拦截。
func isFakeIPDst(cSess *sessdata.ConnSession, pl *sessdata.Payload) bool {
	if cSess.FakeDNS == nil || !cSess.Policy.EnableFakeDNS {
		return false
	}
	var ipDst net.IP
	switch (pl.Data[0] & 0xF0) >> 4 {
	case 4:
		ipDst = waterutil.IPv4Destination(pl.Data)
	case 6:
		info, ok := parseV6Header(pl.Data)
		if !ok {
			return false
		}
		ipDst = info.Dst
	default:
		return false
	}
	return cSess.FakeDNS.IsFakeIP(ipDst)
}

// Acl规则校验（v4/v6 统一；v6 复用 LinkAcl 的 *net.IPNet，兼容 v6 CIDR 规则）
func checkLinkAcl(rp *dbdata.Policy, pl *sessdata.Payload) bool {
	if !(pl.LType == sessdata.LTypeIPData && pl.PType == 0x00) {
		return true
	}
	if len(rp.LinkAcl) == 0 {
		return true // 无 ACL 规则，v4/v6 一致放行
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
		info, ok := parseV6Header(pl.Data)
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

				if dbdata.ContainsPortInStr(v.Port, ipPort) {
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
