package dbdata_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/dbtest"
)

func TestSetGroupErrorMessages(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	newPolicy := func(name string, status int8) int {
		t.Helper()
		p := &dbdata.Policy{Name: name, Status: status, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}}
		ast.NoError(dbdata.Policies.SetPolicy(p))
		return p.Id
	}
	enabledPolicy := newPolicy("group-validate-plcy", 1)
	disabledPolicy := newPolicy("group-validate-off-plcy", 0)

	valid := func() *dbdata.Group {
		return &dbdata.Group{Name: "group-validate", Status: 1, PolicyId: enabledPolicy}
	}
	// 填全独立 v4 网段四项
	withCidr := func(g *dbdata.Group, cidr string) *dbdata.Group {
		g.ClientCidr = cidr
		g.ClientStart = "192.168.50.10"
		g.ClientEnd = "192.168.50.200"
		g.ClientGateway = "192.168.50.1"
		return g
	}

	cases := []struct {
		name    string
		mutate  func(*dbdata.Group)
		wantErr string
	}{
		{"组名为空", func(g *dbdata.Group) { g.Name = "" }, "用户组名错误"},
		{"启用组缺策略", func(g *dbdata.Group) { g.PolicyId = 0 }, "启用的用户组必须指定策略"},
		{"引用不存在的策略", func(g *dbdata.Group) { g.PolicyId = 999999 }, "引用的策略不存在"},
		{"引用停用策略", func(g *dbdata.Group) { g.PolicyId = disabledPolicy }, "引用的策略已停用，请选择启用的策略"},
		{"拆分域名非法", func(g *dbdata.Group) {
			g.SplitDns = []dbdata.ValData{{Val: "not a domain!"}}
		}, "域名 错误"},
		{"出网网卡不存在", func(g *dbdata.Group) { g.OutDev = "eth-does-not-exist" }, "出网网卡 eth-does-not-exist 不存在"},
		{"v4 网段配置不全", func(g *dbdata.Group) { g.ClientCidr = "192.168.50.0/24" }, "组 IP 配置必须全部填写：网段、起始地址、结束地址、网关"},
		{"v4 网段格式错", func(g *dbdata.Group) { withCidr(g, "bad-cidr") }, "组 IP 网段格式无效"},
		{"v4 网段填了 v6", func(g *dbdata.Group) { withCidr(g, "fd00::/64") }, "组 IP 网段必须是 IPv4 CIDR（不能填 IPv6）"},
		{"起始地址不在网段", func(g *dbdata.Group) {
			withCidr(g, "192.168.50.0/24")
			g.ClientStart = "10.0.0.1"
		}, "组 IP 起始地址 不在网段内"},
		{"起始大于结束", func(g *dbdata.Group) {
			withCidr(g, "192.168.50.0/24")
			g.ClientStart, g.ClientEnd = g.ClientEnd, g.ClientStart
		}, "组 IP 起始地址必须小于结束地址"},
		{"未配 v4 却配 v6", func(g *dbdata.Group) { g.ClientCidr6 = "fd00::/64" }, "组 IPv6 网段需要先启用独立 IP 段"},
		{"v6 格式错", func(g *dbdata.Group) {
			withCidr(g, "192.168.50.0/24")
			g.ClientCidr6 = "bad-v6"
		}, "组 IPv6 网段格式无效"},
		{"v6 填了 v4", func(g *dbdata.Group) {
			withCidr(g, "192.168.50.0/24")
			g.ClientCidr6 = "10.0.0.0/8"
		}, "组 IPv6 网段必须是 IPv6 CIDR"},
		{"v6 前缀为 128", func(g *dbdata.Group) {
			withCidr(g, "192.168.50.0/24")
			g.ClientCidr6 = "fd00::1/128"
		}, "组 IPv6 网段前缀须小于 128 才有分配空间"},
		{"认证配置非法", func(g *dbdata.Group) {
			g.AuthProfile = json.RawMessage(`不是 JSON`)
		}, "认证配置格式无效"},
		{"未知认证方式", func(g *dbdata.Group) {
			g.AuthProfile = json.RawMessage(`{"step":[{"type":"unknown"}]}`)
		}, "未知的认证方式"},
		{"外部认证缺 Provider", func(g *dbdata.Group) {
			g.AuthProfile = json.RawMessage(`{"step":[{"type":"ldap"}]}`)
		}, "必须设置 Provider"},
		{"本地认证带 Provider", func(g *dbdata.Group) {
			g.AuthProfile = json.RawMessage(`{"step":[{"type":"local","provider":"x"}]}`)
		}, "不支持 Provider 引用"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := valid()
			c.mutate(g)
			err := dbdata.Groups.SetGroup(g)
			if !assert.Error(t, err, "应被校验拒绝") {
				return
			}
			assert.Contains(t, err.Error(), c.wantErr)
		})
	}
}

func TestSetGroupDefaults(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	p := &dbdata.Policy{Name: "group-default-plcy", Status: 1, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}}
	ast.NoError(dbdata.Policies.SetPolicy(p))

	g := &dbdata.Group{
		Name: "group-default", Status: 1, PolicyId: p.Id,
		SplitDns: []dbdata.ValData{{Val: "  a.example.com  "}, {Val: ""}},
	}
	ast.NoError(dbdata.Groups.SetGroup(g))
	ast.JSONEq(`{"step":[{"type":"local"}]}`, string(g.AuthProfile), "未配置时应补本地口令认证")
	if ast.Len(g.SplitDns, 1) {
		ast.Equal("a.example.com", g.SplitDns[0].Val, "拆分域名应被 trim 并去空")
	}
	ast.False(g.UpdatedAt.IsZero())
}
