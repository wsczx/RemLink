package dbdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 非法 IPv4（"abc"、IPv6 地址）入库，可能导致 MAC 客户端登录时 IP 池分配 panic 且连接数泄漏
func TestSetIpMapValidation(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	err := SetIpMap(&IpMap{IpAddr: "192.168.3.100", MacAddr: "aa:bb:cc:dd:ee:01"})
	ast.Nil(err)

	err = SetIpMap(&IpMap{IpAddr: "abc", MacAddr: "aa:bb:cc:dd:ee:02"})
	ast.Error(err)

	err = SetIpMap(&IpMap{IpAddr: "2001:db8::1", MacAddr: "aa:bb:cc:dd:ee:03"})
	ast.Error(err)

	err = SetIpMap(&IpMap{IpAddr: "192.168.3.101", IpAddr6: "2001:db8:3::10", MacAddr: "aa:bb:cc:dd:ee:04"})
	ast.Nil(err)

	err = SetIpMap(&IpMap{IpAddr: "192.168.3.102", IpAddr6: "not-an-ip", MacAddr: "aa:bb:cc:dd:ee:05"})
	ast.Error(err)

	// ::ffff:x.x.x.x 映射地址 To4() 非 nil，属合法 v4
	err = SetIpMap(&IpMap{IpAddr: "::ffff:192.168.3.103", MacAddr: "aa:bb:cc:dd:ee:06"})
	ast.Nil(err)
}
