package handler

import "testing"

func TestCommonProtocolParser(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		proto uint8
	}{
		{name: "ssh", data: "SSH-2.0-OpenSSH_9.6\r\n", proto: acc_proto_ssh},
		{name: "ftp", data: "220 ftp.example.com FTP server ready\r\n", proto: acc_proto_ftp},
		{name: "smtp", data: "220 mail.example.com ESMTP ready\r\n", proto: acc_proto_smtp},
		{name: "imap", data: "* OK [CAPABILITY IMAP4rev1] ready\r\n", proto: acc_proto_imap},
		{name: "pop3", data: "+OK POP3 server ready\r\n", proto: acc_proto_pop3},
		{name: "smtp command", data: "EHLO client.example\r\n", proto: acc_proto_smtp},
		{name: "ftp command", data: "USER anonymous\r\n", proto: acc_proto_ftp},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proto, info := commonProtocolParser([]byte(test.data))
			if proto != test.proto || info != "" {
				t.Fatalf("commonProtocolParser() = (%d, %q), want (%d, empty)", proto, info, test.proto)
			}
		})
	}
}

func TestCommonProtocolParserRejectsAmbiguousGreeting(t *testing.T) {
	for _, greeting := range []string{"220 service ready\r\n", "+OK ready\r\n", "* OK ready\r\n"} {
		if proto, info := commonProtocolParser([]byte(greeting)); proto != acc_proto_tcp || info != "" {
			t.Fatalf("ambiguous greeting %q classified as (%d, %q)", greeting, proto, info)
		}
	}
}
