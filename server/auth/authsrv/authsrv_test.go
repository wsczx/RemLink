package authsrv

import (
	"encoding/json"
	"path"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

func preTestData(t *testing.T) {
	base.Test()

	// 注册所需认证器（防止 Pipeline 构建时找不到）
	for _, name := range []string{"local", "ldap", "radius", "otp", "cert", "saml", "wxwork", "feishu"} {
		if !auth.Registry.IsRegistered(name) {
			n := name
			auth.Registry.Register(n, func() auth.Authenticator {
				return &testStubAuth{name: n}
			})
		}
	}

	tmpDb := path.Join(t.TempDir(), "remlink_methods_test.db")
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.DbType = "sqlite3"
		c.DbSource = tmpDb
		c.Ipv4CIDR = "192.168.3.0/24"
		c.Ipv4Gateway = "192.168.3.1"
		c.Ipv4Start = "192.168.3.100"
		c.Ipv4End = "192.168.3.150"
		c.MaxClient = 100
		c.MaxUserClient = 3
		c.IpLease = 5
	})

	dbdata.Start()

	ensureDefaultGroup()

	// 注册清理：停止 DB 连接，释放 worker pool
	t.Cleanup(func() {
		_ = dbdata.Stop()
		dbdata.UserActLogIns.Pool.Release()
		dbdata.UserActLogIns.Pool = utils.NewWorkerPool(1, 100)
	})
}

func ensureDefaultGroup() {
	groups := dbdata.GetGroupNames()
	if slices.Contains(groups, "default") {
		return
	}
	_ = dbdata.SetPolicy(&dbdata.Policy{Name: "test-policy", Status: 1, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}})
	_ = dbdata.SetGroup(&dbdata.Group{Name: "default", Status: 1, PolicyId: 1})
}

// testStubAuth 桩认证器，用于测试管道构建
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
