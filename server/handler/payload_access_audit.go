package handler

import (
	"crypto/md5"
	"encoding/binary"
	"net"
	"runtime/debug"
	"time"

	"github.com/songgao/water/waterutil"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

const (
	acc_proto_udp = iota + 1
	acc_proto_tcp
	acc_proto_https
	acc_proto_http
	acc_proto_dns
)

var (
	auditPayload *AuditPayload
	logBatch     *LogBatch
)

// 分析审计日志
type AuditPayload struct {
	Pool       *utils.WorkerPool
	IpAuditMap utils.IMaps
}

// 保存审计日志
type LogBatch struct {
	Logs    []dbdata.AccessAudit
	LogChan chan dbdata.AccessAudit
}

// 异步写入pool
func (p *AuditPayload) Add(userName, groupName string, pl *sessdata.Payload) {
	select {
	case p.Pool.JobQueue <- func() {
		logAudit(userName, groupName, pl)
	}:
	default:
		putPayload(pl)
		base.Error("AccessAudit: AuditPayload channel is full")
	}
}

// 数据落盘并广播到WebSocket
func (l *LogBatch) Write() {
	if len(l.Logs) == 0 {
		return
	}
	err := dbdata.AddBatch(l.Logs)
	if err != nil {
		base.Error("AccessAudit: 批量写入失败:", err)
	}
	l.Reset()
}

// 清空数据
func (l *LogBatch) Reset() {
	l.Logs = []dbdata.AccessAudit{}
}

// 开启批量写入数据功能
func logAuditBatch() {
	if base.GetCfg().AuditInterval < 0 {
		return
	}
	auditPayload = &AuditPayload{
		Pool:       utils.NewWorkerPool(10, 10240),
		IpAuditMap: utils.NewMap("cmap", 0),
	}
	logBatch = &LogBatch{
		LogChan: make(chan dbdata.AccessAudit, 10240),
	}

	// 启动定期清理过期审计去重条目（每 5 分钟）
	go auditMapCleanupLoop()

	var (
		limit       = 100 // 超过上限批量写入数据表
		outTime     = time.NewTimer(time.Second)
		accessAudit = dbdata.AccessAudit{}
	)

	for {
		// 重置超时 时间
		outTime.Reset(time.Second * 1)
		select {
		case accessAudit = <-logBatch.LogChan:
			logBatch.Logs = append(logBatch.Logs, accessAudit)
			if len(logBatch.Logs) >= limit {
				if !outTime.Stop() {
					<-outTime.C
				}
				logBatch.Write()
			}
		case <-outTime.C:
			logBatch.Write()
		}
	}
}

// 解析IP包的数据
func logAudit(userName, groupName string, pl *sessdata.Payload) {
	defer func() {
		if err := recover(); err != nil {
			base.Error("logAudit is panic: ", err, "\n", string(debug.Stack()), "\n", pl.Data)
		}
	}()

	defer func() {
		putPayload(pl)
	}()

	if !(pl.LType == sessdata.LTypeIPData && pl.PType == 0x00) {
		return
	}

	// 按 IP 版本提取五元组（v4/v6 统一审计口径；v6 复用 parseV6Header）
	var ipProto waterutil.IPProtocol
	var ipSrc, ipDst net.IP
	var ipPort uint16

	var tcpSeg []byte // TCP 段（含 TCP 头，不含 IP 头），供域名提取使用
	var udpSeg []byte // UDP 负载，供 DNS 域名提取使用

	switch (pl.Data[0] & 0xF0) >> 4 {
	case 4:
		ipProto = waterutil.IPv4Protocol(pl.Data)
		ipSrc = waterutil.IPv4Source(pl.Data)
		ipDst = waterutil.IPv4Destination(pl.Data)
		ipPl := waterutil.IPv4Payload(pl.Data)
		if len(ipPl) < 4 {
			base.Error("ipPl len < 4", ipPl, pl.Data)
			return
		}
		_ = (uint16(ipPl[0]) << 8) | uint16(ipPl[1]) // srcPort: 审计不记录
		ipPort = (uint16(ipPl[2]) << 8) | uint16(ipPl[3])
		// 用 IP 总长度字段更稳妥地定位上层负载
		ipTotalLen := min(int(binary.BigEndian.Uint16(pl.Data[2:4])), len(pl.Data))
		switch ipProto {
		case waterutil.TCP:
			// IPv4 头长度（IHL）
			ihl := int(pl.Data[0]&0x0f) << 2
			if ihl >= 20 && ihl < ipTotalLen {
				tcpSeg = pl.Data[ihl:ipTotalLen]
			}
		case waterutil.UDP:
			ihl := int(pl.Data[0]&0x0f) << 2
			if ihl >= 20 && ihl < ipTotalLen {
				udpSeg = pl.Data[ihl:ipTotalLen]
			}
		}
	case 6:
		info, ok := parseV6Header(pl.Data)
		if !ok {
			return // 无法解析的 v6 包不审计（安全跳过）
		}
		ipProto = waterutil.IPProtocol(info.Proto)
		ipSrc = info.Src
		ipDst = info.Dst
		if info.Proto != 6 && info.Proto != 17 {
			return // 非 TCP/UDP 不审计（与 v4 一致）
		}
		ipPort = info.DstPort
		if info.L4Off < len(pl.Data) {
			switch info.Proto {
			case 6:
				tcpSeg = pl.Data[info.L4Off:]
			case 17:
				udpSeg = pl.Data[info.L4Off:]
			}
		}
	default:
		return
	}

	// 访问协议
	var accessProto uint8
	// 只统计 tcp和udp 的访问
	switch ipProto {
	case waterutil.TCP:
		accessProto = acc_proto_tcp
	case waterutil.UDP:
		accessProto = acc_proto_udp
	default:
		return
	}
	b := getByte51()
	// key格式 16字节源IP地址 + 16字节目的IP地址 + 2字节目的端口 + 1字节协议类型 + 16字节域名MD5
	key := *b
	copy(key[:16], ipSrc)
	copy(key[16:32], ipDst)
	binary.BigEndian.PutUint16(key[32:34], ipPort)
	key[34] = byte(accessProto)
	copy(key[35:51], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})

	info := ""
	nu := utils.NowSec().Unix()

	// 域名提取：TCP 走 SNI/HTTP；目的端口 53 的 UDP 走 DNS Question。
	if accessProto == acc_proto_tcp && len(tcpSeg) >= 14 {
		// 仅首包（PSH+ACK）尝试解析，避免每包都解析
		if tcpSeg[13]&0x18 == 0x18 {
			accessProto, info = onTCP(tcpSeg)
		}
	} else if accessProto == acc_proto_udp && ipPort == 53 && len(udpSeg) >= 12 {
		if name := parseDNSQuery(udpSeg); name != "" {
			accessProto = acc_proto_dns
			info = name
		}
	}

	// FakeDNS 兜底：报文本身没有携带域名，但目的地址是 FakeIP 时，通过映射反查域名。
	// 直接访问全局单例，避免在测试或未初始化场景触发懒加载。
	if info == "" && sessdata.GlobalFakeDNSManager != nil {
		if domain := sessdata.GlobalFakeDNSManager.GetDomain(ipDst.String()); domain != "" {
			info = domain
		}
	}

	// 任何命中域名的记录：把"仅 IP"的去重键提前占位，避免既记域名又记一笔裸 IP。
	if info != "" {
		ipKey := make([]byte, 51)
		copy(ipKey, key)
		// 占位键使用域名所依赖的底层传输协议：HTTPS/HTTP→TCP，DNS→UDP，FakeDNS 兜底保持当前协议
		placeholderProto := accessProto
		switch accessProto {
		case acc_proto_https, acc_proto_http:
			placeholderProto = acc_proto_tcp
		case acc_proto_dns:
			placeholderProto = acc_proto_udp
		}
		ipKey[34] = byte(placeholderProto)
		ipS := utils.BytesToString(ipKey)
		auditPayload.IpAuditMap.Set(ipS, nu)

		key[34] = byte(accessProto)
		// 存储含域名的key
		md5Sum := md5.Sum([]byte(info))
		copy(key[35:51], md5Sum[:])
	}
	s := utils.BytesToString(key)

	// 判断已经存在，并且没有过期
	v, ok := auditPayload.IpAuditMap.Get(s)
	if ok && nu-v.(int64) < int64(base.GetCfg().AuditInterval) {
		// 回收byte对象
		putByte51(b)
		return
	}

	auditPayload.IpAuditMap.Set(s, nu)

	audit := dbdata.AccessAudit{
		Username:    userName,
		GroupName:   groupName,
		Protocol:    uint8(ipProto),
		Src:         ipSrc.String(),
		Dst:         ipDst.String(),
		DstPort:     ipPort,
		CreatedAt:   utils.NowSec(),
		AccessProto: accessProto,
		Info:        info,
	}
	select {
	case logBatch.LogChan <- audit:
	default:
		base.Error("AccessAudit: LogChan channel is full")
		return
	}
}

// 定期清理过期的审计去重条目，防止内存泄漏
func auditMapCleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if auditPayload == nil {
			return
		}
		now := utils.NowSec().Unix()
		interval := int64(base.GetCfg().AuditInterval)
		if interval <= 0 {
			continue
		}
		expireBefore := now - interval*2 // 保留 2 倍间隔的余量
		for k, v := range auditPayload.IpAuditMap.Items() {
			if ts, ok := v.(int64); ok && ts < expireBefore {
				auditPayload.IpAuditMap.Del(k)
			}
		}
	}
}
