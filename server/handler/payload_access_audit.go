package handler

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"runtime/debug"
	"strings"
	"sync"
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
	acc_proto_ssh
	acc_proto_ftp
	acc_proto_smtp
	acc_proto_imap
	acc_proto_pop3
)

// 审计子系统全局状态（审计载荷池、去重表、批量落盘缓冲）由 App 注入
type AccessAudit struct {
	Payload  *AuditPayload
	Batch    *LogBatch
	sessions *sessdata.SessionManager
}

// handler 内部数据面回收回调使用的审计实例（由 Start 从 Services 赋值）
var auditInstance *AccessAudit

// 回收 payload 时按需写入审计，否则直接归还缓冲池
func putPayloadInBefore(cSess *sessdata.ConnSession, pl *sessdata.Payload) {
	if base.GetCfg().AuditInterval >= 0 {
		auditInstance.Add(cSess.Username, cSess.Group.Name, pl)
		return
	}
	sessdata.Payloads.PutPayload(pl)
}

// 由 Start 按需启动
func NewAccessAudit(sessions *sessdata.SessionManager) *AccessAudit {
	return &AccessAudit{
		Payload: &AuditPayload{
			IpAuditMap: &sync.Map{},
			TCPStreams: newTCPStreamCache(),
		},
		Batch: &LogBatch{
			LogChan: make(chan dbdata.AccessAudit, 10240),
		},
		sessions: sessions,
	}
}

// 在审计开启时启动工作协程池，避免配置关闭时常驻协程
func (a *AccessAudit) Start() {
	if a.Payload.Pool == nil {
		a.Payload.Pool = utils.NewWorkerPool(10, 10240)
	}
}

// 分析审计日志
type AuditPayload struct {
	Pool       *utils.WorkerPool
	IpAuditMap *sync.Map
	TCPStreams *tcpStreamCache
}

// 保存审计日志
type LogBatch struct {
	Logs    []dbdata.AccessAudit
	LogChan chan dbdata.AccessAudit
}

type accessAuditMergeKey struct {
	Username  string
	GroupName string
	Protocol  uint8
	Src       string
	Dst       string
	SrcPort   uint16
	DstPort   uint16
}

func accessAuditKey(audit dbdata.AccessAudit) accessAuditMergeKey {
	return accessAuditMergeKey{
		Username:  audit.Username,
		GroupName: audit.GroupName,
		Protocol:  audit.Protocol,
		Src:       audit.Src,
		Dst:       audit.Dst,
		SrcPort:   audit.SrcPort,
		DstPort:   audit.DstPort,
	}
}

func isRawAccessProto(proto uint8) bool {
	return proto == acc_proto_tcp || proto == acc_proto_udp
}

func accessAuditUniqueKey(username, groupName string, key []byte) string {
	result := make([]byte, 4+len(username)+4+len(groupName)+len(key))
	offset := 0
	binary.BigEndian.PutUint32(result[offset:offset+4], uint32(len(username)))
	offset += 4
	offset += copy(result[offset:], username)
	binary.BigEndian.PutUint32(result[offset:offset+4], uint32(len(groupName)))
	offset += 4
	offset += copy(result[offset:], groupName)
	copy(result[offset:], key)
	return utils.BytesToString(result)
}

// 合并同一批次内先产生的 TCP/UDP 占位记录
// 应用协议识别出来后，用 HTTP/HTTPS/DNS 记录替换占位记录；不同域名的应用记录不合并
func (l *LogBatch) appendAudit(incoming dbdata.AccessAudit) {
	key := accessAuditKey(incoming)
	for i := range l.Logs {
		current := &l.Logs[i]
		if accessAuditKey(*current) != key {
			continue
		}
		if isRawAccessProto(current.AccessProto) && !isRawAccessProto(incoming.AccessProto) {
			*current = incoming
			return
		}
		if !isRawAccessProto(current.AccessProto) && isRawAccessProto(incoming.AccessProto) {
			return
		}
		if current.AccessProto == incoming.AccessProto &&
			strings.EqualFold(current.Info, incoming.Info) {
			return
		}
	}
	l.Logs = append(l.Logs, incoming)
}

// 异步写入pool
func (a *AccessAudit) Add(userName, groupName string, pl *sessdata.Payload) {
	if a.Payload.Pool == nil {
		sessdata.Payloads.PutPayload(pl)
		return
	}
	select {
	case a.Payload.Pool.JobQueue <- func() {
		a.log(userName, groupName, pl)
	}:
	default:
		sessdata.Payloads.PutPayload(pl)
		base.Error("AccessAudit: AuditPayload channel is full")
	}
}

// 数据落盘并广播到WebSocket
func (l *LogBatch) Write() {
	if len(l.Logs) == 0 {
		return
	}
	err := dbdata.GetDB().InsertBatch(l.Logs)
	if err != nil {
		base.Error("AccessAudit: 批量写入失败:", err)
		l.Reset()
		return
	}
	l.Reset()
}

// 清空数据
func (l *LogBatch) Reset() {
	l.Logs = []dbdata.AccessAudit{}
}

