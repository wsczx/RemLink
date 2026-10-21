package dbdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 验证 SetIpMap 入口校验
// 非法 IPv4（如 "abc"、IPv6 地址）曾可通过 len>=4 检查入库
// 导致对应 MAC 客户端每次登录都在 IP 池分配处 panic 且连接数泄漏
func TestSetIpMapValidation(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	// 合法 IPv4 + MAC：应通过
	err := SetIpMap(&IpMap{IpAddr: "192.168.3.100", MacAddr: "aa:bb:cc:dd:ee:01"})
	ast.Nil(err)

	// 非法 IPv4 字符串：应拒绝
	err = SetIpMap(&IpMap{IpAddr: "abc", MacAddr: "aa:bb:cc:dd:ee:02"})
	ast.Error(err)

	// IPv6 地址填入 v4 字段：应拒绝
	err = SetIpMap(&IpMap{IpAddr: "2001:db8::1", MacAddr: "aa:bb:cc:dd:ee:03"})
	ast.Error(err)

	// 合法 v4 + 合法 v6：应通过
	err = SetIpMap(&IpMap{IpAddr: "192.168.3.101", IpAddr6: "2001:db8:3::10", MacAddr: "aa:bb:cc:dd:ee:04"})
	ast.Nil(err)

	// 合法 v4 + 非法 v6：应拒绝
	err = SetIpMap(&IpMap{IpAddr: "192.168.3.102", IpAddr6: "not-an-ip", MacAddr: "aa:bb:cc:dd:ee:05"})
	ast.Error(err)

	// v4 字段填 IPv4 格式的 v6 映射地址（::ffff:192.168.3.103 To4() 非 nil，属合法 v4）：应通过
	err = SetIpMap(&IpMap{IpAddr: "::ffff:192.168.3.103", MacAddr: "aa:bb:cc:dd:ee:06"})
	ast.Nil(err)
}
