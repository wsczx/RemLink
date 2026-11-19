package utils_test

import (
	"fmt"

	"github.com/wsczx/remlink/pkg/utils"
)

// 展示口令哈希与校验的用法
func ExamplePasswordHash() {
	hash, err := utils.PasswordHash("Str0ng-Passw0rd")
	if err != nil {
		fmt.Println("hash error:", err)
		return
	}
	fmt.Println(utils.IsBcryptHash(hash))
	fmt.Println(utils.PasswordVerify("Str0ng-Passw0rd", hash))
	fmt.Println(utils.PasswordVerify("wrong", hash))
	// Output:
	// true
	// true
	// false
}

// 展示字节数的可读化输出
func ExampleHumanByte() {
	fmt.Println(utils.HumanByte(1024))
	fmt.Println(utils.HumanByte(1536))
	// Output:
	// 1.00 KB
	// 1.50 KB
}
