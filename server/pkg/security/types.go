package security

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/wsczx/remlink/pkg/mask"
)

// 自动加解密的字符串。
// DB 写入时 Value() 加密，读出时 Scan() 解密。
type EncryptedString string

// 加密明文：空值/占位符/已加密的值跳过。
func encryptField(s string) string {
	if !IsEnabled() || s == "" || s == mask.Placeholder || IsEncrypted(s) {
		return s
	}
	return EncryptIfNeeded(s)
}

func (e *EncryptedString) Scan(src any) error {
	var s string
	switch v := src.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	case nil:
		s = ""
	default:
		*e = ""
		return nil
	}
	*e = EncryptedString(DecryptIfNeeded(s))
	return nil
}

func (e EncryptedString) Value() (driver.Value, error) {
	return encryptField(string(e)), nil
}

func (e EncryptedString) MarshalJSON() ([]byte, error) {
	return json.Marshal(encryptField(string(e)))
}

func (e *EncryptedString) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*e = EncryptedString(DecryptIfNeeded(s))
	return nil
}

func (e EncryptedString) String() string { return string(e) }

func (e EncryptedString) IsPlaceholder() bool {
	return mask.IsPlaceholder(string(e))
}

// 返回脱敏值 "******"。
func (e EncryptedString) Masked() EncryptedString {
	return EncryptedString(mask.SecretStr(string(e)))
}

// 自动加解密的泛型 JSON 类型。
// DB 读写时 ToDB/FromDB 自动处理加解密
type EncryptedJSON[T any] struct {
	Data T
}

func (e *EncryptedJSON[T]) FromDB(b []byte) error {
	s := DecryptIfNeeded(string(b))
	if s == "" {
		return nil
	}
	return json.Unmarshal([]byte(s), &e.Data)
}

func (e EncryptedJSON[T]) ToDB() ([]byte, error) {
	data, err := json.Marshal(e.Data)
	if err != nil {
		return nil, err
	}
	s := string(data)
	if s == "" || s == "null" {
		return []byte(s), nil
	}
	if !IsEnabled() {
		return []byte(s), nil
	}
	return []byte(EncryptIfNeeded(s)), nil
}

func (e EncryptedJSON[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.Data)
}

// 返回加密形式的 JSON 字符串，用于备份落盘（避免明文泄露凭证）。
// 还原时经 FromDB：json 反序列化得到无引号密文串 -> DecryptIfNeeded -> 明文 JSON -> 写入 Data，可逆。
func (e EncryptedJSON[T]) MarshalEncrypted() ([]byte, error) {
	b, err := e.ToDB()
	if err != nil {
		return nil, err
	}
	if !IsEnabled() || len(b) == 0 {
		return json.Marshal(e.Data)
	}
	return json.Marshal(string(b))
}

func (e *EncryptedJSON[T]) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		// 备份还原等场景可能为密文字符串，统一解密
		s = DecryptIfNeeded(s)
		return json.Unmarshal([]byte(s), &e.Data)
	}
	return json.Unmarshal(data, &e.Data)
}