// 开启批量写入数据功能
func logAuditBatch(ctx context.Context, audit *AccessAudit) {
	if base.GetCfg().AuditInterval < 0 {
		return
	}
	aa := audit
	aa.Start()

	// 启动定期清理过期审计去重条目和闲置 TCP 流（随 ctx 取消）
	base.SafeGo(func() { auditMapCleanupLoop(ctx, audit) })
	base.SafeGo(func() { tcpStreamCleanupLoop(ctx, audit) })

	var (
		limit   = 100 // 超过上限批量写入数据表
		outTime = time.NewTimer(time.Second)
		record  dbdata.AccessAudit
	)

	for {
		// 重置超时 时间
		outTime.Reset(time.Second * 1)
		select {
		case <-ctx.Done():
			// 退出前刷盘，避免丢失待写入审计
			aa.Batch.Write()
			return
		case record = <-aa.Batch.LogChan:
			aa.Batch.appendAudit(record)
			if len(aa.Batch.Logs) >= limit {
				if !outTime.Stop() {
					<-outTime.C
				}
				aa.Batch.Write()
			}
		case <-outTime.C:
			aa.Batch.Write()
		}
	}
}

func (a *AccessAudit) log(userName, groupName string, pl *sessdata.Payload) {
	defer func() {
		if err := recover(); err != nil {
			base.Error("logAudit is panic: ", err, "\n", string(debug.Stack()))
		}
	}()

	defer func() {
		sessdata.Payloads.PutPayload(pl)
	}()

	if !(pl.LType == sessdata.LTypeIPData && pl.PType == 0x00) {
		return
	}

	// 按 IP 版本提取五元组（v4/v6 统一审计口径）
	pkt, ok := parseAuditPacket(pl)
	if !ok {
		return
	}

	// 只统计 tcp 和 udp 的访问
	var accessProto uint8
	switch pkt.proto {
	case waterutil.TCP:
		accessProto = acc_proto_tcp
	case waterutil.UDP:
		accessProto = acc_proto_udp
	default:
		return
	}
	b := sessdata.Payloads.GetByte51()
	defer sessdata.Payloads.PutByte51(b)
	// key格式 16字节源IP地址 + 16字节目的IP地址 + 源/目的端口 + 1字节协议类型 + 16字节域名MD5
	key := *b
	copy(key[:16], pkt.src)
	copy(key[16:32], pkt.dst)
	binary.BigEndian.PutUint16(key[32:34], pkt.srcPort)
	binary.BigEndian.PutUint16(key[34:36], pkt.dstPort)
	key[36] = byte(accessProto)
	clear(key[37:53])

	now := utils.NowSec()
	nu := now.Unix()
	interval := int64(base.GetCfg().AuditInterval)

	// 识别访问协议与域名（TCP SNI/HTTP、UDP DNS、FakeDNS 兜底）
	accessProto, info := a.resolveDomain(userName, groupName, pkt, accessProto)

	// 任何命中域名的记录：把"仅 IP"的去重键提前占位，避免既记域名又记一笔裸 IP
	if info != "" && interval > 0 {
		ipKey := make([]byte, 53)
		copy(ipKey, key)
		// 占位键使用域名所依赖的底层传输协议：HTTPS/HTTP→TCP，DNS→UDP，FakeDNS 兜底保持当前协议
		placeholderProto := accessProto
		switch accessProto {
		case acc_proto_https, acc_proto_http:
			placeholderProto = acc_proto_tcp
		case acc_proto_dns:
			placeholderProto = acc_proto_udp
		}
		ipKey[36] = byte(placeholderProto)
		ipS := accessAuditUniqueKey(userName, groupName, ipKey)
		a.Payload.IpAuditMap.Store(ipS, nu)

		key[36] = byte(accessProto)
		// 存储含域名的key
		md5Sum := md5.Sum([]byte(info))
		copy(key[37:53], md5Sum[:])
	}
	s := accessAuditUniqueKey(userName, groupName, key)

	// audit_interval=0 表示不去重，也不维护去重 Map
	if interval > 0 {
		v, ok := a.Payload.IpAuditMap.Load(s)
		if ok && nu-v.(int64) < interval {
			return
		}
		a.Payload.IpAuditMap.Store(s, nu)
	}

	audit := dbdata.AccessAudit{
		Username:    userName,
		GroupName:   groupName,
		Protocol:    uint8(pkt.proto),
		Src:         pkt.src.String(),
		Dst:         pkt.dst.String(),
		SrcPort:     pkt.srcPort,
		DstPort:     pkt.dstPort,
		CreatedAt:   utils.NowSec(),
		AccessProto: accessProto,
		Info:        info,
	}
	select {
	case a.Batch.LogChan <- audit:
	default:
		base.Error("AccessAudit: LogChan channel is full")
		return
	}
}

// 定期清理闲置 TCP 流，避免依赖后续数据包触发回收
func tcpStreamCleanupLoop(ctx context.Context, audit *AccessAudit) {
	ticker := time.NewTicker(tcpStreamIdleExpiry / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			aa := audit
			if aa == nil || aa.Payload == nil || aa.Payload.TCPStreams == nil {
				return
			}
			aa.Payload.TCPStreams.cleanup(time.Now())
		}
	}
}

// 定期清理过期的审计去重条目，防止内存泄漏
func auditMapCleanupLoop(ctx context.Context, audit *AccessAudit) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			aa := audit
			if aa == nil || aa.Payload == nil {
				return
			}
			now := utils.NowSec().Unix()
			interval := int64(base.GetCfg().AuditInterval)
			if interval <= 0 {
				continue
			}
			expireBefore := now - interval*2 // 保留 2 倍间隔的余量
			aa.Payload.IpAuditMap.Range(func(k, v any) bool {
				if ts, ok := v.(int64); ok && ts < expireBefore {
					aa.Payload.IpAuditMap.Delete(k)
				}
				return true
			})
		}
	}
}
