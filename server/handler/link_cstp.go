package handler

import (
	"bufio"
	"net"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

func LinkCSTP(conn net.Conn, bufRW *bufio.ReadWriter, cSess *sessdata.ConnSession) {
	base.Debug("LinkCSTP connect ip:", cSess.IpAddr, "user:", dbdata.Users.Label(cSess.Username, cSess.Nickname), "rip:", conn.RemoteAddr())
	defer func() {
		// 解析畸形报文等异常不应带崩整个进程
		if err := recover(); err != nil {
			base.Error("LinkCSTP panic", cSess.Username, err)
		}
		base.Debug("LinkCSTP return", cSess.Username, cSess.IpAddr)
		// 兜底原因码：读超时/读错误等链路异常。若上面已设置过更具体的原因
		// （如客户端 DISCONNECT），SetLogoutCode 不会覆盖
		cSess.SetLogoutCode(dbdata.UserLogoutLink)
		_ = conn.Close()
		cSess.Close()
	}()

	dead := time.Second * time.Duration(cSess.CstpDpd+5)
	readFn := func(pl *sessdata.Payload) (int, error) {
		// 设置超时限制，使空闲/断链能在 DPD 周期内被探知
		if err := conn.SetReadDeadline(utils.NowSec().Add(dead)); err != nil {
			return 0, err
		}
		n, err := bufRW.Read(pl.Data)
		if err != nil {
			return n, err
		}
		// 读侧上行限速；TUN 读侧不限速
		if err = cSess.RateLimit(n, true); err != nil {
			base.Error(err)
		}
		return n, nil
	}
	decoder := &cstpDecoder{sess: cSess, conn: conn}
	writeFn := func(pl *sessdata.Payload) (int, error) {
		if pl.LType != sessdata.LTypeIPData {
			return 0, nil
		}
		if cstpFormat.Encode(cSess.CstpPickCmp, pl) {
			// 缓冲越界，丢弃（不写）
			base.Warn("cstpWrite 载荷超出缓冲区容量，丢弃:", cSess.Username, cSess.IpAddr)
			return 0, nil
		}
		n, err := conn.Write(pl.Data)
		if err == nil && n > 0 {
			_ = cSess.RateLimit(n, false)
		}
		return n, err
	}

	go runSessionReadLoop(cSess, readFn, decoder.decode, true)
	go runSessionWriteLoop(cSess, cSess.PayloadOutCstp, cSess.CloseChan, writeFn, sessdata.Payloads.PutPayload, true)
	go runSessionIdleCheck(cSess, cSess.CloseChan)

	// 直到会话关闭才退出并触发 defer 清理
	<-cSess.CloseChan
}

type cstpDecoder struct {
	sess *sessdata.ConnSession
	conn net.Conn
}

func (d *cstpDecoder) decode(pl *sessdata.Payload, n int) bool {
	sess, conn := d.sess, d.conn
	// CSTP 是 TCP 流，bufRW.Read 可能在帧边界返回不足一个完整帧的分片（n < 帧头）
	// 短包直接跳过，避免下方 pl.Data[6] 越界 panic 触发连接断开重连
	if n < 7 {
		sessdata.Payloads.PutPayload(pl)
		return false
	}
	switch pl.Data[6] {
	case 0x07: // KEEPALIVE
		base.Trace("recv LinkCSTP Keepalive", sess.Username, sess.IpAddr, conn.RemoteAddr())
		if idle := int64(base.GetCfg().IdleTimeout); idle > 0 {
			if isSessionIdle(sess, idle) {
				base.Warn("IdleTimeout", sess.Username, sess.IpAddr, conn.RemoteAddr(), "lastTime", sess.LastDataTime.Load())
				sess.CloseSess(dbdata.UserIdleTimeout)
				return true
			}
		}
	case 0x05: // DISCONNECT
		sess.SetLogoutCode(dbdata.UserLogoutClient)
		base.Debug("DISCONNECT", dbdata.Users.Label(sess.Username, sess.Nickname), sess.IpAddr, conn.RemoteAddr())
		sess.CloseSess(dbdata.UserLogoutClient)
		return true
	case 0x03: // DPD-REQ
		base.Trace("recv LinkCSTP DPD-REQ", sess.Username, sess.IpAddr, conn.RemoteAddr(), n, pl.Data[:n])
		pl.PType = 0x04
		// 控制报文须在原通道应答：CSTP 收到的 DPD-REQ 必须走 CSTP 回 DPD-RESP，
		// 不能经 PayloadOut 路由到 DTLS（握手期 DTLS 会话已存在但链路未通，客户端收不到）
		if sess.CstpPayloadOut(pl) {
			return true
		}
	case 0x04:
		base.Trace("recv LinkCSTP DPD-RESP", sess.Username, sess.IpAddr, conn.RemoteAddr())
	case 0x08: // COMPRESSED DATA：解压后按 DATA 处理
		var ok bool
		if n, ok = cstpFormat.Decompress(sess.CstpPickCmp, pl, n); !ok {
			return false
		}
		return d.handleData(pl, n)
	case 0x00: // DATA
		return d.handleData(pl, n)
	}
	return false
}

func (d *cstpDecoder) handleData(pl *sessdata.Payload, n int) bool {
	data, ok := cstpFormat.Decode(pl, n)
	if !ok {
		base.Error("recv error dataLen", d.sess.Username, n)
		return false
	}
	pl.Data = append(pl.Data[:0], data...)
	if d.sess.PayloadInbound(pl) {
		return true
	}
	d.sess.LastDataTime.Store(utils.NowSec().Unix())
	return false
}
