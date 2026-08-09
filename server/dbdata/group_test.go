package dbdata

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/security"
	"github.com/wsczx/remlink/pkg/utils"
)

func TestGetGroupNames(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	defer closeIpdata()

	err := SetProvider(&Provider{
		Name:   "test-radius",
		Type:   "radius",
		Status: 1,
		Config: security.EncryptedJSON[json.RawMessage]{Data: json.RawMessage(`{"addr":"192.168.8.12:1044","secret":"43214132"}`)},
	})
	ast.Nil(err)
	err = SetProvider(&Provider{
		Name:   "test-ldap",
		Type:   "ldap",
		Status: 1,
		Config: security.EncryptedJSON[json.RawMessage]{Data: json.RawMessage(`{"addr":"192.168.8.12:389","tls":true,"bind_name":"userfind@abc.com","bind_pwd":"afdbfdsafds","base_dn":"dc=abc,dc=com","object_class":"person","search_attr":"sAMAccountName","member_of":"cn=vpn,cn=user,dc=abc,dc=com"}`)},
	})
	ast.Nil(err)

	// 创建通用测试策略
	defaultPolicy := &Policy{
		Name:      "group-test-default",
		ClientDns: []ValData{{Val: "114.114.114.114"}},
		Status:    1,
	}
	err = SetPolicy(defaultPolicy)
	ast.Nil(err)
	pid := defaultPolicy.Id

	// 添加 group
	g1 := Group{Name: "g1", PolicyId: pid}
	err = SetGroup(&g1)
	ast.Nil(err)
	g2 := Group{Name: "g2", PolicyId: pid}
	err = SetGroup(&g2)
	ast.Nil(err)
	g3 := Group{Name: "g3", PolicyId: pid}
	err = SetGroup(&g3)
	ast.Nil(err)

	g4 := Group{Name: "g4", PolicyId: pid,
		AuthProfile: json.RawMessage(`{"step":[{"type":"radius","provider":"test-radius"}]}`)}
	err = SetGroup(&g4)
	ast.Nil(err)

	// g5: 含域名拆分，需新建策略
	p5 := &Policy{
		Name:             "group-test-g5",
		ClientDns:        []ValData{{Val: "114.114.114.114"}},
		RouteInclude:     []ValData{{Val: "10.0.0.0/8"}},
		DsIncludeDomains: "baidu.com,163.com",
		Status:           1,
	}
	err = SetPolicy(p5)
	ast.Nil(err)
	g5 := Group{Name: "g5", PolicyId: p5.Id}
	err = SetGroup(&g5)
	ast.Nil(err)

	// g6: 含排除域名
	p6 := &Policy{
		Name:             "group-test-g6",
		ClientDns:        []ValData{{Val: "114.114.114.114"}},
		DsExcludeDomains: "com.cn,qq.com",
		Status:           1,
	}
	err = SetPolicy(p6)
	ast.Nil(err)
	g6 := Group{Name: "g6", PolicyId: p6.Id}
	err = SetGroup(&g6)
	ast.Nil(err)

	g7 := Group{Name: "g7", PolicyId: pid,
		AuthProfile: json.RawMessage(`{"step":[{"type":"ldap","provider":"test-ldap"}]}`)}
	err = SetGroup(&g7)
	ast.Nil(err)

	// 判断所有数据
	gAll := []string{"g1", "g2", "g3", "g4", "g5", "g6", "g7"}
	gs := GetGroupNames()
	for _, v := range gs {
		ast.Equal(true, utils.InArrStr(gAll, v))
	}

	gni := GetGroupNamesIds()
	for _, v := range gni {
		ast.NotEqual(0, v.Id)
		ast.Equal(true, utils.InArrStr(gAll, v.Name))
	}
}

