package utils

import (
	"strings"
	"testing"
)

func TestRedactSensitive(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string // 期望包含的片段
		not  string // 期望不包含的片段
	}{
		{
			name: "XML密码",
			in:   `<auth><username>u</username><password>Secret123</password></auth>`,
			want: "<password>***</password>",
			not:  "Secret123",
		},
		{
			name: "XML二次密码",
			in:   `<password>a</password><secondary_password>b</secondary_password>`,
			want: "<secondary_password>***</secondary_password>",
			not:  ">b<",
		},
		{
			name: "XML SSO token",
			in:   `<sso-token>abc123xyz</sso-token>`,
			want: "<sso-token>***</sso-token>",
			not:  "abc123xyz",
		},
		{
			name: "XML OTP secret",
			in:   `<otp_secret>JBSWY3DPEHPK3PXP</otp_secret>`,
			want: "<otp_secret>***</otp_secret>",
			not:  "JBSWY3DPEHPK3PXP",
		},
		{
			name: "大小写不敏感",
			in:   `<Password>MixedCase</Password>`,
			want: "<Password>***</Password>",
			not:  "MixedCase",
		},
		{
			name: "JSON密码",
			in:   `{"username":"u","password":"p@ss"}`,
			want: `"password":"***"`,
			not:  "p@ss",
		},
		{
			name: "JSON带空格",
			in:   `{"password" : "spaced"}`,
			want: `"password":"***"`,
			not:  "spaced",
		},
		{
			name: "非敏感字段不受影响",
			in:   `<username>admin</username><group>default</group>`,
			want: "<username>admin</username>",
			not:  "",
		},
		{
			name: "空密码值",
			in:   `<password></password>`,
			want: "<password>***</password>",
			not:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactSensitive(tt.in)
			if !strings.Contains(got, tt.want) {
				t.Errorf("RedactSensitive(%q) = %q, 应包含 %q", tt.in, got, tt.want)
			}
			if tt.not != "" && strings.Contains(got, tt.not) {
				t.Errorf("RedactSensitive(%q) = %q, 不应包含敏感值 %q", tt.in, got, tt.not)
			}
		})
	}
}
