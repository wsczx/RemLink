package dbdata

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
)

type testAuthStub struct {
	name string
}

func (a *testAuthStub) Name() string { return a.name }
func (a *testAuthStub) Authenticate(*auth.Context) (auth.StepResult, error) {
	return auth.StepPass, nil
}

func preIpData(t *testing.T) {
	// 注册认证器
	for _, name := range []string{"local", "ldap", "radius", "otp", "cert", "saml", "wxwork", "feishu"} {
		if !auth.Registry.IsRegistered(name) {
			n := name
			auth.Registry.Register(n, func() auth.Authenticator { return &testAuthStub{name: n} })
		}
	}

	tmpDb := path.Join(t.TempDir(), "remlink_test.db")
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.DbType = "sqlite3"
		c.DbSource = tmpDb
	})
	initDb()
}

func closeIpdata() {
	xdb.Close()
	// DB 文件由 t.TempDir() 自动清理
}

func TestDb(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	u := User{Username: "a"}
	err := Add(&u)
	ast.Nil(err)

	ast.Equal(u.Id, 1)
}
