package dbdata_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/dbtest"
)

func seedApplyPolicy(t *testing.T, name string, status int8) int {
	t.Helper()
	p := &dbdata.Policy{Name: name, Status: status, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}}
	if err := dbdata.Policies.SetPolicy(p); err != nil {
		t.Fatal(err)
	}
	return p.Id
}

func seedApplyGroup(t *testing.T, name string, policyID int) *dbdata.Group {
	t.Helper()
	g := &dbdata.Group{Name: name, Status: 1, PolicyId: policyID}
	if err := dbdata.Groups.SetGroup(g); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestPolicyGetByID(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	id := seedApplyPolicy(t, "apply-enabled", 1)
	p, err := dbdata.Policies.GetByID(id)
	ast.NoError(err)
	ast.Equal("apply-enabled", p.Name)

	_, err = dbdata.Policies.GetByID(999999)
	ast.ErrorIs(err, dbdata.ErrPolicyNotFound)
}

func TestGroupsSetPolicy(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	oldID := seedApplyPolicy(t, "assign-old", 1)
	newID := seedApplyPolicy(t, "assign-new", 1)
	g1 := seedApplyGroup(t, "assign-g1", oldID)
	g2 := seedApplyGroup(t, "assign-g2", oldID)

	n, err := dbdata.Groups.SetPolicy([]int{g1.Id, g2.Id, 999999}, newID)
	ast.NoError(err)
	ast.Equal(2, n, "不存在的 ID 静默跳过，只计成功数")

	for _, id := range []int{g1.Id, g2.Id} {
		var g dbdata.Group
		ast.NoError(dbdata.GetDB().Get("Id", id, &g))
		ast.Equal(newID, g.PolicyId)
	}
}

func TestUsersSetPolicy(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	oldID := seedApplyPolicy(t, "assign-u-old", 1)
	newID := seedApplyPolicy(t, "assign-u-new", 1)
	grp := seedApplyGroup(t, "assign-u-grp", oldID)
	u := &dbdata.User{Username: "assign-user", Status: 1, Groups: []string{grp.Name}, PolicyId: oldID}
	ast.NoError(dbdata.Users.SetUser(u))

	n, err := dbdata.Users.SetPolicy([]int{u.Id, 999999}, newID)
	ast.NoError(err)
	ast.Equal(1, n)

	var stored dbdata.User
	ast.NoError(dbdata.GetDB().Get("Id", u.Id, &stored))
	ast.Equal(newID, stored.PolicyId)
}

func TestGroupsClearPolicy(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	targetID := seedApplyPolicy(t, "clear-target", 1)
	otherID := seedApplyPolicy(t, "clear-other", 1)
	mine := seedApplyGroup(t, "clear-mine", targetID)
	other := seedApplyGroup(t, "clear-other-group", otherID)

	n, err := dbdata.Groups.ClearPolicy([]int{mine.Id, other.Id, 999999}, targetID)
	ast.NoError(err)
	ast.Equal(1, n, "只清除引用目标策略的组")

	var g1, g2 dbdata.Group
	ast.NoError(dbdata.GetDB().Get("Id", mine.Id, &g1))
	ast.NoError(dbdata.GetDB().Get("Id", other.Id, &g2))
	ast.Zero(g1.PolicyId)
	ast.Equal(otherID, g2.PolicyId, "引用其他策略的组不得误清")
}

func TestUsersClearPolicy(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	targetID := seedApplyPolicy(t, "clear-u-target", 1)
	otherID := seedApplyPolicy(t, "clear-u-other", 1)
	grp := seedApplyGroup(t, "clear-u-grp", targetID)
	mine := &dbdata.User{Username: "clear-u-mine", Status: 1, Groups: []string{grp.Name}, PolicyId: targetID}
	other := &dbdata.User{Username: "clear-u-other", Status: 1, Groups: []string{grp.Name}, PolicyId: otherID}
	ast.NoError(dbdata.Users.SetUser(mine))
	ast.NoError(dbdata.Users.SetUser(other))

	n, err := dbdata.Users.ClearPolicy([]int{mine.Id, other.Id, 999999}, targetID)
	ast.NoError(err)
	ast.Equal(1, n)

	var u1, u2 dbdata.User
	ast.NoError(dbdata.GetDB().Get("Id", mine.Id, &u1))
	ast.NoError(dbdata.GetDB().Get("Id", other.Id, &u2))
	ast.Zero(u1.PolicyId)
	ast.Equal(otherID, u2.PolicyId, "引用其他策略的用户不得误清")
}
