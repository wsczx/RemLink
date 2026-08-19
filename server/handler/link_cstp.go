package handler

import (
	"bufio"
	"encoding/binary"
	"net"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

func LinkCstp(conn net.Conn, bufRW *bufio.ReadWriter, cSess *sessdata.ConnSession) {
	base.Debug("LinkCstp connect ip:", cSess.IpAddr, "user:", dbdata.UserLabel(cSess.Username, cSess.Nickname), "rip:", conn.RemoteAddr())
	defer func() {
		// 解析畸形报文等异常不应带崩整个进程
		if err := recover(); err != nil {
			base.Error("LinkCstp panic", cSess.Username, err)
		}
		base.Debug("LinkCstp return", cSess.Username, cSess.IpAddr)
		// 兜底原因码：读超时/读错误等链路异常。若上面已设置过更具体的原因
		// （如客户端 DISCONNECT），SetLogoutCode 不会覆盖。
		cSess.SetLogoutCode(dbdata.UserLogoutLink)
		_ = conn.Close()
		cSess.Close()
	}()

	var (
		err       error
		n         int
		dataLen   uint16
		dead      = time.Second * time.Duration(cSess.CstpDpd+5)
		idle      = int64(base.GetCfg().IdleTimeout)
		checkIdle = base.GetCfg().IdleTimeout > 0
		lastTime  int64
	)

	go cstpWrite(conn, bufRW, cSess)

	for {

		// 设置超时限制
		err = conn.SetReadDeadline(utils.NowSec().Add(dead))
		if err != nil {
			base.Error("SetDeadline: ", cSess.Username, cSess.IpAddr, err)
			return
		}
		// hdata := make([]byte, BufferSize)
		pl := getPayload()
		n, err = bufRW.Read(pl.Data)
		if err != nil {
			base.Warn("read hdata: ", cSess.Username, cSess.IpAddr, err)
			return
		}

		// CSTP 头部至少需要 7 字节（1 byte type + 6 bytes header）
		if n < 7 {
			base.Warn("recv short cstp data:", cSess.Username, cSess.IpAddr, n)
			continue
		}

		// 限流设置
		err = cSess.RateLimit(n, true)
		if err != nil {
			base.Error(err)
		}

		switch pl.Data[6] {
		case 0x07: // KEEPALIVE
			// do nothing
			base.Trace("recv LinkCstp Keepalive", cSess.Username, cSess.IpAddr, conn.RemoteAddr())
			// 判断超时时间
			if checkIdle {
				lastTime = cSess.LastDataTime.Load()
				if lastTime < (utils.NowSec().Unix() - idle) {
					base.Warn("IdleTimeout", cSess.Username, cSess.IpAddr, conn.RemoteAddr(), "lastTime", lastTime)
					sessdata.CloseSess(cSess.Sess.Token, dbdata.UserIdleTimeout)
					return
				}
			}
		case 0x05: // DISCONNECT
			cSess.SetLogoutCode(dbdata.UserLogoutClient)
			base.Debug("DISCONNECT", dbdata.UserLabel(cSess.Username, cSess.Nickname), cSess.IpAddr, conn.RemoteAddr())
			sessdata.CloseSess(cSess.Sess.Token, dbdata.UserLogoutClient)
			return
		case 0x03: // DPD-REQ
			base.Trace("recv LinkCstp DPD-REQ", cSess.Username, cSess.IpAddr, conn.RemoteAddr(), n, pl.Data[:n])
			pl.PType = 0x04
			// pl.Data = pl.Data[:n]
			if payloadOutCstp(cSess, pl) {
				return
			}
		case 0x04:
			base.Trace("recv LinkCstp DPD-RESP", cSess.Username, cSess.IpAddr, conn.RemoteAddr())
		case 0x08: // decompress
			if cSess.CstpPickCmp == nil || n < 9 {
				continue
			}
			dst := getByteFull()
			// 只解压实际读到的数据，避免把缓冲区残留数据一起解压
			nn, err := cSess.CstpPickCmp.Uncompress(pl.Data[8:n], *dst)
			if err != nil {
				putByte(dst)
				base.Error("cstp decompress error", err, nn)
				continue
			}
			if nn <= 0 || 8+nn > BufferSize {
				putByte(dst)
				base.Error("cstp decompress invalid len:", cSess.Username, nn)
				continue
			}
			binary.BigEndian.PutUint16(pl.Data[4:6], uint16(nn))
			pl.Data = append(pl.Data[:8], (*dst)[:nn]...)
			putByte(dst)
			fallthrough
		case 0x00: // DATA
			// 获取数据长度
			dataLen = binary.BigEndian.Uint16(pl.Data[4:6]) // 4,5
			// dataLen==0 会使 pl.Data 变为空切片，下游 payloadIn 读 pl.Data[0] 越界崩溃；
			// 比较须转 int：uint16 下 8+dataLen 在 dataLen≥65528 时回绕成小值，绕过上限检查
			if dataLen == 0 || 8+int(dataLen) > BufferSize {
				base.Error("recv error dataLen", cSess.Username, dataLen)
				continue
			}
			// 去除数据头
			copy(pl.Data, pl.Data[8:8+dataLen])
			// 更新切片长度
			pl.Data = pl.Data[:dataLen]
			// pl.Data = append(pl.Data[:0], pl.Data[8:8+dataLen]...)
			if payloadIn(cSess, pl) {
				return
			}
			// 只记录返回正确的数据时间
			cSess.LastDataTime.Store(utils.NowSec().Unix())
		}
	}
}

func cstpWrite(conn net.Conn, _ *bufio.ReadWriter, cSess *sessdata.ConnSession) {
	defer func() {
		base.Debug("cstpWrite return", cSess.Username, cSess.IpAddr)
		_ = conn.Close()
		cSess.Close()
	}()

	var (
		err error
		n   int
		pl  *sessdata.Payload
	)

	for {
		select {
		case pl = <-cSess.PayloadOutCstp:
		case <-cSess.CloseChan:
			return
		}

		if pl.LType != sessdata.LTypeIPData {
			continue
		}

		if pl.PType == 0x00 {
			isCompress := false
			if cSess.CstpPickCmp != nil && len(pl.Data) > base.GetCfg().NoCompressLimit {
				dst := getByteFull()
				size, err := cSess.CstpPickCmp.Compress(pl.Data, (*dst)[8:])
				if err == nil && size < len(pl.Data) {
					copy((*dst)[:8], plHeader)
					binary.BigEndian.PutUint16((*dst)[4:6], uint16(size))
					(*dst)[6] = 0x08
					pl.Data = append(pl.Data[:0], (*dst)[:size+8]...)
					isCompress = true
				}
				putByte(dst)
			}
			if !isCompress {
				// 获取数据长度
				l := len(pl.Data)
				// 先扩容 +8
				pl.Data = pl.Data[:l+8]
				// 数据后移
				copy(pl.Data[8:], pl.Data)
				// 添加头信息
				copy(pl.Data[:8], plHeader)
				// 更新头长度
				binary.BigEndian.PutUint16(pl.Data[4:6], uint16(l))
			}
		} else {
			pl.Data = append(pl.Data[:0], plHeader...)
			// 设置头类型
			pl.Data[6] = pl.PType
		}

		n, err = conn.Write(pl.Data)
		if err != nil {
			base.Warn("write err", cSess.Username, cSess.IpAddr, err)
			return
		}

		putPayload(pl)

		// 限流设置
		err = cSess.RateLimit(n, false)
		if err != nil {
			base.Error(err)
		}
	}
}
