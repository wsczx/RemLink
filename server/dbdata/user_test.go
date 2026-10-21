package dbdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveGroupFromUsers(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	// u0：目标组在列表首位
	u0 := User{Type: "local", Username: "rgf-u0", Groups: []string{"todelete", "keep0"}, Status: 1}
	// u1：目标组在列表次位
	u1 := User{Type: "local", Username: "rgf-u1", Groups: []string{"keep1", "todelete"}, Status: 1}
	// u2：目标组在列表次位但用户下标为 2
	u2 := User{Type: "local", Username: "rgf-u2", Groups: []string{"keep2", "todelete"}, Status: 1}
	// u3：不含目标组，应完全不动
	u3 := User{Type: "local", Username: "rgf-u3", Groups: []string{"other"}, Status: 1}
	for _, u := range []*User{&u0, &u1, &u2, &u3} {
		ast.Nil(Add(u))
	}

	ast.Nil(RemoveGroupFromUsers("todelete"))

	check := func(username string, want []string) {
		var u User
		ast.Nil(One("username", username, &u))
		ast.Equal(want, u.Groups, username+" 的组关系应被正确清理")
	}
	check("rgf-u0", []string{"keep0"})
	check("rgf-u1", []string{"keep1"})
	check("rgf-u2", []string{"keep2"})
	check("rgf-u3", []string{"other"})
}

// 目标组是用户唯一组时，清理后 Groups 为空（delByID 回调依赖此路径
// 触发 Delete(&localUser)，而非留下空组名残留）
func TestRemoveGroupFromUsers_LastGroup(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	u := User{Type: "local", Username: "rgf-solo", Groups: []string{"todelete"}, Status: 1}
	ast.Nil(Add(&u))

	ast.Nil(RemoveGroupFromUsers("todelete"))

	var got User
	ast.Nil(One("username", "rgf-solo", &got))
	ast.Empty(got.Groups, "唯一组被移除后 Groups 应为空")
}
