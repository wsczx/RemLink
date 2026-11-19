package service

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/dbtest"
	"github.com/wsczx/remlink/sessdata"
)

func TestMain(m *testing.M) {
	auth.SetRegistry(auth.NewProviderRegistry())
	authsrv.RegisterProviders(auth.Registry)
	SetActions(ProductionActions(sessdata.NewSessionManager()))
	os.Exit(m.Run())
}

func TestLoadPortalUser(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)
	group := seedUserGroup(t, "portal-load")

	active := &dbdata.User{Username: "portal-active", Status: 1, Groups: []string{group}}
	ast.NoError(dbdata.Users.SetUser(active))
	got, exists, err := LoadPortalUser("portal-active")
	ast.NoError(err)
	ast.True(exists)
	ast.Equal("portal-active", got.Username)

	// 记录不存在不是错误：外部认证首登由 ResolvePortalUser 投影
	got, exists, err = LoadPortalUser("portal-not-exist")
	ast.NoError(err)
	ast.False(exists)
	ast.Nil(got)

	expired := &dbdata.User{Username: "portal-expired", Status: 2, Groups: []string{group}}
	ast.NoError(dbdata.Users.SetUser(expired))
	_, exists, err = LoadPortalUser("portal-expired")
	ast.True(exists)
	ast.EqualError(err, "用户已过期")

	disabled := &dbdata.User{Username: "portal-disabled", Status: 0, Groups: []string{group}}
	ast.NoError(dbdata.Users.SetUser(disabled))
	_, exists, err = LoadPortalUser("portal-disabled")
	ast.True(exists)
	ast.EqualError(err, "用户不存在或已停用")
}

func TestPortalCandidateGroups(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	seedAuthGroup(t, "cand-local", auth.AuthMethodConfig{Type: "local"})
	seedAuthGroup(t, "cand-ldap", auth.AuthMethodConfig{Type: "ldap", Provider: "ldap-test"})

	// 本地用户：只匹配含 local 的组
	local := &dbdata.User{Username: "cand-local-user", Type: "local", Groups: []string{"cand-local", "cand-ldap"}}
	gotLocal := portalCandidateGroups(local, true)
	ast.Contains(gotLocal, "cand-local")
	ast.NotContains(gotLocal, "cand-ldap")

	// ldap 用户：只匹配含 ldap 的组
	ldapUser := &dbdata.User{Username: "cand-ldap-user", Type: "ldap", Groups: []string{"cand-local", "cand-ldap"}}
	gotLdap := portalCandidateGroups(ldapUser, true)
	ast.Contains(gotLdap, "cand-ldap")
	ast.NotContains(gotLdap, "cand-local")

	// 用户不存在：候选组取全量，且只匹配 ldap/radius
	unknown := &dbdata.User{Username: "cand-unknown", Groups: nil}
	gotUnknown := portalCandidateGroups(unknown, false)
	ast.Contains(gotUnknown, "cand-ldap", "外部用户首登应匹配 ldap 组")
	ast.NotContains(gotUnknown, "cand-local")

	// 已存在但无归属组：仍按本地用户口径筛全量
	noGroups := &dbdata.User{Username: "cand-no-group", Type: "local"}
	gotNoGroups := portalCandidateGroups(noGroups, true)
	ast.Contains(gotNoGroups, "cand-local")
	ast.NotContains(gotNoGroups, "cand-ldap")
}

func TestResolvePortalUser(t *testing.T) {
	ast := assert.New(t)

	existing := &dbdata.User{Username: "existing", Status: 1, Type: "local"}
	got, err := ResolvePortalUser("existing", "g", existing, true)
	ast.NoError(err)
	ast.Same(existing, got, "已存在用户应原样返回")

	got, err = ResolvePortalUser("new-external", "ext-group", nil, false)
	ast.NoError(err)
	ast.Equal("external", got.Type)
	ast.Equal("new-external", got.Username)
	ast.Equal([]string{"ext-group"}, got.Groups)
	ast.Equal(int8(1), got.Status)
}
