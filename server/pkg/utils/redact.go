package utils

import "regexp"

// 敏感字段脱敏正则：XML 标签对与 JSON 键值对两种形态
var (
	sensitiveXMLRe    = regexp.MustCompile(`(?i)<(password|secondary_password|sso-token|sso_token|otp_secret)>[^<]*</(password|secondary_password|sso-token|sso_token|otp_secret)>`)
	sensitiveJSONRe   = regexp.MustCompile(`(?i)"(password|secondary_password|sso_token|sso-token|otp_secret)"\s*:\s*"(?:\\.|[^"\\])*"`)
	sensitiveHeaderRe = regexp.MustCompile(`(?im)^(X-DTLS-Master-Secret|Cookie):.*$`)
)

// 用于 trace 级请求体日志：DumpRequest(r, true) 会把明文口令/OTP/SSO token
// 写入日志并外发 SIEM，脱敏仅替换敏感值、保留报文结构
func RedactSensitive(s string) string {
	s = sensitiveXMLRe.ReplaceAllString(s, "<$1>***</$2>")
	s = sensitiveJSONRe.ReplaceAllString(s, `"$1":"***"`)
	s = sensitiveHeaderRe.ReplaceAllString(s, "$1: ***")
	return s
}
