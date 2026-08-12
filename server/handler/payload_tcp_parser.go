package handler

import (
	"bufio"
	"bytes"
	"net/http"
	"regexp"
	"strings"
)

var tcpParsers = []func([]byte) (uint8, string){
	sniNewParser,
	httpParser,
}

// onTCP 接收一段完整的 TCP 报文（含 TCP 头，不含 IP 头），跳过 TCP 头后
// 用 SNI/HTTP parser 提取域名。tcpPl 长度必须 >= 14（TCP 头最小长度）。
func onTCP(tcpPl []byte) (uint8, string) {
	if len(tcpPl) < 14 {
		return acc_proto_tcp, ""
	}
	// TCP 头长度（Data Offset 字段，单位为 4 字节）
	dataOff := int(tcpPl[12]>>4) << 2
	if dataOff < 20 || dataOff > len(tcpPl) {
		return acc_proto_tcp, ""
	}
	data := tcpPl[dataOff:]
	for _, parser := range tcpParsers {
		if proto, info := parser(data); proto != acc_proto_tcp {
			return proto, info
		}
	}
	return acc_proto_tcp, ""
}

// sniNewParser 从 TLS ClientHello 中提取 SNI 域名。
// 基于游标遍历：先定位 Handshake 主体，再跳过固定字段，最后按扩展声明长度遍历扩展链。
func sniNewParser(b []byte) (uint8, string) {
	if len(b) < 5 || b[0] != 0x16 || b[1] != 0x03 {
		return acc_proto_tcp, ""
	}
	// TLS 记录层：record type(1) + version(2) + length(2)，之后为握手消息
	if len(b) < 9 {
		return acc_proto_tcp, ""
	}
	hsType := b[5]
	if hsType != 0x01 { // ClientHello
		return acc_proto_tcp, ""
	}
	hsLen := int(b[6])<<16 | int(b[7])<<8 | int(b[8])
	body := b[9:]
	if hsLen < len(body) {
		body = body[:hsLen]
	}
	// 跳过 version(2) + random(32)
	off := 2 + 32
	if off+1 > len(body) {
		return acc_proto_https, ""
	}
	sidLen := int(body[off])
	off += 1 + sidLen
	if off+2 > len(body) {
		return acc_proto_https, ""
	}
	csl := int(body[off])<<8 | int(body[off+1])
	off += 2 + csl
	if off+1 > len(body) {
		return acc_proto_https, ""
	}
	cml := int(body[off])
	off += 1 + cml
	if off+2 > len(body) {
		return acc_proto_https, ""
	}
	extTotal := int(body[off])<<8 | int(body[off+1])
	off += 2
	extEnd := min(off+extTotal, len(body))
	for off+4 <= extEnd {
		etype := int(body[off])<<8 | int(body[off+1])
		off += 2
		elen := int(body[off])<<8 | int(body[off+1])
		off += 2
		if etype == 0 { // server_name
			listEnd := min(off+elen, extEnd)
			if off+2 > listEnd {
				return acc_proto_https, ""
			}
			listLen := int(body[off])<<8 | int(body[off+1])
			off += 2
			nameEnd := min(off+listLen, listEnd)
			for off+3 <= nameEnd {
				nt := body[off]
				off++
				nl := int(body[off])<<8 | int(body[off+1])
				off += 2
				if nt == 0 { // host_name
					if off+nl > nameEnd {
						return acc_proto_https, ""
					}
					host := string(body[off : off+nl])
					if validDomainChar(host) {
						return acc_proto_https, host
					}
					return acc_proto_https, ""
				}
				off += nl
			}
		} else {
			off += elen
		}
	}
	return acc_proto_https, ""
}

// Beta
func httpNewParser(data []byte) (uint8, string) {
	methodArr := []string{"OPTIONS", "HEAD", "GET", "POST", "PUT", "DELETE", "TRACE", "CONNECT"}
	before, _, ok := bytes.Cut(data, []byte{10})
	if !ok {
		return acc_proto_tcp, ""
	}
	method, uri, _ := strings.Cut(string(before), " ")
	ok = false
	for _, v := range methodArr {
		if v == method {
			ok = true
		}
	}
	if !ok {
		return acc_proto_tcp, ""
	}
	hostname := ""
	// GET http://www.google.com/index.html HTTP/1.1
	if len(uri) > 7 && uri[:4] == "http" {
		uriSlice := strings.Split(uri[7:], "/")
		hostname = uriSlice[0]
		return acc_proto_http, hostname
	}
	packet := string(data)
	hostPos := strings.Index(packet, "Host: ")
	if hostPos == -1 {
		hostPos = strings.Index(packet, "HOST: ")
		if hostPos == -1 {
			return acc_proto_tcp, ""
		}
	}
	hostEndPos := strings.Index(packet[hostPos:], "\n")
	if hostEndPos == -1 {
		return acc_proto_tcp, ""
	}
	hostname = packet[hostPos+6 : hostPos+hostEndPos-1]
	return acc_proto_http, hostname
}

func sniParser(data []byte) (uint8, string) {
	if len(data) < 2 || data[0] != 0x16 || data[1] != 0x03 {
		return acc_proto_tcp, ""
	}
	sniRe := regexp.MustCompile("\x00\x00.{4}\x00.{2}([a-z0-9]+([\\-\\.]{1}[a-z0-9]+)*\\.[a-z]{2,6})\x00")
	m := sniRe.FindSubmatch(data)
	if len(m) < 2 {
		return acc_proto_tcp, ""
	}
	host := string(m[1])
	return acc_proto_https, host
}

func httpParser(data []byte) (uint8, string) {
	if req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data))); err == nil {
		return acc_proto_http, req.Host
	}
	return acc_proto_tcp, ""
}

// 校验域名的合法字符, 处理乱码问题
func validDomainChar(addr string) bool {
	// Allow a-z A-Z . - 0-9
	for i := 0; i < len(addr); i++ {
		c := addr[i]
		if !((c >= 97 && c <= 122) || (c >= 65 && c <= 90) || (c >= 45 && c <= 46) || (c >= 48 && c <= 57)) {
			return false
		}
	}
	return true
}