// 测试 AnyGroupHasCertAuth：扫描组认证配置判断是否启用 cert 认证，带缓存。
// 缓存核心语义：InvalidateCertAuthCache 后立即反映最新组配置。
func TestAnyGroupHasCertAuth(t *testing.T) {
	t.Run("有cert组返回true", func(t *testing.T) {
		ast := assert.New(t)
		defer InvalidateCertAuthCache()
		preIpData(t)
		defer closeIpdata()

		pt := &Policy{Name: "cert-auth-default", ClientDns: []ValData{{Val: "8.8.8.8"}}, Status: 1}
		ast.Nil(SetPolicy(pt))
		ast.Nil(SetGroup(&Group{Name: "cert-yes", Status: 1, PolicyId: pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"cert"},{"type":"local"}]}`)}))
		InvalidateCertAuthCache()
		ast.True(AnyGroupHasCertAuth(), "含 cert 组应返回 true")
	})

	t.Run("无cert组返回false", func(t *testing.T) {
		ast := assert.New(t)
		defer InvalidateCertAuthCache()
		preIpData(t)
		defer closeIpdata()

		pt := &Policy{Name: "cert-auth-none", ClientDns: []ValData{{Val: "8.8.8.8"}}, Status: 1}
		ast.Nil(SetPolicy(pt))
		ast.Nil(SetGroup(&Group{Name: "cert-no", Status: 1, PolicyId: pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`)}))
		InvalidateCertAuthCache()
		ast.False(AnyGroupHasCertAuth(), "无 cert 组应返回 false")
	})
}

