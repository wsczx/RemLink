package service

import (
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/dbtest"
	"github.com/wsczx/remlink/sessdata"
)

func TestActionsFieldsNotNil(t *testing.T) {
	ast := assert.New(t)
	v := reflect.ValueOf(ProductionActions(sessdata.NewSessionManager()))
	for i := range v.NumField() {
		ast.NotNil(v.Field(i).Interface(), "%s 必须绑定", v.Type().Field(i).Name)
	}
}

// 收集联动调用记录
type spyActions struct {
	certInvalidated int
	natRemoved      [][3]string
	syncedGroups    []string
	revokedGroups   [][]string
	revokedUsers    []string
	revokedUserSets [][]string
	jwtRevoked      []string
	limitUpdated    []string
	sessionsClosed  []string
}

func (s *spyActions) bindings() Actions {
	return Actions{
		RemoveGroupNAT:      func(v4, v6, dev string) { s.natRemoved = append(s.natRemoved, [3]string{v4, v6, dev}) },
		InvalidateCertCache: func() { s.certInvalidated++ },
		SyncExternalUsers:   func(g *dbdata.Group) { s.syncedGroups = append(s.syncedGroups, g.Name) },
		RevokeGroupWebVpn:   func(names []string) { s.revokedGroups = append(s.revokedGroups, names) },
		RevokeUserWebVpn:    func(username string) error { s.revokedUsers = append(s.revokedUsers, username); return nil },
		RevokeUsersWebVpn:   func(usernames []string) { s.revokedUserSets = append(s.revokedUserSets, usernames) },
		RevokeJwt:           func(username string) int { s.jwtRevoked = append(s.jwtRevoked, username); return 0 },
		UpdateUserLimitTime: func(username string, _ *time.Time) { s.limitUpdated = append(s.limitUpdated, username) },
		CloseUserSessions:   func(username string, _ uint8) { s.sessionsClosed = append(s.sessionsClosed, username) },
	}
}

func useSpy(t *testing.T) *spyActions {
	t.Helper()
	old := actions
	spy := &spyActions{}
	actions = spy.bindings()
	t.Cleanup(func() { actions = old })
	return spy
}

// 填全组的独立 v4 网段配置（SetGroup 要求四项同时填写）
func withCidr(g *dbdata.Group, cidr string) *dbdata.Group {
	g.ClientCidr = cidr
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return g
	}
	b := ip.To4()
	g.ClientStart = net.IPv4(b[0], b[1], b[2], 10).String()
	g.ClientEnd = net.IPv4(b[0], b[1], b[2], 200).String()
	g.ClientGateway = net.IPv4(b[0], b[1], b[2], 1).String()
	return g
}

func TestSaveGroup_NewGroup(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)
	spy := useSpy(t)

	g := &dbdata.Group{Name: "svc-new-group", Status: 1, PolicyId: seedPolicy(t, "svc-new-plcy", 1)}
	ast.NoError(SaveGroup(g))
	ast.NotZero(g.Id, "保存后应回填 Id")
	ast.Equal(1, spy.certInvalidated, "证书认证缓存必须失效")
	ast.Empty(spy.natRemoved, "新建组没有旧 NAT 需要清理")
	ast.Equal([][]string{{"svc-new-group"}}, spy.revokedGroups, "保存成功后应按组名触发 WebVPN 撤回")
	ast.Equal([]string{"svc-new-group"}, spy.syncedGroups)

	var saved dbdata.Group
	ast.NoError(dbdata.Groups.Get(g.Id, &saved))
	ast.Equal("svc-new-group", saved.Name)
}

func TestSaveGroup_CidrChangedRemovesOldNAT(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)
	spy := useSpy(t)

	g := withCidr(&dbdata.Group{
		Name: "svc-nat-group", Status: 1, PolicyId: seedPolicy(t, "svc-nat-plcy", 1),
		ClientCidr6: "fd00::/64",
	}, "10.10.0.0/24")
	ast.NoError(SaveGroup(g))

	withCidr(g, "10.20.0.0/24")
	ast.NoError(SaveGroup(g))
	ast.Len(spy.natRemoved, 1, "CIDR 变化应清理旧 NAT 一次")
	ast.Equal([3]string{"10.10.0.0/24", "fd00::/64", ""}, spy.natRemoved[0])
}

func TestSaveGroup_NoCidrChangeKeepsNAT(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)
	spy := useSpy(t)

	g := withCidr(&dbdata.Group{
		Name: "svc-stable-group", Status: 1, PolicyId: seedPolicy(t, "svc-stable-plcy", 1),
	}, "10.30.0.0/24")
	ast.NoError(SaveGroup(g))
	g.Note = "只改备注"
	ast.NoError(SaveGroup(g))
	ast.Empty(spy.natRemoved, "CIDR 未变不应清理 NAT")
	ast.Equal(2, spy.certInvalidated, "每次保存都要失效证书缓存")
}

func TestSaveGroup_FailureSkipsActions(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)
	spy := useSpy(t)

	// 缺策略的启用组会被 SetGroup 拒绝
	g := &dbdata.Group{Name: "svc-invalid-group", Status: 1}
	ast.Error(SaveGroup(g))
	ast.Zero(spy.certInvalidated, "保存失败不应触发联动")
	ast.Empty(spy.syncedGroups)
	ast.Empty(spy.natRemoved)
}

func TestDeleteGroup_CleansUp(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)
	spy := useSpy(t)

	g := withCidr(&dbdata.Group{
		Name: "svc-del-group", Status: 1, PolicyId: seedPolicy(t, "svc-del-plcy", 1),
	}, "10.40.0.0/24")
	ast.NoError(SaveGroup(g))
	u := &dbdata.User{Username: "svc-del-user", Status: 1, Groups: []string{"svc-del-group"}}
	ast.NoError(dbdata.Users.SetUser(u))

	DeleteGroup(g)

	ast.Equal(2, spy.certInvalidated, "保存与删除各失效一次缓存")
	ast.Equal([][3]string{{"10.40.0.0/24", "", ""}}, spy.natRemoved)
	ast.Equal([][]string{{"svc-del-group"}, {"svc-del-group"}}, spy.revokedGroups, "保存与删除各触发一次组撤回")

	var reloaded dbdata.User
	ast.NoError(dbdata.Users.Get(u.Id, &reloaded))
	ast.NotContains(reloaded.Groups, "svc-del-group", "已删组名应从用户 Groups 中清除")
}
