package handler

import (
	"sync"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/sessdata"
)

// 不允许直接修改
// [6] => PType
var plHeader = []byte{
	'S', 'T', 'F', 1,
	0x00, 0x00, /* Length */
	0x00, /* Type */
	0x00, /* Unknown */
}

var plPool = sync.Pool{
	New: func() any {
		b := make([]byte, BufferSize)
		pl := sessdata.Payload{
			LType: sessdata.LTypeIPData,
			PType: 0x00,
			Data:  b,
		}
		return &pl
	},
}

func getPayload() *sessdata.Payload {
	pl := plPool.Get().(*sessdata.Payload)
	return pl
}

func putPayload(pl *sessdata.Payload) {
	// 错误数据丢弃
	if cap(pl.Data) != BufferSize {
		base.Warn("payload cap is err", cap(pl.Data))
		return
	}

	pl.Data = pl.Data[:BufferSize]
	// 清零整个缓冲区：防止复用缓冲中残留的其它会话数据被越界长度字段读出
	clear(pl.Data)
	pl.LType = sessdata.LTypeIPData
	pl.PType = 0x00
	plPool.Put(pl)
}

var bytePool = sync.Pool{
	New: func() any {
		b := make([]byte, BufferSize)
		return &b
	},
}

func getByteFull() *[]byte {
	b := bytePool.Get().(*[]byte)
	return b
}
func putByte(b *[]byte) {
	*b = (*b)[:BufferSize]
	bytePool.Put(b)
}

type BufferPool struct {
	sync.Pool
}

// 审计去重键小对象
var byte51Pool = sync.Pool{
	New: func() any {
		b := make([]byte, 53)
		return &b
	},
}

func getByte51() *[]byte {
	b := byte51Pool.Get().(*[]byte)
	return b
}

func putByte51(b *[]byte) {
	*b = (*b)[:53]
	byte51Pool.Put(b)
}
