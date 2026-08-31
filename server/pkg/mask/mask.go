// Package mask 提供敏感字段脱敏函数。
package mask

// Placeholder 脱敏占位符。非空敏感字段脱敏后统一替换为此值。
const Placeholder = "******"

// 通用敏感字段脱敏。空字符串返回空，非空返回 Placeholder。
func Secret(s string) string {
	if s == "" {
		return ""
	}
	return Placeholder
}

// 检查值是否为脱敏占位符或空（前端未修改）。
func IsPlaceholder(s string) bool {
	return s == "" || s == Placeholder
}

// 对外暴露的脱敏实现，供 security.EncryptedString.Masked() 等类型方法内部调用。
func SecretStr(s string) string {
	return Secret(s)
}
