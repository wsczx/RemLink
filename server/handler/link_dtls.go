package handler

import (
	"net"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

func LinkDTLS(conn net.Conn, cSess *sessdata.ConnSession) {
	base.Debug("LinkDTLS connect ip:", cSess.IpAddr, "user:", cSess.Username, "udp-rip:", conn.RemoteAddr())
	dSess := cSess.NewDtlsConn()
	if dSess == nil {
		// 创建失败，直接关闭链接
		_ = conn.Close()
		return
	}

	defer func() {
		// 解析畸形报文等异常不应带崩整个进程
		if err := recover(); err != nil {
			base.Error("LinkDTLS panic", cSess.Username, err)
		}
		base.Debug("LinkDTLS return", cSess.Username, cSess.IpAddr)
		_ = conn.Close()
		dSess.Close()
	}()

	dead := time.Second * time.Duration(cSess.CstpDpd+5)
	readFn := func(pl *sessdata.Payload) (int, error) {
		if err := conn.SetReadDeadline(utils.NowSec().Add(dead)); err != nil {
			dSess.Close()
			return 0, err
		}
		n, err := conn.Read(pl.Data)
		if err != nil {
			dSess.Close()
			return n, err
		}
		// 读侧上行限速
		if err = cSess.RateLimit(n, true); err != nil {
			base.Error(err)
		}
		return n, nil
	}
	decoder := &dtlsDecoder{sess: cSess, conn: conn, dSess: dSess}
	writeFn := func(pl *sessdata.Payload) (int, error) {
		if pl.LType != sessdata.LTypeIPData {
			return 0, nil
		}
		// 数据报文(0x00/0x08)由 DTLS 承载；DPD-RESP(0x04) 等控制报文也走 DTLS，
		// Frame 对控制类型仅写 1 字节类型头，客户端据此判活（CSTP 收到的 DPD 已改走 CSTP 应答）
		if dtlsFormat.Encode(cSess.DtlsPickCmp, pl) {
			// 缓冲越界，丢弃（不写）
			base.Warn("dtlsWrite 载荷超出缓冲区容量，丢弃:", cSess.Username, cSess.IpAddr)
			return 0, nil
		}
		n, err := conn.Write(pl.Data)
		if err != nil {
			// 写失败需关闭本通道，否则 LinkDTLS 永久等待 CloseChan
			dSess.Close()
			return n, err
		}
		if n > 0 {
			_ = cSess.RateLimit(n, false)
		}
		return n, nil
	}

	go runSessionReadLoop(cSess, readFn, decoder.decode, false)
	go runSessionWriteLoop(cSess, cSess.PayloadOutDtls, dSess.CloseChan, writeFn, sessdata.Payloads.PutPayload, false)
	go runSessionIdleCheck(cSess, dSess.CloseChan)

	// DTLS 通道关闭即返回并触发 defer 清理；主会话不受影响
	<-dSess.CloseChan
}

type dtlsDecoder struct {
	sess  *sessdata.ConnSession
	conn  net.Conn
	dSess *sessdata.DtlsSession
}

func (d *dtlsDecoder) decode(pl *sessdata.Payload, n int) bool {
	sess, conn := d.sess, d.conn

	switch pl.Data[0] {
	case 0x07: // KEEPALIVE
		base.Trace("recv LinkDTLS Keepalive", sess.Username, sess.IpAddr, conn.RemoteAddr())
	case 0x05: // DISCONNECT
		sess.SetLogoutCode(dbdata.UserLogoutClient)
		base.Info("DISCONNECT DTLS", dbdata.Users.Label(sess.Username, sess.Nickname), sess.IpAddr, conn.RemoteAddr())
		d.dSess.Close()
		return true
	case 0x03: // DPD-REQ
		base.Trace("recv LinkDTLS DPD-REQ", sess.Username, sess.IpAddr, conn.RemoteAddr(), n)
		pl.PType = 0x04
		pl.Data = pl.Data[:n]
		if sess.PayloadOut(pl) {
			return true
		}
	case 0x04:
		base.Trace("recv LinkDTLS DPD-RESP", sess.Username, sess.IpAddr, conn.RemoteAddr())
	case 0x08: // COMPRESSED DATA：解压后按 DATA 处理
		var ok bool
		if n, ok = dtlsFormat.Decompress(sess.DtlsPickCmp, pl, n); !ok {
			return false
		}
		return d.handleData(pl, n)
	case 0x00: // DATA
		return d.handleData(pl, n)
	}
	return false
}

func (d *dtlsDecoder) handleData(pl *sessdata.Payload, n int) bool {
	data, ok := dtlsFormat.Decode(pl, n)
	if !ok {
		return false
	}
	pl.Data = append(pl.Data[:0], data...)
	if d.sess.PayloadInbound(pl) {
		return true
	}
	d.sess.LastDataTime.Store(utils.NowSec().Unix())
	return false
}
