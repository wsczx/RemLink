package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/wsczx/remlink/base"
)

// 密文前缀，用于区分明文和密文。
const prefix = "$AES256$"

var ErrNotEnabled = errors.New("加密未启用")

// 使用 AES-256-GCM 执行加解密
type Cipher struct {
	key []byte
}

// 使用 32 字节密钥创建 Cipher
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, errors.New("密钥长度必须为 32 字节（AES-256）")
	}
	return &Cipher{key: key}, nil
}

// 返回 "$AES256$<base64>" 格式的密文
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// 解密带有 "$AES256$" 前缀的密文，其他值原样返回
func (c *Cipher) Decrypt(ciphertext string) (string, error) {
	if !strings.HasPrefix(ciphertext, prefix) {
		return ciphertext, nil
	}

	b64 := strings.TrimPrefix(ciphertext, prefix)
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("密文数据不完整")
	}
	nonce, ct := data[:nonceSize], data[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
func Encrypt(plaintext string) (string, error) {
	key := getKeyLocked()
	if key == nil {
		return "", ErrNotEnabled
	}
	c, err := NewCipher(key)
	if err != nil {
		return "", err
	}
	return c.Encrypt(plaintext)
}
func Decrypt(ciphertext string) (string, error) {
	if !strings.HasPrefix(ciphertext, prefix) {
		return ciphertext, nil
	}
	key := getKeyLocked()
	if key == nil {
		return "", ErrNotEnabled
	}
	c, err := NewCipher(key)
	if err != nil {
		return "", err
	}
	return c.Decrypt(ciphertext)
}

// 判断值是否为密文。
func IsEncrypted(s string) bool { return strings.HasPrefix(s, prefix) }

// 返回密文前缀
func Prefix() string { return prefix }

// 加密：密文/空值直接返回，明文加密。
func EncryptIfNeeded(s string) string {
	if s == "" || IsEncrypted(s) {
		return s
	}
	enc, err := Encrypt(s)
	if err != nil {
		base.Error("加密失败，返回明文: ", err)
		return s
	}
	return enc
}

// 解密：非密文直接返回，密文解密。
func DecryptIfNeeded(s string) string {
	if !IsEncrypted(s) {
		return s
	}
	dec, err := Decrypt(s)
	if err != nil {
		base.Error("解密失败，返回密文: ", err)
		return s
	}
	return dec
}

// 递归解密 JSON 中所有密文字符串值，返回解密后的 JSON。
func DecryptJSON(raw []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	decryptJSONVal(m)
	return json.Marshal(m)
}

func decryptJSONVal(v any) {
	switch val := v.(type) {
	case map[string]any:
		for k, vv := range val {
			if s, ok := vv.(string); ok && IsEncrypted(s) {
				if dec, err := Decrypt(s); err == nil {
					val[k] = dec
				}
			} else {
				decryptJSONVal(vv)
			}
		}
	case []any:
		for _, vv := range val {
			decryptJSONVal(vv)
		}
	}
}
