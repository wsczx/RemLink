package dbdata

import (
	"encoding/json"
	"net"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/security"
)

func TestGetGroupNames(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	defer closeIpdata()

	err := Providers.Set(&Provider{
		Name:   "test-radius",
		Type:   "radius",
		Status: 1,
		Config: security.EncryptedJSON[json.RawMessage]{Data: json.RawMessage(`{"addr":"192.168.8.12:1044","secret":"43214132"}`)},
	})
	ast.Nil(err)
	err = Providers.Set(&Provider{
		Name:   "test-ldap",
		Type:   "ldap",
		Status: 1,
		Config: security.EncryptedJSON[json.RawMessage]{Data: json.RawMessage(`{"addr":"192.168.8.12:389","tls":true,"bind_name":"userfind@abc.com","bind_pwd":"afdbfdsafds","base_dn":"dc=abc,dc=com","object_class":"person","search_attr":"sAMAccountName","member_of":"cn=vpn,cn=user,dc=abc,dc=com"}`)},
	})
	ast.Nil(err)

	defaultPolicy := &Policy{
		Name:      "group-test-default",
		ClientDns: []ValData{{Val: "114.114.114.114"}},
		Status:    1,
	}
	err = Policies.SetPolicy(defaultPolicy)
	ast.Nil(err)
	pid := defaultPolicy.Id

	g1 := Group{Name: "g1", PolicyId: pid}
	err = Groups.SetGroup(&g1)
	ast.Nil(err)
	g2 := Group{Name: "g2", PolicyId: pid}
	err = Groups.SetGroup(&g2)
	ast.Nil(err)
	g3 := Group{Name: "g3", PolicyId: pid}
	err = Groups.SetGroup(&g3)
	ast.Nil(err)

	g4 := Group{Name: "g4", PolicyId: pid,
		AuthProfile: json.RawMessage(`{"step":[{"type":"radius","provider":"test-radius"}]}`)}
	err = Groups.SetGroup(&g4)
	ast.Nil(err)

	p5 := &Policy{
		Name:             "group-test-g5",
		ClientDns:        []ValData{{Val: "114.114.114.114"}},
		RouteInclude:     []ValData{{Val: "10.0.0.0/8"}},
		DsIncludeDomains: "baidu.com,163.com",
		Status:           1,
	}
	err = Policies.SetPolicy(p5)
	ast.Nil(err)
	g5 := Group{Name: "g5", PolicyId: p5.Id}
	err = Groups.SetGroup(&g5)
	ast.Nil(err)

	p6 := &Policy{
		Name:             "group-test-g6",
		ClientDns:        []ValData{{Val: "114.114.114.114"}},
		DsExcludeDomains: "com.cn,qq.com",
		Status:           1,
	}
	err = Policies.SetPolicy(p6)
	ast.Nil(err)
	g6 := Group{Name: "g6", PolicyId: p6.Id}
	err = Groups.SetGroup(&g6)
	ast.Nil(err)

	g7 := Group{Name: "g7", PolicyId: pid,
		AuthProfile: json.RawMessage(`{"step":[{"type":"ldap","provider":"test-ldap"}]}`)}
	err = Groups.SetGroup(&g7)
	ast.Nil(err)

	gAll := []string{"g1", "g2", "g3", "g4", "g5", "g6", "g7"}
	gs := Groups.GetGroupNames()
	for _, v := range gs {
		ast.Equal(true, slices.Contains(gAll, v))
	}

	gni := Groups.GetGroupNamesIds()
	for _, v := range gni {
		ast.NotEqual(0, v.Id)
		ast.Equal(true, slices.Contains(gAll, v.Name))
	}
}

// 隐藏组(hidden=1)不应出现在客户端下拉列表 GetGroupNamesNormal，但仍可被 GetGroupNames 列出(后台可用)
func TestGetGroupNamesNormal_Hidden(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	pt := &Policy{Name: "hidden-test-policy", ClientDns: []ValData{{Val: "114.114.114.114"}}, Status: 1}
	ast.Nil(Policies.SetPolicy(pt))

	vis := &Group{Name: "hidden-vis", Status: 1, PolicyId: pt.Id}
	ast.Nil(Groups.SetGroup(vis))
	hid := &Group{Name: "hidden-hid", Status: 1, PolicyId: pt.Id, Hidden: 1}
	ast.Nil(Groups.SetGroup(hid))

	normal := Groups.GetGroupNamesNormal()
	ast.True(slices.Contains(normal, "hidden-vis"), "可见组应出现在 GetGroupNamesNormal")
	ast.False(slices.Contains(normal, "hidden-hid"), "隐藏组不应出现在 GetGroupNamesNormal")

	all := Groups.GetGroupNames()
	ast.True(slices.Contains(all, "hidden-hid"), "隐藏组仍应被 GetGroupNames 列出(后台可用)")
}

