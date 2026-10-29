package authsrv

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/dbtest"
)

func preTestData(t *testing.T) {
	dbtest.SetupDB(t)

	// 注册所需认证器（防止 Pipeline 构建时找不到）
	for _, name := range []string{"local", "ldap", "radius", "otp", "cert", "saml", "wxwork", "feishu"} {
		if !auth.Registry.IsRegistered(name) {
			n := name
			auth.Registry.Register(n, func() auth.Authenticator {
				return &testStubAuth{name: n}
			})
		}
	}

	defaultGroup()
}

func defaultGroup() {
	groups := dbdata.GetGroupNames()
	if slices.Contains(groups, "default") {
		return
	}
	_ = dbdata.SetPolicy(&dbdata.Policy{Name: "test-policy", Status: 1, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}})
	_ = dbdata.SetGroup(&dbdata.Group{Name: "default", Status: 1, PolicyId: 1})
}
func TestAuthenticate_PassesForGroupWithoutLocalRecord(t *testing.T) {
	preTestData(t)

	auth.Registry.Register("test_noop_group", func() auth.Authenticator {
		return &testStubAuth{name: "test_noop_group"}
	})

	groupB := &dbdata.Group{
		Name:        "group-b",
		Status:      1,
		PolicyId:    1,
		AuthProfile: json.RawMessage(`{"step":[{"type":"test_noop_group"}]}`),
	}
	assert.NoError(t, dbdata.SetGroup(groupB))

	// 该用户在本地库中没有任何记录
	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "external-user",
			Password:  "testpass123",
			GroupName: "group-b",
		},
	}
	result := Authenticate(ctx)

	assert.Equal(t, auth.StepPass, result.Result)
	assert.NoError(t, result.Err)
}

type testStubAuth struct {
	auth.NopChallenger
	name string
}

func (a *testStubAuth) Name() string { return a.name }
func (a *testStubAuth) Authenticate(*auth.Context) (auth.StepResult, error) {
	return auth.StepPass, nil
}
func TestAuthenticate_DisabledGroup(t *testing.T) {
	preTestData(t)

	disabledGroup := &dbdata.Group{
		Name:        "disabled_group",
		Status:      2,
		AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`),
	}
	assert.NoError(t, dbdata.SetGroup(disabledGroup))

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "testuser",
			Password:  "testpass123",
			GroupName: "disabled_group",
		},
	}
	result := Authenticate(ctx)
	assert.Equal(t, auth.StepFail, result.Result)
	assert.NotNil(t, result.Err)
	assert.Contains(t, result.Err.Error(), "已禁用")
}

func TestCertAutoAuth_DisabledGroup(t *testing.T) {
	preTestData(t)

	disabledGroup := &dbdata.Group{
		Name:        "disabled_cert_group",
		Status:      2,
		AuthProfile: json.RawMessage(`{"step":[{"type":"cert"}]}`),
	}
	assert.NoError(t, dbdata.SetGroup(disabledGroup))

	assert.False(t, CertAutoAuth("disabled_cert_group"))
}
