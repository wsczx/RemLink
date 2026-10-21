package utils

import "regexp"

// 敏感字段脱敏正则，覆盖两种常见形态：
//   - XML 标签对（CSTP 认证请求体：<password>xxx</password>）
//   - JSON 键值对（部分 API 请求体："password":"xxx"）
var (
	sensitiveXMLRe  = regexp.MustCompile(`(?i)<(password|secondary_password|sso-token|sso_token|otp_secret)>[^<]*</(password|secondary_password|sso-token|sso_token|otp_secret)>`)
	sensitiveJSONRe = regexp.MustCompile(`(?i)"(password|secondary_password|sso_token|sso-token|otp_secret)"\s*:\s*"(?:\\.|[^"\\])*"`)
)

// 脱敏字符串中的敏感字段值（密码/SSO 令牌/OTP 密钥等）
// 用于 trace 级别请求体日志：httputil.DumpRequest(r, true) 会把认证 XML 里的
// 明文口令/OTP/SSO token 写入日志并广播到管理后台，落盘与外发 SIEM 时造成
// 凭据扩散。脱敏保留报文结构（便于排障）仅替换敏感值
func RedactSensitive(s string) string {
	s = sensitiveXMLRe.ReplaceAllString(s, "<$1>***</$2>")
	s = sensitiveJSONRe.ReplaceAllString(s, `"$1":"***"`)
	return s
}