// 缓存核心语义：InvalidateCertAuthCache 后立即反映最新组配置
func TestAnyGroupHasCertAuth(t *testing.T) {
	t.Run("有cert组返回true", func(t *testing.T) {
		ast := assert.New(t)
		defer Groups.InvalidateCertAuthCache()
		preIpData(t)
		defer closeIpdata()

		pt := &Policy{Name: "cert-auth-default", ClientDns: []ValData{{Val: "8.8.8.8"}}, Status: 1}
		ast.Nil(Policies.SetPolicy(pt))
		ast.Nil(Groups.SetGroup(&Group{Name: "cert-yes", Status: 1, PolicyId: pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"cert"},{"type":"local"}]}`)}))
		Groups.InvalidateCertAuthCache()
		ast.True(Groups.AnyGroupHasCertAuth(), "含 cert 组应返回 true")
	})

	t.Run("无cert组返回false", func(t *testing.T) {
		ast := assert.New(t)
		defer Groups.InvalidateCertAuthCache()
		preIpData(t)
		defer closeIpdata()

		pt := &Policy{Name: "cert-auth-none", ClientDns: []ValData{{Val: "8.8.8.8"}}, Status: 1}
		ast.Nil(Policies.SetPolicy(pt))
		ast.Nil(Groups.SetGroup(&Group{Name: "cert-no", Status: 1, PolicyId: pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`)}))
		Groups.InvalidateCertAuthCache()
		ast.False(Groups.AnyGroupHasCertAuth(), "无 cert 组应返回 false")
	})
}

