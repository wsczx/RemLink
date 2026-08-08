package security

import (
	"encoding/json"
	"testing"
)

// 固定测试密钥（32 字节），用于不依赖磁盘密钥文件的注入式单测。
var testKey = []byte("0123456789abcdef0123456789abcdef")

func TestNewCipherEncryptDecryptRoundtrip(t *testing.T) {
	c, err := NewCipher(testKey)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	const plain = "hello-remlink-加密"
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == plain {
		t.Fatal("Encrypt 未改变明文")
	}
	if !IsEncrypted(enc) {
		t.Fatalf("密文缺前缀: %s", enc)
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != plain {
		t.Fatalf("往返不一致: got %q want %q", dec, plain)
	}
}

func TestNewCipherRejectsWrongKeySize(t *testing.T) {
	if _, err := NewCipher([]byte("short")); err == nil {
		t.Fatal("短密钥应报错")
	}
	if _, err := NewCipher(make([]byte, 16)); err == nil {
		t.Fatal("16 字节密钥应报错（须 32）")
	}
	// 32 字节正常
	if _, err := NewCipher(testKey); err != nil {
		t.Fatalf("32 字节密钥应成功: %v", err)
	}
}

// NewCipher 与全局密钥包级函数应基于同一密钥，密文可互相解密。
// 注意：AES-GCM 每次加密使用随机 nonce，故密文字节本身不要求相同（这是安全特性）。
func TestNewCipherMatchesGlobalKey(t *testing.T) {
	enableForTest(t) // 全局 key 已加载

	key := getKeyLocked()
	if key == nil {
		t.Fatal("全局密钥未启用")
	}

	c, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}

	const plain = "一致性校验数据"
	encPkg, err := Encrypt(plain)
	if err != nil {
		t.Fatalf("包级 Encrypt: %v", err)
	}
	encCipher, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Cipher.Encrypt: %v", err)
	}
	// 二者密文应可互相解密（验证同一密钥、同一算法）。
	if dec, err := Decrypt(encCipher); err != nil || dec != plain {
		t.Fatalf("包级 Decrypt 解 Cipher 密文失败: %v %q", err, dec)
	}
	if dec, err := c.Decrypt(encPkg); err != nil || dec != plain {
		t.Fatalf("Cipher.Decrypt 解包级密文失败: %v %q", err, dec)
	}
}

func TestNewCipherNonPrefixPassthrough(t *testing.T) {
	c, _ := NewCipher(testKey)
	if v, _ := c.Decrypt("明文无前缀"); v != "明文无前缀" {
		t.Fatalf("非前缀应透传: got %q", v)
	}
}

// DecryptJSON 应递归解密 JSON 中所有密文字符串值（含嵌套对象、数组、混合字段），
// 非密文字段原样保留，结构不破坏。
func TestDecryptJSON_Recursive(t *testing.T) {
	enableForTest(t)

	encSecret, err := Encrypt("super-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	encPwd, err := Encrypt("bind-password")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	raw := []byte(`{
		"name": "provider-a",
		"config": {"bind_pwd": "` + encPwd + `", "plain": "hello", "nested": {"token": "` + encSecret + `"}},
		"items": [{"key": "` + encSecret + `"}, {"key": "plain-item"}],
		"count": 3,
		"enabled": true
	}`)

	out, err := DecryptJSON(raw)
	if err != nil {
		t.Fatalf("DecryptJSON: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("解析解密结果: %v", err)
	}

	// 顶层明文保留
	if m["name"] != "provider-a" {
		t.Fatalf("顶层明文丢失: %v", m["name"])
	}
	// 嵌套对象密文解密
	cfg := m["config"].(map[string]any)
	if cfg["bind_pwd"] != "bind-password" {
		t.Fatalf("嵌套 bind_pwd 未解密: %v", cfg["bind_pwd"])
	}
	if cfg["plain"] != "hello" {
		t.Fatalf("嵌套明文丢失: %v", cfg["plain"])
	}
	nested := cfg["nested"].(map[string]any)
	if nested["token"] != "super-secret" {
		t.Fatalf("深层嵌套 token 未解密: %v", nested["token"])
	}
	// 数组元素密文解密
	items := m["items"].([]any)
	if items[0].(map[string]any)["key"] != "super-secret" {
		t.Fatalf("数组元素未解密: %v", items[0])
	}
	if items[1].(map[string]any)["key"] != "plain-item" {
		t.Fatalf("数组明文元素丢失: %v", items[1])
	}
	// 非字符串值类型保持
	if m["count"].(float64) != 3 {
		t.Fatalf("数字类型破坏: %v", m["count"])
	}
	if m["enabled"].(bool) != true {
		t.Fatalf("布尔类型破坏: %v", m["enabled"])
	}
}

// DecryptJSON 对非法 JSON 应返回错误而非 panic。
func TestDecryptJSON_InvalidJSON(t *testing.T) {
	enableForTest(t)
	if _, err := DecryptJSON([]byte(`{not-json`)); err == nil {
		t.Fatal("非法 JSON 应返回错误")
	}
}

// DecryptJSON 对全明文 JSON 应原样返回（无密文时不改写）。
func TestDecryptJSON_AllPlain(t *testing.T) {
	enableForTest(t)
	raw := []byte(`{"a":"x","b":1}`)
	out, err := DecryptJSON(raw)
	if err != nil {
		t.Fatalf("DecryptJSON: %v", err)
	}
	if string(out) != string(raw) {
		t.Fatalf("全明文不应改写: got %s want %s", out, raw)
	}
}
