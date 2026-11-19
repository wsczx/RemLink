package handler

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/wsczx/remlink/sessdata"
)

func TestCSTPFormatDecode(t *testing.T) {
	tests := []struct {
		name    string
		dataLen uint16
		n       int
		want    bool
	}{
		{"正常完整帧", 100, 108, true},
		{"正常完整帧-最大", 2040, 2048, true},
		{"dataLen为0", 0, 8, false},
		{"超过实际上读长度-截断攻击", 2000, 18, false},
		{"超过实际上读长度-差1字节", 100, 107, false},
		{"刚好等于实际读取", 100, 108, true},
		{"超过缓冲区上限", 2041, 2049, false},
		{"uint16回绕-65528", 65528, 70000, false},
		{"uint16回绕-65535", 65535, 70000, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pl := sessdata.Payloads.GetPayload()
			defer sessdata.Payloads.PutPayload(pl)
			binary.BigEndian.PutUint16(pl.Data[4:6], tt.dataLen)
			if _, ok := cstpFormat.Decode(pl, tt.n); ok != tt.want {
				t.Errorf("cstpFormat.Decode(dataLen=%d, n=%d) ok=%v, want %v", tt.dataLen, tt.n, ok, tt.want)
			}
		})
	}
}

// 复用缓冲区必须清零，防跨会话残留数据泄出（H-1）
func TestPutPayloadZeroesBuffer(t *testing.T) {
	pl := sessdata.Payloads.GetPayload()
	for i := range pl.Data {
		pl.Data[i] = 0xAA
	}
	pl.Data = pl.Data[:100]
	sessdata.Payloads.PutPayload(pl)

	pl2 := sessdata.Payloads.GetPayload()
	defer sessdata.Payloads.PutPayload(pl2)
	if !bytes.Equal(pl2.Data, make([]byte, sessdata.PayloadBufferSize)) {
		t.Error("putPayload 后复用缓冲区未被清零，残留数据可能跨会话泄露")
	}
}
