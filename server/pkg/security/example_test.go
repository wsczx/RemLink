package security_test

import (
	"fmt"

	"github.com/wsczx/remlink/pkg/security"
)

// 展示敏感字段的加解密与按需解密用法
func ExampleEncryptIfNeeded() {
	c, err := security.NewCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		fmt.Println("cipher error:", err)
		return
	}
	cipher := c
	enc, err := cipher.Encrypt("s3cret")
	if err != nil {
		fmt.Println("encrypt error:", err)
		return
	}
	fmt.Println(security.IsEncrypted(enc))
	dec, err := cipher.Decrypt(enc)
	if err != nil {
		fmt.Println("decrypt error:", err)
		return
	}
	fmt.Println(dec)
	// Output:
	// true
	// s3cret
}
