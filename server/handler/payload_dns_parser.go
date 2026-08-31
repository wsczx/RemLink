package handler

import (
	"encoding/binary"
)

// parseDNSQuery 从一段 UDP 负载（DNS 报文）中提取第一个 Question 的域名。
// 仅在目的端口为 53 的 UDP 包上调用。返回 "" 表示解析失败或非查询报文。
func parseDNSQuery(udpPl []byte) string {
	// DNS 头固定 12 字节：id(2) flags(2) qdcount(2) ancount(2) nscount(2) arcount(2)
	if len(udpPl) < 12 {
		return ""
	}
	// QR=0 表示查询（flags 最高位）
	flags := binary.BigEndian.Uint16(udpPl[2:4])
	if flags&0x8000 != 0 {
		return "" // 响应报文，不记录
	}
	qdCount := binary.BigEndian.Uint16(udpPl[4:6])
	if qdCount == 0 {
		return ""
	}

	// 跳到 Question Section 起始
	off := 12
	// 解析 Name（可能含压缩指针；审计只取第一个标签序列，遇指针即停）
	name := ""
	for off < len(udpPl) {
		length := int(udpPl[off])
		if length == 0 {
			off++
			break
		}
		// 压缩指针（前两位 11）出现时不再继续拼接，避免误读
		if length&0xC0 == 0xC0 {
			off += 2
			break
		}
		if length&0xC0 != 0 {
			return ""
		}
		off++
		if off+length > len(udpPl) {
			return ""
		}
		if name != "" {
			name += "."
		}
		name += string(udpPl[off : off+length])
		off += length
	}
	if name == "" {
		return ""
	}
	// 跳过 QTYPE(2) + QCLASS(2)
	if off+4 > len(udpPl) {
		return ""
	}
	if !validDomainChar(name) {
		return ""
	}
	return name
}
