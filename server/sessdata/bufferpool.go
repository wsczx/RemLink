package sessdata

import (
	"sync"

	"github.com/wsczx/remlink/base"
)

const (
	// PayloadBufferSize 是复用缓冲与 Payload 载荷缓冲区的唯一尺寸来源
	// handler 的 CSTP/DTLS 帧上限、DTLS MTU、TAP 帧以及 GetByte 压缩临时缓冲都复用它，
	// 合并为单一常量可避免多处魔数静默失配（帧上限 > 缓冲容量会导致越界）
	PayloadBufferSize = 2048
	auditKeyBufSize   = 53
)

// 管理复用的字节缓冲与 Payload 对象
type BufferPool struct {
	plPool     sync.Pool
	bytePool   sync.Pool
	byte51Pool sync.Pool
}

func NewBufferPool() *BufferPool {
	return &BufferPool{
		plPool: sync.Pool{New: func() any {
			b := make([]byte, PayloadBufferSize)
			return &Payload{LType: LTypeIPData, PType: 0x00, Data: b}
		}},
		bytePool: sync.Pool{New: func() any {
			b := make([]byte, PayloadBufferSize)
			return &b
		}},
		byte51Pool: sync.Pool{New: func() any {
			b := make([]byte, auditKeyBufSize)
			return &b
		}},
	}
}

// 缓冲池实例，运行时由 App 注入
var Payloads *BufferPool

// 注入缓冲池实例（启动时一次）
func SetPayloads(p *BufferPool) { Payloads = p }
func (p *BufferPool) GetPayload() *Payload {
	return p.plPool.Get().(*Payload)
}

// 归还 Payload；错误容量直接丢弃，防越界复用
func (p *BufferPool) PutPayload(pl *Payload) {
	if pl == nil || cap(pl.Data) != PayloadBufferSize {
		base.Warn("payload cap is err", cap(pl.Data))
		return
	}
	pl.Data = pl.Data[:PayloadBufferSize]
	clear(pl.Data)
	pl.LType = LTypeIPData
	pl.PType = 0x00
	p.plPool.Put(pl)
}

func (p *BufferPool) GetByte() *[]byte {
	return p.bytePool.Get().(*[]byte)
}

func (p *BufferPool) PutByte(b *[]byte) {
	if b == nil {
		return
	}
	*b = (*b)[:PayloadBufferSize]
	p.bytePool.Put(b)
}

func (p *BufferPool) GetByte51() *[]byte {
	return p.byte51Pool.Get().(*[]byte)
}

func (p *BufferPool) PutByte51(b *[]byte) {
	if b == nil {
		return
	}
	*b = (*b)[:auditKeyBufSize]
	p.byte51Pool.Put(b)
}
