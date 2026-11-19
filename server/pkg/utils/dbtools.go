package utils

import (
	"crypto/subtle"
	"errors"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func EscapeLike(s string) string {
	r := make([]byte, 0, len(s)*2)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '%', '_', '"':
			r = append(r, '\\')
		}
		r = append(r, s[i])
	}
	return string(r)
}

func ContainsPortInStr(portStr string, port uint16) bool {
	portStr = strings.TrimSpace(portStr)
	if portStr == "" || portStr == "0" {
		return true
	}
	for pt := range strings.SplitSeq(portStr, ",") {
		pt = strings.TrimSpace(pt)
		if pt == "" {
			continue
		}
		if idx := strings.Index(pt, "-"); idx > 0 {
			from, err1 := strconv.ParseUint(pt[:idx], 10, 16)
			to, err2 := strconv.ParseUint(pt[idx+1:], 10, 16)
			if err1 != nil || err2 != nil {
				continue
			}
			if uint16(from) <= port && port <= uint16(to) {
				return true
			}
		} else {
			p, err := strconv.ParseUint(pt, 10, 16)
			if err != nil {
				continue
			}
			if uint16(p) == port {
				return true
			}
		}
	}
	return false
}

func ValidateDomainName(domain string) bool {
	regExp := regexp.MustCompile(`^([a-zA-Z0-9][-a-zA-Z0-9]{0,62}\.)+[A-Za-z]{2,18}$`)
	return regExp.MatchString(domain)
}

func IsExpired(limitTime *time.Time) bool {
	return limitTime != nil && !time.Now().Before(*limitTime)
}

func VerifyPassword(password, pinCode string) bool {
	if IsBcryptHash(pinCode) {
		return PasswordVerify(password, pinCode)
	}
	return subtle.ConstantTimeCompare([]byte(pinCode), []byte(password)) == 1
}

func StructName(data any) string {
	t := reflect.TypeOf(data)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

// ErrNotFound 表示数据库记录不存在
var ErrNotFound = errors.New("record not found")

// 判断错误是否为记录不存在
func CheckErrNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
