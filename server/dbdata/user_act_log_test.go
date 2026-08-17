package dbdata

import "testing"

func TestParseBrowserUA(t *testing.T) {
	tests := []struct {
		name     string
		ua       string
		wantName string
		wantVer  string
	}{
		{
			name:     "Chrome Windows",
			ua:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
			wantName: "Chrome",
			wantVer:  "126.0.0.0",
		},
		{
			name:     "Edge Windows",
			ua:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0",
			wantName: "Edge",
			wantVer:  "126.0.0.0",
		},
		{
			name:     "Safari macOS",
			ua:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
			wantName: "Safari",
			wantVer:  "17.5",
		},
		{
			name:     "Firefox Linux",
			ua:       "Mozilla/5.0 (X11; Linux x86_64; rv:127.0) Gecko/20100101 Firefox/127.0",
			wantName: "Firefox",
			wantVer:  "127.0",
		},
		{
			name:     "Opera",
			ua:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 OPR/110.0.0.0",
			wantName: "Opera",
			wantVer:  "110.0.0.0",
		},
		{
			name:     "unknown",
			ua:       "SomeCustomBot/1.0",
			wantName: "",
			wantVer:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVer := parseBrowserUA(tt.ua)
			if gotName != tt.wantName || gotVer != tt.wantVer {
				t.Errorf("parseBrowserUA() = %q, %q, want %q, %q", gotName, gotVer, tt.wantName, tt.wantVer)
			}
		})
	}
}

func TestParseUserAgent(t *testing.T) {
	type args struct {
		userAgent string
	}
	type res struct {
		os_idx     uint8
		client_idx uint8
		ver        string
	}
	tests := []struct {
		name string
		args args
		want res
	}{
		{
			name: "mac os 1",
			args: args{userAgent: "cisco anyconnect vpn agent for mac os x 4.10.05085"},
			want: res{os_idx: 2, client_idx: 1, ver: "4.10.05085"},
		},
		{
			name: "mac os 2",
			args: args{userAgent: "anyconnect darwin_i386 4.10.05085"},
			want: res{os_idx: 2, client_idx: 1, ver: "4.10.05085"},
		},
		{
			name: "windows",
			args: args{userAgent: "cisco anyconnect vpn agent for windows 4.8.02042"},
			want: res{os_idx: 1, client_idx: 1, ver: "4.8.02042"},
		},
		{
			name: "iPad",
			args: args{userAgent: "anyconnect applesslvpn_darwin_arm (ipad) 4.10.04060"},
			want: res{os_idx: 5, client_idx: 1, ver: "4.10.04060"},
		},
		{
			name: "iPhone",
			args: args{userAgent: "cisco anyconnect vpn agent for apple iphone 4.10.04060"},
			want: res{os_idx: 5, client_idx: 1, ver: "4.10.04060"},
		},
		{
			name: "android",
			args: args{userAgent: "anyconnect android 4.10.05096"},
			want: res{os_idx: 4, client_idx: 1, ver: "4.10.05096"},
		},
		{
			name: "linux",
			args: args{userAgent: "cisco anyconnect vpn agent for linux v7.08"},
			want: res{os_idx: 3, client_idx: 1, ver: "7.08"},
		},
		{
			// 安卓 Chrome 门户 UA 含 AppleWebKit，曾因 "apple" 关键字被误判为 iOS。
			name: "android chrome with AppleWebKit",
			args: args{userAgent: "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36"},
			want: res{os_idx: 4, client_idx: 0, ver: ""},
		},
		{
			name: "macOS apple silicon darwin_arm64",
			args: args{userAgent: "anyconnect darwin_arm64 4.10.05085"},
			want: res{os_idx: 2, client_idx: 1, ver: "4.10.05085"},
		},
		{
			name: "macOS secure mobility client",
			args: args{userAgent: "cisco anyconnect secure mobility client (macos)"},
			want: res{os_idx: 2, client_idx: 1, ver: ""},
		},
		{
			// 全空格 UA 不能触发 ver[0] 越界 Panic。
			name: "whitespace only",
			args: args{userAgent: "   "},
			want: res{os_idx: 0, client_idx: 0, ver: ""},
		},
		{
			// 空 UA 同样不能 Panic。
			name: "empty",
			args: args{userAgent: ""},
			want: res{os_idx: 0, client_idx: 0, ver: ""},
		},
		{
			name: "openconnect",
			args: args{userAgent: "openconnect-gui 1.5.3 v7.08"},
			want: res{os_idx: 0, client_idx: 2, ver: "7.08"},
		},
		{
			name: "unknown",
			args: args{userAgent: "unknown 1.4.3 aabcd"},
			want: res{os_idx: 0, client_idx: 0, ver: ""},
		},
		{
			name: "unknown 2",
			args: args{userAgent: ""},
			want: res{os_idx: 0, client_idx: 0, ver: ""},
		},
		{
			name: "anylink",
			args: args{userAgent: "anylink vpn agent for linux v1.0"},
			want: res{os_idx: 3, client_idx: 3, ver: "1.0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if os_idx, client_idx, ver := UserActLogIns.ParseUserAgent(tt.args.userAgent); os_idx != tt.want.os_idx || client_idx != tt.want.client_idx || ver != tt.want.ver {
				t.Errorf("ParseUserAgent() = %v, %v, %v, want %v, %v, %v", os_idx, client_idx, ver, tt.want.os_idx, tt.want.client_idx, tt.want.ver)
			}
		})
	}
}
