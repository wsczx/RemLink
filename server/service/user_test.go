package service

import (
	"encoding/json"
	"testing"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/pkg/utils"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/dbtest"
)

func TestSaveUser_NewGeneratesPassword(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	group := seedUserGroup(t, "new")
	u := &dbdata.User{Username: "svc-new-user", Status: 1, Groups: []string{group}}
	plain, changed, err := SaveUser(u)

	ast.NoError(err)
	ast.True(changed, "新建用户的随机口令应视为变更")
	ast.Len(plain, 8, "默认口令长度应为 8")
	var saved dbdata.User
	ast.NoError(dbdata.Users.Get(u.Id, &saved))
	ast.True(utils.VerifyPassword(plain, saved.PinCode), "保存的口令应能校验通过")
}

func TestSaveUser_EditKeepsExistingPassword(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	group := seedUserGroup(t, "edit")
	u := &dbdata.User{Username: "svc-edit-user", Status: 1, Groups: []string{group}, PinCode: "orig-pass"}
	_, _, err := SaveUser(u)
	ast.NoError(err)

	var stored dbdata.User
	ast.NoError(dbdata.Users.Get(u.Id, &stored))
	edit := &dbdata.User{Id: u.Id, Username: u.Username, Status: 1, Groups: []string{group}}
	plain, changed, err := SaveUser(edit)

	ast.NoError(err)
	ast.False(changed, "修改资料未填口令不应视为口令变更")
	ast.Equal(stored.PinCode, plain, "应沿用原口令哈希")
	ast.Equal(stored.PinCode, edit.PinCode)
}

func TestSaveUser_ExplicitPasswordMarksChanged(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	group := seedUserGroup(t, "explicit")
	u := &dbdata.User{Username: "svc-explicit-user", Status: 1, Groups: []string{group}, PinCode: "new-pass"}
	plain, changed, err := SaveUser(u)
	ast.NoError(err)
	ast.True(changed)
	ast.Equal("new-pass", plain)
}

func TestSaveUser_FailureReturnsError(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	// 归属不存在的组会被 SetUser 拒绝
	u := &dbdata.User{Username: "svc-invalid-user", Status: 1, Groups: []string{"不存在的组"}}
	_, changed, err := SaveUser(u)
	ast.Error(err)
	ast.False(changed)
}

func TestAfterSaveUser(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)

	cases := []struct {
		name       string
		status     int8
		pwdChanged bool
		wantClose  bool
		wantRevoke bool
	}{
		{"正常保存", 1, false, false, false},
		{"口令变更", 1, true, false, true},
		{"停用用户", 0, false, true, true},
		{"改密并停用", 0, true, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ast := assert.New(t)
			spy := useSpy(t)

			u := &dbdata.User{Username: "svc-effect-user", Status: c.status}
			ApplyUserChange(u, c.pwdChanged)

			ast.Equal([]string{"svc-effect-user"}, spy.limitUpdated, "每次保存都要刷新在线缓存")
			if c.wantClose {
				ast.Equal([]string{"svc-effect-user"}, spy.sessionsClosed, "停用应断开在线会话")
			} else {
				ast.Empty(spy.sessionsClosed)
			}
			if c.wantRevoke {
				ast.Equal([]string{"svc-effect-user"}, spy.jwtRevoked, "改密或停用应吊销门户 JWT")
			} else {
				ast.Empty(spy.jwtRevoked)
			}
		})
	}
}

func TestDeleteUser_CleansUp(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)
	spy := useSpy(t)

	DeleteUser(&dbdata.User{Username: "svc-del-user"})
	ast.Equal([]string{"svc-del-user"}, spy.revokedUsers, "单个删除应同步吊销 WebVPN 会话")
	ast.Empty(spy.revokedUserSets, "单个删除不应走批量异步吊销")
	ast.Equal([]string{"svc-del-user"}, spy.jwtRevoked, "应吊销门户 JWT")
	ast.Equal([]string{"svc-del-user"}, spy.sessionsClosed, "应断开在线会话")
}

func TestRevokeDeletedUsers_CleansUpAll(t *testing.T) {
	base.Test()
	ast := assert.New(t)
	spy := useSpy(t)

	RevokeDeletedUsers([]string{"u1", "u2"})

	ast.Equal([][]string{{"u1", "u2"}}, spy.revokedUserSets, "应批量吊销 WebVPN 会话")
	ast.Equal([]string{"u1", "u2"}, spy.jwtRevoked, "应吊销全部门户 JWT")
	ast.Equal([]string{"u1", "u2"}, spy.sessionsClosed, "应断开全部在线会话")
}

// 建一个满足业务校验的策略，返回其 ID
func seedPolicy(t *testing.T, name string, status int8) int {
	t.Helper()
	p := &dbdata.Policy{
		Name: name, Status: status,
		ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}},
	}
	if err := dbdata.Policies.SetPolicy(p); err != nil {
		t.Fatal(err)
	}
	return p.Id
}

// 建一个策略 + 启用组，返回组名
func seedUserGroup(t *testing.T, suffix string) string {
	t.Helper()
	name := "svc-user-group-" + suffix
	policyID := seedPolicy(t, "svc-user-plcy-"+suffix, 1)
	if err := dbdata.Groups.SetGroup(&dbdata.Group{Name: name, Status: 1, PolicyId: policyID}); err != nil {
		t.Fatal(err)
	}
	return name
}

// 建一个带指定认证步骤的启用组
func seedAuthGroup(t *testing.T, name string, steps ...auth.AuthMethodConfig) {
	t.Helper()
	policyID := seedPolicy(t, name+"-plcy", 1)
	profile, err := json.Marshal(auth.GroupAuthProfile{Step: steps})
	if err != nil {
		t.Fatal(err)
	}
	if err := dbdata.Groups.SetGroup(&dbdata.Group{
		Name: name, Status: 1, PolicyId: policyID, AuthProfile: profile,
	}); err != nil {
		t.Fatal(err)
	}
}