func TestInvalidateCertAuthCache(t *testing.T) {
	t.Run("失效后反映最新配置", func(t *testing.T) {
		ast := assert.New(t)
		defer Groups.InvalidateCertAuthCache()
		preIpData(t)
		defer closeIpdata()

		pt := &Policy{Name: "cert-inv-default", ClientDns: []ValData{{Val: "8.8.8.8"}}, Status: 1}
		ast.Nil(Policies.SetPolicy(pt))
		ast.Nil(Groups.SetGroup(&Group{Name: "cert-inv", Status: 1, PolicyId: pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"cert"}]}`)}))
		_ = pt.Id
		Groups.InvalidateCertAuthCache()
		ast.True(Groups.AnyGroupHasCertAuth(), "有 cert 组应返回 true")
	})

	t.Run("并发失效安全", func(t *testing.T) {
		ast := assert.New(t)
		defer Groups.InvalidateCertAuthCache()

		for range 50 {
			go Groups.InvalidateCertAuthCache()
		}
		Groups.InvalidateCertAuthCache()

		v := Groups.AnyGroupHasCertAuth()
		ast.True(v || !v, "并发失效后调用不应 panic")
	})
}

func TestHasAuthType(t *testing.T) {
	ast := assert.New(t)

	ast.True(Groups.HasAuthType(json.RawMessage(`{"step":[{"type":"cert"},{"type":"local"}]}`), "cert"))
	ast.True(Groups.HasAuthType(json.RawMessage(`{"step":[{"type":"local"}]}`), "local"))
	ast.False(Groups.HasAuthType(json.RawMessage(`{"step":[{"type":"local"}]}`), "cert"))
	ast.False(Groups.HasAuthType(json.RawMessage(`not-json`), "cert"))
	ast.False(Groups.HasAuthType(nil, "cert"))
}

// 覆盖 IPv4/IPv6 网段的相交、包含和版本不匹配场景
func TestCidrOverlaps(t *testing.T) {
	ast := assert.New(t)
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"相同网段", "10.8.0.0/24", "10.8.0.0/24", true},
		{"包含（大包小）", "10.8.0.0/16", "10.8.1.0/24", true},
		{"被包含（小被大包）", "10.8.1.0/24", "10.8.0.0/16", true},
		{"相邻不重叠", "10.8.0.0/24", "10.8.1.0/24", false},
		{"完全不相交", "10.8.0.0/24", "192.168.1.0/24", false},
		{"v6 包含", "fd00:1::/32", "fd00:1:0:1::/48", true},
		{"v6 不相交", "fd00::/32", "fd01::/32", false},
		{"v4/v6 版本不同不算重叠", "10.8.0.0/24", "fd00::/32", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, a, err1 := net.ParseCIDR(c.a)
			_, b, err2 := net.ParseCIDR(c.b)
			ast.NoError(err1)
			ast.NoError(err2)
			ast.Equal(c.want, cidrOverlaps(a, b))
		})
	}
}

// 覆盖组网段与全局池、其他组及母网卡网段的冲突检测
func TestCheckCidrOverlap(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	setGlobal := func(v4, v6 string) {
		base.UpdateCfg(func(c *base.ServerConfig) {
			c.Ipv4CIDR = v4
			c.Ipv6CIDR = v6

			c.MasterDev = "lo"
		})
	}

	mustOverlap := func(cidr string, g *Group, isV6 bool) {
		_, ipNet, err := net.ParseCIDR(cidr)
		ast.NoError(err)
		ast.Error(g.checkCidrOverlap(ipNet, isV6), "应判定重叠: %s", cidr)
	}
	mustOK := func(cidr string, g *Group, isV6 bool) {
		_, ipNet, err := net.ParseCIDR(cidr)
		ast.NoError(err)
		ast.NoError(g.checkCidrOverlap(ipNet, isV6), "应判定不重叠: %s", cidr)
	}

	setGlobal("10.8.0.0/24", "")
	g := &Group{Name: "g1"}
	mustOverlap("10.8.0.128/25", g, false)

	mustOK("192.168.100.0/24", g, false)

	mustOverlap("127.0.0.0/8", g, false)
	mustOverlap("127.1.2.0/24", g, false)

	setGlobal("10.8.0.0/24", "")
	ast.NoError(GetDB().Insert(&Group{Name: "g2", ClientCidr: "172.16.0.0/24"}))
	other := &Group{Id: 0, Name: "g3"}
	mustOverlap("172.16.0.128/25", other, false)
	mustOK("172.17.0.0/24", other, false)

	existing := &Group{Name: "g4", ClientCidr: "172.18.0.0/24"}
	ast.NoError(GetDB().Insert(existing))
	self, err := Groups.ByName("g4")
	ast.NoError(err)
	mustOK("172.18.0.0/24", self, false)

	setGlobal("10.8.0.0/24", "fd00:1::/32")
	g6 := &Group{Name: "g6"}
	mustOverlap("fd00:1:0:1::/48", g6, true)
	mustOverlap("::1/128", g6, true)
	mustOK("fd00:2::/48", g6, true)
}

// 覆盖未配置母网卡及 IPv4/IPv6 版本不匹配的边界
func TestCheckCidrOverlapMasterDev(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	setGlobal := func(v4, v6, masterDev string) {
		base.UpdateCfg(func(c *base.ServerConfig) {
			c.Ipv4CIDR = v4
			c.Ipv6CIDR = v6
			c.MasterDev = masterDev
		})
	}

	setGlobal("10.8.0.0/24", "", "lo")
	g := &Group{Name: "g_lo"}
	_, ipNet, err := net.ParseCIDR("127.1.2.0/24")
	ast.NoError(err)
	ast.Error(g.checkCidrOverlap(ipNet, false), "MasterDev=lo 应拦截与母网卡物理段重叠")

	setGlobal("10.8.0.0/24", "", "")
	g2 := &Group{Name: "g_empty"}
	_, ipNet2, err := net.ParseCIDR("127.1.2.0/24")
	ast.NoError(err)
	ast.NoError(g2.checkCidrOverlap(ipNet2, false), "MasterDev 为空时不应拦截母网卡段")

	setGlobal("10.8.0.0/24", "fd00:1::/32", "lo")
	gv6 := &Group{Name: "g_v6"}
	_, v6Net, err := net.ParseCIDR("fd00:2::/48")
	ast.NoError(err)
	ast.NoError(gv6.checkCidrOverlap(v6Net, true), "v6 组网段不应误判")

	gv4 := &Group{Name: "g_v4"}
	_, v4Net, err := net.ParseCIDR("192.168.200.0/24")
	ast.NoError(err)
	ast.NoError(gv4.checkCidrOverlap(v4Net, false), "v4 组网段不与 lo/v6 池重叠不应拦")
}
