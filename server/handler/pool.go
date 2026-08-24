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

	pl.LType = sessdata.LTypeIPData
	pl.PType = 0x00
	pl.Data = pl.Data[:BufferSize]
	plPool.Put(pl)
}

var bytePool = sync.Pool{
	New: func() any {
		b := make([]byte, BufferSize)
		return &b
	},
}

// func getByteZero() *[]byte {
// 	b := bytePool.Get().(*[]byte)
// 	*b = (*b)[:0]
// 	return b
// }

func getByteFull() *[]byte {
	b := bytePool.Get().(*[]byte)
	return b
}
func putByte(b *[]byte) {
	*b = (*b)[:BufferSize]
	bytePool.Put(b)
}

// 长度 34 小对象
// var byte34Pool = sync.Pool{
// 	New: func() interface{} {
// 		b := make([]byte, 34)
// 		return &b
// 	},
// }

// func getByte34() *[]byte {
// 	b := byte34Pool.Get().(*[]byte)
// 	return b
// }

// func putByte34(b *[]byte) {
// 	*b = (*b)[:34]
// 	byte34Pool.Put(b)
// }

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
