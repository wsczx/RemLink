package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/dbtest"
	"github.com/wsczx/remlink/pkg/jwt"
)

func TestPortalAuthTypeLabels(t *testing.T) {
	ast := assert.New(t)

	raw := json.RawMessage(`{"step":[{"type":"local"},{"type":"otp"}]}`)
	ast.Equal([]string{"本地密码", "动态验证码"}, portalAuthTypeLabels(raw))

	// SSO 单独成组时可解析
	ast.Equal([]string{"企微"}, portalAuthTypeLabels(json.RawMessage(`{"step":[{"type":"wxwork"}]}`)))

	ast.Nil(portalAuthTypeLabels(json.RawMessage(`不是 JSON`)))
	ast.Nil(portalAuthTypeLabels(json.RawMessage(`{"step":[]}`)), "步骤为空属非法配置")
}

func TestPortalDnsList(t *testing.T) {
	ast := assert.New(t)
	ast.Equal([]string{"8.8.8.8", "1.1.1.1"},
		portalDnsList([]dbdata.ValData{{Val: "8.8.8.8"}, {Val: ""}, {Val: "1.1.1.1"}}))
	ast.Equal([]string{}, portalDnsList(nil))
}

func TestPortalPolicyInfo(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	ast.Nil(portalPolicyInfo(0), "policyId<=0 时无策略信息")
	ast.Nil(portalPolicyInfo(-1))

	pt := &dbdata.Policy{Name: "view-plcy", Status: 1, Bandwidth: 100, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}}
	ast.Nil(dbdata.Policies.SetPolicy(pt))

	info := portalPolicyInfo(pt.Id)
	ast.NotNil(info)
	ast.Equal("view-plcy", info["name"])
	ast.Equal(100, info["bandwidth"])
}

func TestPortalGroupsDetail(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	pt := &dbdata.Policy{Name: "view-grp-plcy", Status: 1, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}}
	ast.Nil(dbdata.Policies.SetPolicy(pt))
	g := &dbdata.Group{
		Name: "view-group", Status: 1, PolicyId: pt.Id,
		AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`),
	}
	ast.Nil(dbdata.Groups.SetGroup(g))

	details := PortalGroupsDetail([]string{"view-group", "不存在"}, 0)
	ast.Len(details, 1, "不存在的组应被跳过")
	d := details[0]
	ast.Equal("view-group", d["name"])
	ast.Equal([]string{"本地密码"}, d["auth_types"])
	ast.NotNil(d["policy"], "用户未指定策略时应带上组策略")

	// 用户自带策略时不再挂组策略
	details2 := PortalGroupsDetail([]string{"view-group"}, pt.Id)
	ast.Len(details2, 1)
	ast.Nil(details2[0]["policy"])
}

func TestPortalUserInfo(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	pt := &dbdata.Policy{Name: "view-user-plcy", Status: 1, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}}
	ast.Nil(dbdata.Policies.SetPolicy(pt))
	ast.Nil(dbdata.Groups.SetGroup(&dbdata.Group{
		Name: "view-user-group", Status: 1, PolicyId: pt.Id,
	}))
	u := &dbdata.User{
		Username: "view-user", Nickname: "昵称", Type: "local", Status: 1,
		Groups: []string{"view-user-group"},
	}
	ast.Nil(dbdata.Users.SetUser(u))

	info := PortalUserInfo(u, "https://vpn.example.com:8800")
	ast.Equal(u.Id, info["id"])
	ast.Equal("view-user", info["username"])
	ast.Equal("昵称", info["name"])
	ast.Equal("https://vpn.example.com:8800", info["server_addr"])
	ast.Equal(true, info["can_change_password"])
	ast.NotNil(info["groups_detail"])
	ast.NotNil(info["dashboard"])
}

func TestCheckLocalPassword(t *testing.T) {
	ast := assert.New(t)
	u := &dbdata.User{Username: "u", PinCode: "plain-pass"}
	ast.NoError(VerifyLocalPassword(u, "plain-pass"))
	ast.Error(VerifyLocalPassword(u, "wrong"))
	ast.Error(VerifyLocalPassword(u, ""), "空密码应拒绝")
}

func TestIssuePortalToken(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	u := &dbdata.User{Id: 7, Username: "token-user", Type: "local", Groups: []string{"g1"}}
	token, err := IssuePortalToken(u)
	ast.NoError(err)
	ast.NotEmpty(token)

	data, err := jwt.GetJwtData(token)
	ast.NoError(err)
	ast.Equal("token-user", data["portal_user"])
	ast.Equal("local", data["portal_type"])
	ast.Equal(float64(7), data["portal_user_id"])

	// 令牌应已登记，改密/停用时可批量吊销
	ast.GreaterOrEqual(jwt.RevokeSubjectJwt("token-user"), 1)
}

func TestPreparePortalLogin(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	group := seedUserGroup(t, "precheck")
	u := &dbdata.User{Username: "precheck-user", Status: 1, Type: "local", Groups: []string{group}}
	ast.NoError(dbdata.Users.SetUser(u))

	pre, err := PreparePortalLogin("precheck-user")
	ast.NoError(err)
	ast.True(pre.UserExists)
	ast.Equal("precheck-user", pre.User.Username)
	ast.Contains(pre.Groups, group)

	// 不存在的用户给出空壳对象，供管道尝试外部认证
	pre, err = PreparePortalLogin("precheck-unknown")
	ast.NoError(err)
	ast.False(pre.UserExists)
	ast.Equal("precheck-unknown", pre.User.Username)
	ast.NotNil(pre.User)
}

func TestPreparePortalLoginPropagatesStateError(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	group := seedUserGroup(t, "precheck-err")
	expired := &dbdata.User{Username: "precheck-expired", Status: 2, Groups: []string{group}}
	ast.NoError(dbdata.Users.SetUser(expired))

	pre, err := PreparePortalLogin("precheck-expired")
	ast.Nil(pre)
	ast.EqualError(err, "用户已过期")
}
