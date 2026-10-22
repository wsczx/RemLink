package dbdata_test

import (
	"testing"

	_ "github.com/wsczx/remlink/auth/authsrv" // 触发认证器注册
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/dbtest"

	"github.com/stretchr/testify/assert"
)

// 编辑用户时，表单未提供的计数字段/创建时间/OTP 密钥不应被 AllCols 清零或重生成
func TestSetUser_PreservesCountersAndOtpOnEdit(t *testing.T) {
	dbtest.SetupDB(t)
	ast := assert.New(t)
	// SetUser 会按组名校验 Groups，测试库需先存在 default 组
	ast.NoError(dbdata.SetPolicy(&dbdata.Policy{Name: "test-policy", Status: 1, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}}))
	ast.NoError(dbdata.SetGroup(&dbdata.Group{Name: "default", Status: 1, PolicyId: 1}))

	u := &dbdata.User{
		Username:  "preserve",
		Nickname:  "n",
		Type:      "local",
		Groups:    []string{"default"},
		PinCode:   "secret123",
		OtpSecret: "OLDSECRET",
		Status:    1,
	}
	ast.NoError(dbdata.Add(u))

	// 模拟已用流量写入库
	var created dbdata.User
	ast.NoError(dbdata.One("Username", "preserve", &created))
	created.TrafficUsed = 1000
	ast.NoError(dbdata.Set(&created))

	// 编辑：仅改昵称与密码，不传 OtpSecret / TrafficUsed
	edit := &dbdata.User{
		Id:       created.Id,
		Username: "preserve",
		Nickname: "newnick",
		Type:     "local",
		Groups:   []string{"default"},
		PinCode:  "newpwd",
	}
	ast.NoError(dbdata.SetUser(edit))

	var got dbdata.User
	ast.NoError(dbdata.One("Id", created.Id, &got))
	ast.Equal("newnick", got.Nickname)
	ast.Equal(int64(1000), got.TrafficUsed, "编辑不应清零流量计数(TrafficUsed)")
	ast.Equal("OLDSECRET", got.OtpSecret, "编辑不应重生成 OTP 密钥(OtpSecret)")
}

// 显式传入 OtpSecret 时应被采用
func TestSetUser_ExplicitOtpSecretApplied(t *testing.T) {
	dbtest.SetupDB(t)
	ast := assert.New(t)
	ast.NoError(dbdata.SetPolicy(&dbdata.Policy{Name: "test-policy", Status: 1, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}}))
	ast.NoError(dbdata.SetGroup(&dbdata.Group{Name: "default", Status: 1, PolicyId: 1}))

	u := &dbdata.User{Username: "rotate", Type: "local", Groups: []string{"default"}, PinCode: "secret123", OtpSecret: "OLD", Status: 1}
	ast.NoError(dbdata.Add(u))
	var created dbdata.User
	ast.NoError(dbdata.One("Username", "rotate", &created))

	edit := &dbdata.User{Id: created.Id, Username: "rotate", Type: "local", Groups: []string{"default"}, PinCode: "newpwd", OtpSecret: "NEWSECRET"}
	ast.NoError(dbdata.SetUser(edit))

	var got dbdata.User
	ast.NoError(dbdata.One("Id", created.Id, &got))
	ast.Equal("NEWSECRET", got.OtpSecret, "显式传入的 OTP 密钥应被采用")
}
