package utils

import (
	"unsafe"
)

// 零拷贝地把字节切片转成字符串
func BytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}
