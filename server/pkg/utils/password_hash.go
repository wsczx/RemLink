package utils

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	mt "math/rand"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func PasswordHash(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func PasswordVerify(password, hash string) bool {
	// 保留老用户明文验证：以 bcrypt 前缀判定
	if IsBcryptHash(hash) {
		err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
		return err == nil
	}
	return subtle.ConstantTimeCompare([]byte(hash), []byte(password)) == 1
}

// 判断字符串是否为 bcrypt 哈希
// bcrypt 标准前缀为 $2a$/$2b$/$2y$，固定长度 60。
func IsBcryptHash(hash string) bool {
	if len(hash) != 60 {
		return false
	}
	return strings.HasPrefix(hash, "$2a$") ||
		strings.HasPrefix(hash, "$2b$") ||
		strings.HasPrefix(hash, "$2y$")
}

func RandSecret(min int, max int) (string, error) {
	rb := make([]byte, randInt(min, max))
	_, err := rand.Read(rb)
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(rb), nil
}

func randInt(min int, max int) int {
	return min + mt.Intn(max-min)
}

// 校验密码复杂度：至少 8 位，且同时包含字母和数字
func CheckPasswordPolicy(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("密码长度不能少于 8 位")
	}
	hasLetter := false
	hasDigit := false
	for _, c := range password {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			hasLetter = true
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return fmt.Errorf("密码需同时包含字母和数字")
	}
	return nil
}
