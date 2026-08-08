package mask

import "testing"

func TestSecret(t *testing.T) {
	// 空字符串脱敏后应为空（前端未填写/未修改）
	if got := Secret(""); got != "" {
		t.Fatalf("Secret(\"\") = %q, want empty", got)
	}
	// 非空敏感字段统一替换为占位符
	if got := Secret("password123"); got != Placeholder {
		t.Fatalf("Secret(\"password123\") = %q, want %q", got, Placeholder)
	}
	// 占位符本身非空，不应再被替换（保持幂等）
	if got := Secret(Placeholder); got != Placeholder {
		t.Fatalf("Secret(Placeholder) = %q, want %q", got, Placeholder)
	}
}

func TestIsPlaceholder(t *testing.T) {
	// 占位符与空字符串都视作"未修改"
	if !IsPlaceholder(Placeholder) {
		t.Fatal("IsPlaceholder(Placeholder) = false, want true")
	}
	if !IsPlaceholder("") {
		t.Fatal("IsPlaceholder(\"\") = false, want true")
	}
	// 真实明文不视为占位符
	if IsPlaceholder("realvalue") {
		t.Fatal("IsPlaceholder(\"realvalue\") = true, want false")
	}
}

func TestSecretStr(t *testing.T) {
	// SecretStr 与 Secret 行为一致
	if got := SecretStr(""); got != "" {
		t.Fatalf("SecretStr(\"\") = %q, want empty", got)
	}
	if got := SecretStr("x"); got != Placeholder {
		t.Fatalf("SecretStr(\"x\") = %q, want %q", got, Placeholder)
	}
}
