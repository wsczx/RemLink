package handler

import (
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

// CSTP/DTLS/TUN/TAP 共用，协议差异只在 readFn/decodeFn；读侧限速由 readFn 自行处理
// closeOnError 为 true 时读错误会关闭整个会话，DTLS 辅助通道传 false
func runSessionReadLoop(cSess *sessdata.ConnSession, readFn func(pl *sessdata.Payload) (int, error), decodeFn func(pl *sessdata.Payload, n int) bool, closeOnError bool) {
	for {
		pl := sessdata.Payloads.GetPayload()
		n, err := readFn(pl)
		if err != nil {
			base.Warn("read frame err:", cSess.Username, cSess.IpAddr, err)
			sessdata.Payloads.PutPayload(pl)
			if closeOnError {
				cSess.Close()
			}
			return
		}
		if n < 1 {
			sessdata.Payloads.PutPayload(pl)
			continue
		}
		if decodeFn(pl, n) {
			sessdata.Payloads.PutPayload(pl)
			return
		}
	}
}

// select 出向通道/done → writeFn 写出 → 回收
// 下行限速由各网络 writeFn 自己按实际写出字节处理，TUN/TAP 写网卡不计
// done 由调用方给出，DTLS 用 dSess.CloseChan，其余用 cSess.CloseChan
func runSessionWriteLoop(cSess *sessdata.ConnSession, outCh <-chan *sessdata.Payload, done <-chan struct{}, writeFn func(pl *sessdata.Payload) (int, error), recycle func(pl *sessdata.Payload), closeOnError bool) {
	for {
		select {
		case pl := <-outCh:
			if pl == nil {
				return
			}
			_, err := writeFn(pl)
			recycle(pl)
			if err != nil {
				if closeOnError {
					cSess.Close()
				}
				return
			}
		case <-done:
			return
		}
	}
}

// 空闲判定：最后的数据传输时间早于 idle 秒
func isSessionIdle(cSess *sessdata.ConnSession, idle int64) bool {
	return cSess.LastDataTime.Load() < (utils.NowSec().Unix() - idle)
}

// 超时则关闭会话；done 为当前数据通道的关闭信号
func runSessionIdleCheck(cSess *sessdata.ConnSession, done <-chan struct{}) {
	idle := int64(base.GetCfg().IdleTimeout)
	if idle <= 0 {
		return
	}
	ticker := time.NewTicker(time.Duration(idle) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if isSessionIdle(cSess, idle) {
				base.Warn("IdleTimeout", cSess.Username, cSess.IpAddr)
				cSess.CloseSess(dbdata.UserIdleTimeout)
				return
			}
		}
	}
}