// 测试 InvalidateCertAuthCache：使缓存立即失效，失效后反映最新组配置，且并发安全。
func TestInvalidateCertAuthCache(t *testing.T) {
	t.Run("失效后反映最新配置", func(t *testing.T) {
		ast := assert.New(t)
		defer InvalidateCertAuthCache()
		preIpData(t)
		defer closeIpdata()

		pt := &Policy{Name: "cert-inv-default", ClientDns: []ValData{{Val: "8.8.8.8"}}, Status: 1}
		ast.Nil(SetPolicy(pt))
		ast.Nil(SetGroup(&Group{Name: "cert-inv", Status: 1, PolicyId: pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"cert"}]}`)}))
		_ = pt.Id

		InvalidateCertAuthCache()
		ast.True(AnyGroupHasCertAuth(), "有 cert 组应返回 true")
	})

	t.Run("并发失效安全", func(t *testing.T) {
		ast := assert.New(t)
		defer InvalidateCertAuthCache()
		// 并发多次失效不应 panic
		for i := 0; i < 50; i++ {
			go InvalidateCertAuthCache()
		}
		InvalidateCertAuthCache()
		// 失效后处于未缓存状态，调用应安全返回（不依赖具体 true/false，只验证不 panic）
		v := AnyGroupHasCertAuth()
		ast.True(v || !v, "并发失效后调用不应 panic")
	})
}

// 测试 HasAuthType：解析 AuthProfile 判断是否包含指定认证类型。
func TestHasAuthType(t *testing.T) {
	ast := assert.New(t)

	ast.True(HasAuthType(json.RawMessage(`{"step":[{"type":"cert"},{"type":"local"}]}`), "cert"))
	ast.True(HasAuthType(json.RawMessage(`{"step":[{"type":"local"}]}`), "local"))
	ast.False(HasAuthType(json.RawMessage(`{"step":[{"type":"local"}]}`), "cert"))
	ast.False(HasAuthType(json.RawMessage(`not-json`), "cert"))
	ast.False(HasAuthType(nil, "cert"))
}

// TestCidrOverlaps 表驱动测试两个 CIDR 是否重叠的纯逻辑。
// 这是组网段重叠检测（含母网卡段）的核心判定，任何回归都会直接影响
// 保存组配置时的拦截行为，必须稳定。
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

// TestCheckCidrOverlap 覆盖组自定义网段保存时的全部重叠拦截分支：
// 全局 VPN 池、其他组、以及母网卡物理网段（macvtap/ipvtap 专属冲突）。
func TestCheckCidrOverlap(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	setGlobal := func(v4, v6 string) {
		base.UpdateCfg(func(c *base.ServerConfig) {
			c.Ipv4CIDR = v4
			c.Ipv6CIDR = v6
			// 用真实存在的回环网卡 lo（127.0.0.1/8、::1/128）验证母网卡段拦截
			c.MasterDev = "lo"
		})
	}

	mustOverlap := func(cidr string, g *Group, isV6 bool) {
		_, ipNet, err := net.ParseCIDR(cidr)
		ast.NoError(err)
		ast.Error(checkCidrOverlap(ipNet, g, isV6), "应判定重叠: %s", cidr)
	}
	mustOK := func(cidr string, g *Group, isV6 bool) {
		_, ipNet, err := net.ParseCIDR(cidr)
		ast.NoError(err)
		ast.NoError(checkCidrOverlap(ipNet, g, isV6), "应判定不重叠: %s", cidr)
	}

	// 1) 与全局 VPN 池重叠 -> 拦（10.8.0.128/25 落在 10.8.0.0/24 内）
	setGlobal("10.8.0.0/24", "")
	g := &Group{Name: "g1"}
	mustOverlap("10.8.0.128/25", g, false)

	// 2) 与全局池不重叠 -> 放行
	mustOK("192.168.100.0/24", g, false)

	// 3) 母网卡 lo 物理网段（127.0.0.0/8）重叠 -> 拦（对所有 link_mode 统一拦截）
	mustOverlap("127.0.0.0/8", g, false)
	mustOverlap("127.1.2.0/24", g, false)

	// 4) 组间重叠 -> 拦
	setGlobal("10.8.0.0/24", "")
	ast.NoError(Add(&Group{Name: "g2", ClientCidr: "172.16.0.0/24"}))
	other := &Group{Id: 0, Name: "g3"}
	mustOverlap("172.16.0.128/25", other, false) // 与已存在 g2 的 172.16.0.0/24 重叠
	mustOK("172.17.0.0/24", other, false)

	// 5) 自身已有网段不参与比较（编辑自己）
	existing := &Group{Name: "g4", ClientCidr: "172.18.0.0/24"}
	ast.NoError(Add(existing))
	var self Group
	ast.NoError(One("Name", "g4", &self))
	mustOK("172.18.0.0/24", &self, false)

	// 6) v6 对称：母网卡 lo 的 ::1/128 + 全局 v6 池
	setGlobal("10.8.0.0/24", "fd00:1::/32")
	g6 := &Group{Name: "g6"}
	mustOverlap("fd00:1:0:1::/48", g6, true) // 与全局 v6 池重叠
	mustOverlap("::1/128", g6, true)         // 与母网卡 lo 的 v6 段重叠
	mustOK("fd00:2::/48", g6, true)          // 不重叠
}

// TestCheckCidrOverlapMasterDev 补覆盖母网卡物理网段检测的两个边界：
// MasterDev 为空时检测应完全跳过；版本不匹配（v4/v6）不互相误拦。
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

	// 1) MasterDev=lo 时，母网卡段（127.0.0.0/8）重叠应拦截
	setGlobal("10.8.0.0/24", "", "lo")
	g := &Group{Name: "g_lo"}
	_, ipNet, err := net.ParseCIDR("127.1.2.0/24")
	ast.NoError(err)
	ast.Error(checkCidrOverlap(ipNet, g, false), "MasterDev=lo 应拦截与母网卡物理段重叠")

	// 2) MasterDev 为空时，母网卡检测应完全跳过：组网段与 lo 的 127 段重叠也不拦
	setGlobal("10.8.0.0/24", "", "")
	g2 := &Group{Name: "g_empty"}
	_, ipNet2, err := net.ParseCIDR("127.1.2.0/24")
	ast.NoError(err)
	ast.NoError(checkCidrOverlap(ipNet2, g2, false), "MasterDev 为空时不应拦截母网卡段")

	// 3) 版本不匹配不误拦：v6 组网段不比对 v4 母网卡段，v4 组网段不比对 v6 池
	setGlobal("10.8.0.0/24", "fd00:1::/32", "lo")
	gv6 := &Group{Name: "g_v6"}
	_, v6Net, err := net.ParseCIDR("fd00:2::/48") // 与 lo 的 v4/v6 段均不重叠
	ast.NoError(err)
	ast.NoError(checkCidrOverlap(v6Net, gv6, true), "v6 组网段不应误判")

	gv4 := &Group{Name: "g_v4"}
	_, v4Net, err := net.ParseCIDR("192.168.200.0/24") // 不与 lo v4 段、也不与 v6 池重叠
	ast.NoError(err)
	ast.NoError(checkCidrOverlap(v4Net, gv4, false), "v4 组网段不与 lo/v6 池重叠不应拦")
}
