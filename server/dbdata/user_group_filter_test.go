package dbdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserGroupFilterSQL(t *testing.T) {
	preIpData(t)
	ast := assert.New(t)

	cases := []struct {
		username string
		groups   []string
	}{
		{"u-default", []string{"default"}},
		{"u-dev", []string{"dev"}},
		{"u-devops", []string{"devops"}},
		{"u-both", []string{"default", "dev"}},
		{"u-dev2", []string{"dev2"}},
	}
	for _, c := range cases {
		u := &User{Username: c.username, Groups: c.groups, Status: 1}
		ast.NoError(Add(u), "插入测试用户失败: "+c.username)
	}

	groupWhere := func(group string) (string, []any) {
		return "groups LIKE ? ESCAPE '\\'", []any{"%\"" + EscapeLike(group) + "\"%"}
	}

	query := func(group string) []string {
		where, args := groupWhere(group)
		var users []User
		ast.NoError(FindWhere(&users, 0, 0, where, args...))
		names := make([]string, 0, len(users))
		for _, u := range users {
			names = append(names, u.Username)
		}
		return names
	}

	ast.ElementsMatch([]string{"u-dev", "u-both"}, query("dev"), "dev 不得误中同名前缀的组")

	ast.ElementsMatch([]string{"u-default", "u-both"}, query("default"))

	ast.ElementsMatch([]string{"u-devops"}, query("devops"))
	where, args := groupWhere("dev")
	ast.Equal(2, FindWhereCount(&User{}, where, args...), "计数应与查询结果一致")
}

func TestUserPagingSQL(t *testing.T) {
	preIpData(t)
	ast := assert.New(t)

	const total = 25
	for i := 0; i < total; i++ {
		u := &User{Username: "pager-" + pad3(i), Groups: []string{"default"}, Status: 1}
		ast.NoError(Add(u))
	}

	ast.Equal(total, FindWhereCount(&User{}, "username LIKE ?", "pager-%"), "总数应正确")

	seen := map[string]bool{}
	pageSize := 10
	for page := 1; page <= 3; page++ {
		var users []User
		ast.NoError(FindWhere(&users, pageSize, page, "username LIKE ?", "pager-%"))
		ast.LessOrEqual(len(users), pageSize)
		for _, u := range users {
			ast.False(seen[u.Username], "不应出现重复记录: "+u.Username)
			seen[u.Username] = true
		}
	}
	ast.Len(seen, total, "分页应完整覆盖全部记录")
}

func pad3(i int) string {
	s := itoa(i)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
