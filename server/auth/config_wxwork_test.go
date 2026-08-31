package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWXWorkConfig_ParseDepartments(t *testing.T) {
	ast := assert.New(t)

	// 空字符串返回 nil（不限制）
	ast.Nil((&WXWorkConfig{AllowedDepartments: ""}).ParseDepartments())
	ast.Nil((&WXWorkConfig{AllowedDepartments: "  "}).ParseDepartments())

	// 正常逗号分隔
	ast.Equal([]int{1, 2, 3}, (&WXWorkConfig{AllowedDepartments: "1,2,3"}).ParseDepartments())

	// 带空格
	ast.Equal([]int{10, 20}, (&WXWorkConfig{AllowedDepartments: " 10 , 20 "}).ParseDepartments())

	// 含非法片段被忽略
	ast.Equal([]int{5}, (&WXWorkConfig{AllowedDepartments: "5,abc,,0,-3"}).ParseDepartments())

	// 全部非法返回 nil
	ast.Nil((&WXWorkConfig{AllowedDepartments: "x,y"}).ParseDepartments())
}

func TestWXWorkConfig_ParseBlockedUserIDs(t *testing.T) {
	ast := assert.New(t)

	ast.Nil((&WXWorkConfig{BlockedUserIDs: ""}).ParseBlockedUserIDs())
	ast.Nil((&WXWorkConfig{BlockedUserIDs: " , "}).ParseBlockedUserIDs())

	ast.Equal([]string{"zhangsan", "lisi"}, (&WXWorkConfig{BlockedUserIDs: "zhangsan,lisi"}).ParseBlockedUserIDs())
	ast.Equal([]string{"a", "b"}, (&WXWorkConfig{BlockedUserIDs: " a , b "}).ParseBlockedUserIDs())
	// 空片段被忽略
	ast.Equal([]string{"a", "b"}, (&WXWorkConfig{BlockedUserIDs: "a,,b,"}).ParseBlockedUserIDs())
}

func TestWXWorkConfig_CheckUserID(t *testing.T) {
	ast := assert.New(t)

	blocked := []string{"zhangsan", "lisi"}
	// 空拒绝列表表示不限制，任何用户都允许
	ast.False((&WXWorkConfig{}).CheckUserID("zhangsan", nil))
	ast.False((&WXWorkConfig{}).CheckUserID("zhangsan", []string{}))

	// 命中拒绝列表
	ast.True((&WXWorkConfig{}).CheckUserID("zhangsan", blocked))
	// 未命中
	ast.False((&WXWorkConfig{}).CheckUserID("wangwu", blocked))
	// 大小写敏感
	ast.False((&WXWorkConfig{}).CheckUserID("ZhangSan", blocked))
}
