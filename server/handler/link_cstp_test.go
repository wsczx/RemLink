package handler

import (
	"bytes"
	"testing"
)

func TestValidCstpDataLen(t *testing.T) {
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
			if got := validCstpDataLen(tt.dataLen, tt.n); got != tt.want {
				t.Errorf("validCstpDataLen(%d, %d) = %v, want %v", tt.dataLen, tt.n, got, tt.want)
			}
		})
	}
}

// 复用缓冲区必须清零，防跨会话残留数据泄出（H-1）
func TestPutPayloadZeroesBuffer(t *testing.T) {
	pl := getPayload()
	for i := range pl.Data {
		pl.Data[i] = 0xAA
	}
	pl.Data = pl.Data[:100]
	putPayload(pl)

	pl2 := getPayload()
	defer putPayload(pl2)
	if !bytes.Equal(pl2.Data, make([]byte, BufferSize)) {
		t.Error("putPayload 后复用缓冲区未被清零，残留数据可能跨会话泄露")
	}
}
