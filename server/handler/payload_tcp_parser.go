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
	commonProtocolParser,
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
	// TLS 记录层：record type(1) + version(2) + length(2)，之后为握手消息。
	recordLen := int(b[3])<<8 | int(b[4])
	if recordLen < 4 || len(b) < 5+recordLen || len(b) < 9 {
		return acc_proto_tcp, ""
	}
	hsType := b[5]
	if hsType != 0x01 { // ClientHello
		return acc_proto_tcp, ""
	}
	hsLen := int(b[6])<<16 | int(b[7])<<8 | int(b[8])
	if hsLen > recordLen-4 || hsLen > len(b)-9 {
		return acc_proto_tcp, ""
	}
	body := b[9 : 9+hsLen]
	// 跳过 version(2) + random(32)
	off := 2 + 32
	if off+1 > len(body) {
		return acc_proto_tcp, ""
	}
	sidLen := int(body[off])
	off++
	if off+sidLen > len(body) {
		return acc_proto_tcp, ""
	}
	off += sidLen
	if off+2 > len(body) {
		return acc_proto_tcp, ""
	}
	csl := int(body[off])<<8 | int(body[off+1])
	off += 2
	if off+csl > len(body) {
		return acc_proto_tcp, ""
	}
	off += csl
	if off+1 > len(body) {
		return acc_proto_tcp, ""
	}
	cml := int(body[off])
	off++
	if off+cml > len(body) {
		return acc_proto_tcp, ""
	}
	off += cml
	if off+2 > len(body) {
		return acc_proto_tcp, ""
	}
	extTotal := int(body[off])<<8 | int(body[off+1])
	off += 2
	if off+extTotal > len(body) {
		return acc_proto_tcp, ""
	}
	extEnd := off + extTotal
	for off < extEnd {
		if off+4 > extEnd {
			return acc_proto_tcp, ""
		}
		etype := int(body[off])<<8 | int(body[off+1])
		off += 2
		elen := int(body[off])<<8 | int(body[off+1])
		off += 2
		if off+elen > extEnd {
			return acc_proto_tcp, ""
		}
		if etype == 0 { // server_name
			listEnd := off + elen
			if off+2 > listEnd {
				return acc_proto_tcp, ""
			}
			listLen := int(body[off])<<8 | int(body[off+1])
			off += 2
			if off+listLen > listEnd {
				return acc_proto_tcp, ""
			}
			nameEnd := off + listLen
			for off < nameEnd {
				if off+3 > nameEnd {
					return acc_proto_tcp, ""
				}
				nt := body[off]
				off++
				nl := int(body[off])<<8 | int(body[off+1])
				off += 2
				if off+nl > nameEnd {
					return acc_proto_tcp, ""
				}
				if nt == 0 { // host_name
					host := string(body[off : off+nl])
					if host != "" && validDomainChar(host) {
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

func commonProtocolParser(data []byte) (uint8, string) {
	line := firstProtocolLine(data)
	if strings.HasPrefix(line, "SSH-") && len(line) >= 8 {
		return acc_proto_ssh, ""
	}
	upper := strings.ToUpper(line)
	if strings.HasPrefix(upper, "EHLO ") || strings.HasPrefix(upper, "HELO ") ||
		strings.HasPrefix(upper, "MAIL FROM:") || strings.HasPrefix(upper, "RCPT TO:") ||
		strings.HasPrefix(upper, "DATA") || strings.HasPrefix(upper, "QUIT") {
		return acc_proto_smtp, ""
	}
	if strings.HasPrefix(upper, "USER ") || strings.HasPrefix(upper, "PASS ") ||
		strings.HasPrefix(upper, "LIST") || strings.HasPrefix(upper, "RETR ") ||
		strings.HasPrefix(upper, "STOR ") || strings.HasPrefix(upper, "SYST") {
		return acc_proto_ftp, ""
	}
	if strings.HasPrefix(upper, "A001 ") || strings.HasPrefix(upper, "A002 ") ||
		strings.HasPrefix(upper, "A003 ") || strings.HasPrefix(upper, "LOGIN ") ||
		strings.HasPrefix(upper, "SELECT ") || strings.HasPrefix(upper, "FETCH ") {
		return acc_proto_imap, ""
	}
	if strings.HasPrefix(upper, "APOP ") || strings.HasPrefix(upper, "STAT") ||
		strings.HasPrefix(upper, "UIDL") {
		return acc_proto_pop3, ""
	}
	if strings.HasPrefix(line, "220 ") || strings.HasPrefix(line, "220-") {
		if containsAnyFold(line, "SMTP", "ESMTP", "MAIL") {
			return acc_proto_smtp, ""
		}
		if containsAnyFold(line, "FTP", "FILEZILLA") {
			return acc_proto_ftp, ""
		}
	}
	if strings.HasPrefix(line, "* OK") && containsAnyFold(line, "IMAP", "IMAP4") {
		return acc_proto_imap, ""
	}
	if strings.HasPrefix(line, "+OK") && containsAnyFold(line, "POP3", "POP") {
		return acc_proto_pop3, ""
	}
	return acc_proto_tcp, ""
}

func firstProtocolLine(data []byte) string {
	line := data
	if before, _, ok := bytes.Cut(data, []byte{'\n'}); ok {
		line = before
	}
	return strings.TrimSpace(string(line))
}

func containsAnyFold(value string, parts ...string) bool {
	value = strings.ToUpper(value)
	for _, part := range parts {
		if strings.Contains(value, part) {
			return true
		}
	}
	return false
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
