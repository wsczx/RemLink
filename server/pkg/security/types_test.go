package security

import (
	"encoding/json"
	"strings"
	"testing"
)

func enableForTest(t *testing.T) {
	t.Helper()
	SetDir(t.TempDir())
	if err := GenerateKey(); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := LoadKey(); err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	t.Cleanup(func() { _ = DeleteKey() })
}

func TestEncryptedJSON_MarshalEncryptedHidesPlaintext(t *testing.T) {
	enableForTest(t)

	const secret = "super-secret-bind-pwd"
	ej := EncryptedJSON[json.RawMessage]{
		Data: json.RawMessage(`{"client_id":"abc","bind_pwd":"` + secret + `"}`),
	}

	enc, err := ej.MarshalEncrypted()
	if err != nil {
		t.Fatalf("MarshalEncrypted: %v", err)
	}
	if strings.Contains(string(enc), secret) {
		t.Fatalf("MarshalEncrypted 泄露明文凭证: %s", enc)
	}

	// 还原（模拟备份恢复时的 json.Unmarshal 进入 UnmarshalJSON）
	var restored EncryptedJSON[json.RawMessage]
	if err := json.Unmarshal(enc, &restored); err != nil {
		t.Fatalf("restore Unmarshal: %v", err)
	}
	if string(restored.Data) != string(ej.Data) {
		t.Fatalf("还原不一致: got %s want %s", restored.Data, ej.Data)
	}
}

func TestEncryptedJSON_MarshalJSONStaysPlaintextForAPI(t *testing.T) {
	enableForTest(t)

	const secret = "super-secret-bind-pwd"
	ej := EncryptedJSON[json.RawMessage]{
		Data: json.RawMessage(`{"bind_pwd":"` + secret + `"}`),
	}

	// API 序列化必须保持明文，脱敏由调用方(maskProviderSecrets)处理。
	out, err := ej.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if !strings.Contains(string(out), secret) {
		t.Fatalf("MarshalJSON 应为明文供 API 脱敏使用: %s", out)
	}
}
