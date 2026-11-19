package dbdata_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/dbtest"
)

func TestSetPolicyErrorMessages(t *testing.T) {
	base.Test()

	valid := func() *dbdata.Policy {
		return &dbdata.Policy{
			Name:         "policy-validate",
			Status:       1,
			ClientDns:    []dbdata.ValData{{Val: "8.8.8.8"}},
			RouteInclude: nil, // 默认路由
		}
	}
	// 非默认路由，用于绕过「默认路由必须设 DNS」的前置检查
	splitRoute := func(p *dbdata.Policy) {
		p.RouteInclude = []dbdata.ValData{{Val: "10.0.0.0/8"}}
	}

	cases := []struct {
		name    string
		mutate  func(*dbdata.Policy)
		wantErr string
		exact   bool
	}{
		{"名称为空", func(p *dbdata.Policy) { p.Name = "" }, "策略名称不能为空", true},
		{"下行带宽为负", func(p *dbdata.Policy) { p.Bandwidth = -1 }, "下行带宽不能小于0", true},
		{"上行带宽为负", func(p *dbdata.Policy) { p.BandwidthUp = -1 }, "上行带宽不能小于0", true},
		{"流量配额为负", func(p *dbdata.Policy) { p.TrafficQuota = -1 }, "流量配额不能小于0", true},
		{"重置周期非法", func(p *dbdata.Policy) { p.TrafficReset = "bogus" }, "流量配额重置周期错误，可选: daily/weekly/monthly", true},
		{"包含路由格式错", func(p *dbdata.Policy) { p.RouteInclude = []dbdata.ValData{{Val: "not-a-cidr"}} }, "RouteInclude 错误", false},
		{"排除路由格式错", func(p *dbdata.Policy) { p.RouteExclude = []dbdata.ValData{{Val: "not-a-cidr"}} }, "RouteExclude 错误", false},
		{"ACL 网段格式错", func(p *dbdata.Policy) {
			p.LinkAcl = []dbdata.GroupLinkAcl{{Val: "not-a-cidr", Port: "80"}}
		}, "GroupLinkAcl 错误", false},
		{"ACL 端口格式错", func(p *dbdata.Policy) {
			p.LinkAcl = []dbdata.GroupLinkAcl{{Val: "192.168.1.0/24", Port: "abc"}}
		}, "格式错误", false},
		{"DNS IP 非法", func(p *dbdata.Policy) { p.ClientDns = []dbdata.ValData{{Val: "999.1.1.1"}} }, "DNS IP 错误", true},
		{"默认路由缺 DNS", func(p *dbdata.Policy) { p.ClientDns = nil }, "默认路由，必须设置一个DNS", true},
		{"包含排除域名并存", func(p *dbdata.Policy) {
			p.DsIncludeDomains, p.DsExcludeDomains = "a.com", "b.com"
		}, "包含/排除域名不能同时填写", true},
		{"默认路由设包含域名", func(p *dbdata.Policy) { p.DsIncludeDomains = "a.com" }, "不允许设置", false},
		{"FakeDNS 缺上游", func(p *dbdata.Policy) {
			splitRoute(p)
			p.ClientDns = nil
			p.EnableFakeDNS = true
		}, "必须配置 FakeDNS 上游 DNS", false},
		{"FakeDNS 上游格式错", func(p *dbdata.Policy) {
			p.EnableFakeDNS = true
			p.FakeDNSUpstream = "not-an-ip"
			p.FakeDNSInclude = "a.com"
		}, "FakeDNS 上游 DNS 格式错误（应为 IP[:port]）", true},
		{"FakeDNS 包含排除并存", func(p *dbdata.Policy) {
			p.EnableFakeDNS = true
			p.FakeDNSInclude, p.FakeDNSExclude = "a.com", "b.com"
		}, "FakeDNS 包含/排除域名不能同时填写", true},
		{"FakeDNS 域名都为空", func(p *dbdata.Policy) {
			p.EnableFakeDNS = true
			p.FakeDNSUpstream = "1.1.1.1"
		}, "必须至少填写", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ast := assert.New(t)
			p := valid()
			c.mutate(p)
			err := dbdata.Policies.SetPolicy(p)
			if !ast.Error(err, "应被校验拒绝") {
				return
			}
			if c.exact {
				ast.Equal(c.wantErr, err.Error())
			} else {
				ast.Contains(err.Error(), c.wantErr)
			}
		})
	}
}

func TestSetPolicyNormalization(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	p := &dbdata.Policy{Name: "policy-norm", Status: 1, ClientDns: []dbdata.ValData{{Val: " 8.8.8.8 "}}, TrafficQuota: 100}
	ast.NoError(dbdata.Policies.SetPolicy(p))

	ast.Equal("monthly", p.TrafficReset, "配额>0 且周期为空时应补 monthly")
	if ast.Len(p.ClientDns, 1) {
		ast.Equal("8.8.8.8", p.ClientDns[0].Val, "DNS 应被 trim")
	}
	ast.False(p.UpdatedAt.IsZero(), "应写入更新时间")
}
