package handler

import (
	"encoding/binary"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/sessdata"
)

var plHeader = []byte{
	'S', 'T', 'F', 1,
	0x00, 0x00, /* Length */
	0x00, /* Type */
	0x00, /* Unknown */
}

// CSTP 头 8 字节、[4:6] 存载荷长度；DTLS 头 1 字节、无长度字段
type PayloadFormat struct {
	headerLen      int  // 头部字节数
	hasLengthField bool // 头部是否携带载荷长度
}

var (
	cstpFormat = &PayloadFormat{headerLen: 8, hasLengthField: true}
	dtlsFormat = &PayloadFormat{headerLen: 1, hasLengthField: false}
)

// 解压 COMPRESSED-DATA(0x08)，ok=false 表示失败或越界
func (f *PayloadFormat) Decompress(cmp sessdata.CmpEncoding, pl *sessdata.Payload, n int) (int, bool) {
	if cmp == nil || n < f.headerLen+1 {
		return 0, false
	}
	dst := sessdata.Payloads.GetByte()
	nn, err := cmp.Uncompress(pl.Data[f.headerLen:n], *dst)
	if err != nil || nn <= 0 || nn+f.headerLen > sessdata.PayloadBufferSize {
		sessdata.Payloads.PutByte(dst)
		return 0, false
	}
	if f.hasLengthField {
		// 长度写回头部，读侧据此截取
		binary.BigEndian.PutUint16(pl.Data[4:6], uint16(nn))
	}
	pl.Data = append(pl.Data[:f.headerLen], (*dst)[:nn]...)
	sessdata.Payloads.PutByte(dst)
	return nn + f.headerLen, true
}

// 提取 DATA 载荷（去头），ok=false 表示长度非法
func (f *PayloadFormat) Decode(pl *sessdata.Payload, n int) ([]byte, bool) {
	if f.hasLengthField {
		dataLen := binary.BigEndian.Uint16(pl.Data[4:6])
		if dataLen == 0 || f.headerLen+int(dataLen) > sessdata.PayloadBufferSize {
			return nil, false
		}
		if f.headerLen+int(dataLen) > n {
			// 复用缓冲区中其它会话的残留数据会泄入载荷
			return nil, false
		}
		return pl.Data[f.headerLen : f.headerLen+int(dataLen)], true
	}
	if n < f.headerLen+1 {
		return nil, false
	}
	return pl.Data[f.headerLen:n], true
}

// 编码出向 pl，discard=true 表示缓冲越界需丢弃
func (f *PayloadFormat) Encode(cmp sessdata.CmpEncoding, pl *sessdata.Payload) (discard bool) {
	if pl.PType != 0x00 {
		// 控制报文：仅把类型字节作为头部
		if f.headerLen == 1 {
			pl.Data = append(pl.Data[:0], pl.PType)
		} else {
			pl.Data = append(pl.Data[:0], plHeader...)
			pl.Data[6] = pl.PType
		}
		return false
	}

	if cmp != nil && len(pl.Data) > base.GetCfg().NoCompressLimit {
		dst := sessdata.Payloads.GetByte()
		size, err := cmp.Compress(pl.Data, (*dst)[f.headerLen:])
		if err == nil && size < len(pl.Data) {
			if f.hasLengthField {
				copy((*dst)[:f.headerLen], plHeader)
				binary.BigEndian.PutUint16((*dst)[4:6], uint16(size))
				(*dst)[6] = 0x08
			} else {
				(*dst)[0] = 0x08
			}
			pl.Data = append(pl.Data[:0], (*dst)[:size+f.headerLen]...)
			sessdata.Payloads.PutByte(dst)
			return false
		}
		sessdata.Payloads.PutByte(dst)
	}

	// 容量不足则丢弃，避免 reslice 越界 panic
	if len(pl.Data)+f.headerLen > cap(pl.Data) {
		return true
	}
	if f.headerLen == 1 {
		pl.Data = pl.Data[:len(pl.Data)+1]
		copy(pl.Data[1:], pl.Data[:len(pl.Data)-1])
		pl.Data[0] = pl.PType
	} else {
		pl.Data = pl.Data[:len(pl.Data)+8]
		copy(pl.Data[8:], pl.Data[:len(pl.Data)-8])
		copy(pl.Data[:8], plHeader)
		binary.BigEndian.PutUint16(pl.Data[4:6], uint16(len(pl.Data)-8))
	}
	return false
}
