package sessdata

import (
	"fmt"
	"math/big"
	"net"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

func preData(tmpDir string) {
	// 注册认证器
	for _, name := range []string{"local", "ldap", "radius", "otp", "cert", "saml", "wxwork", "feishu"} {
		if !auth.Registry.IsRegistered(name) {
			n := name
			auth.Registry.Register(n, func() auth.Authenticator { return &testAuthStub{name: n} })
		}
	}

	base.Test()
	tmpDb := path.Join(tmpDir, "test.db")
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.DbType = "sqlite3"
		c.DbSource = tmpDb
		c.Ipv4CIDR = "192.168.3.0/24"
		c.Ipv4Gateway = "192.168.3.1"
		c.Ipv4Start = "192.168.3.100"
		c.Ipv4End = "192.168.3.150"
		c.Ipv6CIDR = "2001:db8:3::/120"
		c.MaxClient = 100
		c.MaxUserClient = 3
		c.IpLease = 5
	})

	dbdata.Start()
	p := dbdata.Policy{
		Name:      "ip-pool-test-policy",
		Status:    1,
		Bandwidth: 1000,
		ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}},
	}
	_ = dbdata.Add(&p)
	group := dbdata.Group{
		Name:     "group1",
		PolicyId: p.Id,
	}
	_ = dbdata.Add(&group)

	user := dbdata.User{
		Username: "user-test",
		Mtu:      1000,
	}
	_ = dbdata.Add(&user)
	if err := initIpPool(); err != nil {
		panic(err)
	}
}

func cleardata(tmpDir string) {
	_ = dbdata.Stop()
	tmpDb := path.Join(tmpDir, "test.db")
	os.Remove(tmpDb)
}

func TestIpPool(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)

	var ip net.IP

	for i := 100; i <= 150; i++ {
		_ = AcquireIp(getTestUser(i), getTestMacAddr(i), true)
	}

	// 回收
	ReleaseIp(net.IPv4(192, 168, 3, 140), nil, getTestMacAddr(140))
	time.Sleep(time.Second * 6)

	// 从头循环获取可用ip
	user_new := getTestUser(210)
	mac_new := getTestMacAddr(210)
	ip = AcquireIp(user_new, mac_new, true)
	t.Log("mac_new", ip)
	assert.NotNil(ip)
	assert.True(net.IPv4(192, 168, 3, 140).Equal(ip))

	// 回收全部
	for i := 100; i <= 150; i++ {
		ReleaseIp(net.IPv4(192, 168, 3, byte(i)), nil, getTestMacAddr(i))
	}
}

func TestIpv6Pool(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)

	// 重置 v6 池游标与活跃表，保证干净起步
	ipActive = map[string]bool{}
	_ = initIpPool()

	// 分配若干 v6 地址
	const n = 50
	var ips []net.IP
	for i := range n {
		ip := acquireIpV6(getTestUser(3000+i), getTestMacAddr(3000+i), true, IpPool)
		assert.NotNil(ip)
		assert.Equal(16, len(ip)) // 128 位
		ips = append(ips, ip)
	}

	// 全部应在 Ipv6CIDR(2001:db8:3::/120) 网段内
	_, v6Net, _ := net.ParseCIDR("2001:db8:3::/120")
	for _, ip := range ips {
		assert.True(v6Net.Contains(ip), "v6 地址不在池网段内: %s", ip)
	}

	// 释放后过租期再次分配应仍在池内（轮询游标继续，半满池不保证返回同一地址；回绕后才复用）
	ReleaseIp(nil, ips[0], getTestMacAddr(3000))
	time.Sleep(time.Second * 6)
	ip2 := acquireIpV6(getTestUser(3999), getTestMacAddr(3999), true, IpPool)
	assert.NotNil(ip2)
	assert.True(v6Net.Contains(ip2), "复用分配的 v6 地址应在池网段内: %s", ip2)

	// 回收全部，避免影响其他用例
	for i := range n {
		ReleaseIp(nil, ips[i], getTestMacAddr(3000+i))
	}
	ReleaseIp(nil, ip2, getTestMacAddr(3999))
}

// 启用 IPv6 时若配置 MTU<1280，initIpPool 应自动上调到 1280
func TestIpv6MtuFloor(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)

	base.GetCfg().Mtu = 1000
	assert.NoError(initIpPool())
	assert.Equal(1280, base.GetCfg().Mtu, "启用 IPv6 且 MTU<1280 时应被自动上调到 1280")
}
func TestGroupIpPoolIsolation(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)

	// 清理跨用例可能残留的状态，保证干净起步
	ipActive = map[string]bool{}
	groupPoolCache = map[string]*ipPoolConfig{}

	// 组B范围数值上高于组A，才能暴露旧的“共享游标”bug
	groupA := &dbdata.Group{
		Name:          "groupA",
		ClientCidr:    "10.0.1.0/24",
		ClientStart:   "10.0.1.100",
		ClientEnd:     "10.0.1.150",
		ClientGateway: "10.0.1.1",
	}
	groupB := &dbdata.Group{
		Name:          "groupB",
		ClientCidr:    "10.0.2.0/24",
		ClientStart:   "10.0.2.100",
		ClientEnd:     "10.0.2.150",
		ClientGateway: "10.0.2.1",
	}

	poolA := GetGroupIpPool(groupA)
	poolB := GetGroupIpPool(groupB)

	// 缓存复用：同一组配置必须返回同一实例（游标才能累积）
	poolA2 := GetGroupIpPool(groupA)
	assert.Same(poolA, poolA2)

	// 池配置断言
	assert.Equal(utils.Ip2long(net.ParseIP("10.0.1.100")), poolA.IpLongMin)
	assert.Equal(utils.Ip2long(net.ParseIP("10.0.1.150")), poolA.IpLongMax)
	assert.Equal(utils.Ip2long(net.ParseIP("10.0.2.100")), poolB.IpLongMin)
	assert.Equal(utils.Ip2long(net.ParseIP("10.0.2.150")), poolB.IpLongMax)

	// 确定性顺序：先把组A灌若干个，再灌组B
	var ipsA, ipsB []net.IP
	for i := range 10 {
		ip := AcquireIpWithRange(getTestUser(1000+i), getTestMacAddr(1000+i), true, poolA)
		assert.NotNil(ip)
		ipsA = append(ipsA, ip)
	}
	for i := range 10 {
		ip := AcquireIpWithRange(getTestUser(2000+i), getTestMacAddr(2000+i), true, poolB)
		assert.NotNil(ip)
		ipsB = append(ipsB, ip)
	}

	// 断言：组A的IP必须全部在 10.0.1.100-150 范围内
	for _, ip := range ipsA {
		ipLong := utils.Ip2long(ip)
		assert.True(ipLong >= poolA.IpLongMin && ipLong <= poolA.IpLongMax,
			"组A分配了超出范围的IP: %s (min=%d max=%d)", ip, poolA.IpLongMin, poolA.IpLongMax)
	}
	// 断言：组B的IP必须全部在 10.0.2.100-150 范围内
	for _, ip := range ipsB {
		ipLong := utils.Ip2long(ip)
		assert.True(ipLong >= poolB.IpLongMin && ipLong <= poolB.IpLongMax,
			"组B分配了超出范围的IP: %s (min=%d max=%d)", ip, poolB.IpLongMin, poolB.IpLongMax)
	}

	t.Logf("组A分配了 %d 个IP, 组B分配了 %d 个IP", len(ipsA), len(ipsB))
}

func getTestUser(i int) string {
	return fmt.Sprintf("user-%d", i)
}

// 用给定 v4 段构造一个组级 IP 池
func groupPool(cidr, start, end, gw string) *ipPoolConfig {
	g := &dbdata.Group{
		Name:          "g_" + cidr,
		ClientCidr:    cidr,
		ClientStart:   start,
		ClientEnd:     end,
		ClientGateway: gw,
	}
	return GetGroupIpPool(g)
}

func rowsByMac(t *testing.T, mac string) []dbdata.IpMap {
	var rows []dbdata.IpMap
	if err := dbdata.FindWhere(&rows, 0, 0, "mac_addr=?", mac); err != nil {
		t.Fatalf("查询 mac=%s 失败: %v", mac, err)
	}
	return rows
}

func rowByIP(t *testing.T, ip net.IP) *dbdata.IpMap {
	m := &dbdata.IpMap{}
	if err := dbdata.One("ip_addr", ip.String(), m); err != nil {
		t.Fatalf("查询 ip=%s 失败: %v", ip, err)
	}
	return m
}

func rowByGroupMac(t *testing.T, mac, group string) *dbdata.IpMap {
	var rows []dbdata.IpMap
	_ = dbdata.FindWhere(&rows, 0, 0, "mac_addr=? AND ip_group=?", mac, group)
	if len(rows) == 0 {
		t.Fatalf("未找到 mac=%s group=%s 的绑定行", mac, group)
	}
	return &rows[0]
}

// 同 MAC 在不同组应各自保留独立绑定，重连同组保持稳定、不互相覆盖
func TestIpBindingCrossGroupMultiBind(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)
	ipActive = map[string]bool{}
	groupPoolCache = map[string]*ipPoolConfig{}

	poolA := groupPool("10.0.1.0/24", "10.0.1.100", "10.0.1.150", "10.0.1.1")
	poolB := groupPool("10.0.2.0/24", "10.0.2.100", "10.0.2.150", "10.0.2.1")
	mac := getTestMacAddr(7777)
	u := getTestUser(7777)

	ipA := AcquireIpWithRange(u, mac, true, poolA)
	assert.NotNil(ipA)
	assert.True(ipInPool(ipA, poolA))
	ipB := AcquireIpWithRange(u, mac, true, poolB)
	assert.NotNil(ipB)
	assert.True(ipInPool(ipB, poolB))
	assert.False(ipA.Equal(ipB), "同MAC跨组应分配不同IP")

	rows := rowsByMac(t, mac)
	assert.Len(rows, 2)
	groups := map[string]string{}
	for _, r := range rows {
		groups[r.Group] = r.IpAddr
	}
	assert.Equal(ipA.String(), groups["g_10.0.1.0/24"])
	assert.Equal(ipB.String(), groups["g_10.0.2.0/24"])

	// 重连组A应保持稳定（先释放旧会话，模拟正常重连）
	ReleaseIp(ipA, nil, mac)
	ipA2 := AcquireIpWithRange(u, mac, true, poolA)
	assert.True(ipA.Equal(ipA2))
	assert.Len(rowsByMac(t, mac), 2)
}

func TestIpBindingLegacyRowCleanedOnCustomGroup(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)
	ipActive = map[string]bool{}
	groupPoolCache = map[string]*ipPoolConfig{}

	mac := "02:00:00:00:00:99"
	legacyIP := "192.168.3.120"
	if err := dbdata.Add(&dbdata.IpMap{IpAddr: legacyIP, MacAddr: mac, UniqueMac: true, Username: "legacy", LastLogin: time.Now(), Group: ""}); err != nil {
		t.Fatal(err)
	}

	poolC := groupPool("10.0.3.0/24", "10.0.3.100", "10.0.3.150", "10.0.3.1")
	ipC := AcquireIpWithRange(getTestUser(8800), mac, true, poolC)
	assert.NotNil(ipC)
	assert.True(ipInPool(ipC, poolC))
	rows := rowsByMac(t, mac)
	assert.Len(rows, 1, "遗留全局绑定应被单一主键删除、由组C新建行取代")
	assert.Equal("g_10.0.3.0/24", rows[0].Group)
	assert.Equal(ipC.String(), rows[0].IpAddr)
}

// 同 MAC 在两组各有一行后，连接新组时不应误删已有组绑定（不丢行）
func TestIpBindingCrossGroupNoCollateralDelete(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)
	ipActive = map[string]bool{}
	groupPoolCache = map[string]*ipPoolConfig{}

	poolA := groupPool("10.0.11.0/24", "10.0.11.100", "10.0.11.150", "10.0.11.1")
	poolB := groupPool("10.0.12.0/24", "10.0.12.100", "10.0.12.150", "10.0.12.1")
	poolC := groupPool("10.0.13.0/24", "10.0.13.100", "10.0.13.150", "10.0.13.1")
	mac := getTestMacAddr(7788)
	u := getTestUser(7788)

	_ = AcquireIpWithRange(u, mac, true, poolA)
	_ = AcquireIpWithRange(u, mac, true, poolB)
	assert.Len(rowsByMac(t, mac), 2, "两组应各有一行")
	// 再连第三组：应新增一行，前两组的行必须保留
	_ = AcquireIpWithRange(u, mac, true, poolC)
	assert.Len(rowsByMac(t, mac), 3, "连接新组不应误删已有组绑定")
}

// 池耗尽复用最早登录 IP 时，DB 行必须更新为当前客户端，且不能丢失已分配的 v6 地址
func TestLoopFarIpReusePersistsAndKeepsV6(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)
	ipActive = map[string]bool{}
	groupPoolCache = map[string]*ipPoolConfig{}

	// 仅 2 个 IP 的组池，便于耗尽触发最早登录复用
	pool := groupPool("10.0.7.0/24", "10.0.7.100", "10.0.7.101", "10.0.7.1")
	mac1 := getTestMacAddr(9101)
	mac2 := getTestMacAddr(9102)
	mac3 := getTestMacAddr(9103)
	u1, u2, u3 := getTestUser(9101), getTestUser(9102), getTestUser(9103)

	ip1 := AcquireIpWithRange(u1, mac1, true, pool)
	ip2 := AcquireIpWithRange(u2, mac2, true, pool)
	assert.True(ip1.Equal(net.ParseIP("10.0.7.100")))
	assert.True(ip2.Equal(net.ParseIP("10.0.7.101")))

	// 之后把两行的 LastLogin 设为「租期内(未过期)、但有序」的确定值：mac1 更早、mac2 稍晚，
	// 这样才能稳定走到 loopFarIp 的「最早登录」复用分支（若落在租期外会走过期复用、先碰谁用谁）。
	ReleaseIp(ip1, nil, mac1)
	ReleaseIp(ip2, nil, mac2)
	row1 := rowByIP(t, ip1)
	row1.IpAddr6 = "2001:db8:3::abc"
	row1.LastLogin = time.Now().Add(-3 * time.Second)
	assert.NoError(dbdata.Set(row1))
	row2 := rowByIP(t, ip2)
	row2.LastLogin = time.Now().Add(-1 * time.Second)
	assert.NoError(dbdata.Set(row2))

	// 池已耗尽，第三次分配应复用最早登录(10.0.7.100)那一行
	ip3 := AcquireIpWithRange(u3, mac3, true, pool)
	assert.NotNil(ip3)
	assert.True(ip3.Equal(net.ParseIP("10.0.7.100")), "池耗尽应复用最早登录IP, 实际=%s", ip3)

	// DB 行必须已更新为 mac3（验证修复：原代码 Id=0 导致永不落库）
	rowReused := rowByIP(t, ip3)
	assert.Equal(mac3, rowReused.MacAddr, "复用行的 mac 应更新为新客户端")
	assert.Equal(u3, rowReused.Username)
	// v6 地址不应被抹掉
	assert.Equal("2001:db8:3::abc", rowReused.IpAddr6, "复用行不应丢失原 v6 地址")
}

// 非 uniqueMac（按用户名）也应按组各自留绑定
func TestNonUniqueMacPerGroupBinding(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)
	ipActive = map[string]bool{}
	groupPoolCache = map[string]*ipPoolConfig{}

	poolA := groupPool("10.0.4.0/24", "10.0.4.100", "10.0.4.150", "10.0.4.1")
	poolB := groupPool("10.0.5.0/24", "10.0.5.100", "10.0.5.150", "10.0.5.1")
	macX := getTestMacAddr(9200)
	macY := getTestMacAddr(9201)
	u := getTestUser(9299) // 同一用户名，不同 mac

	ipA := AcquireIpWithRange(u, macX, false, poolA)
	ipB := AcquireIpWithRange(u, macY, false, poolB)
	assert.NotNil(ipA)
	assert.NotNil(ipB)
	assert.True(ipInPool(ipA, poolA))
	assert.True(ipInPool(ipB, poolB))

	rows := []dbdata.IpMap{}
	_ = dbdata.FindWhere(&rows, 0, 0, "username=?", u)
	assert.Len(rows, 2, "同一用户名在两组应各留一行")

	// 重连组A（同一用户名+同一 mac）应稳定复用（先释放旧会话）
	ReleaseIp(ipA, nil, macX)
	ipA2 := AcquireIpWithRange(u, macX, false, poolA)
	assert.True(ipA.Equal(ipA2))
}

// 非 uniqueMac 崩溃重连（未 Release、IP 仍活跃）应复用同设备地址，而非返回 nil 或撞复合唯一
func TestNonUniqueMacCrashReconnect(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)
	ipActive = map[string]bool{}
	groupPoolCache = map[string]*ipPoolConfig{}

	poolA := groupPool("10.0.41.0/24", "10.0.41.100", "10.0.41.150", "10.0.41.1")
	macX := getTestMacAddr(9250)
	u := getTestUser(9250)

	ipA := AcquireIpWithRange(u, macX, false, poolA)
	assert.NotNil(ipA)
	assert.True(ipActive[ipA.String()])
	// 同设备重连（同用户名+同 mac），应直接复用，不能返回 nil/撞唯一
	ipA2 := AcquireIpWithRange(u, macX, false, poolA)
	assert.NotNil(ipA2, "崩溃重连不应返回 nil")
	assert.True(ipA.Equal(ipA2), "同设备重连应复用同一地址")
	rows := rowsByMac(t, macX)
	assert.Len(rows, 1, "崩溃重连不应新增重复行")
}

// 跨组应分配不同的 v6 地址，且各自记在对应组的行；单组 v6 释放不影响另一组
func TestGroupV6CrossGroupDistinct(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)
	ipActive = map[string]bool{}
	groupPoolCache = map[string]*ipPoolConfig{}

	poolA := groupPool("10.0.6.0/24", "10.0.6.100", "10.0.6.150", "10.0.6.1")
	poolB := groupPool("10.0.66.0/24", "10.0.66.100", "10.0.66.150", "10.0.66.1")
	mac := getTestMacAddr(9300)
	u := getTestUser(9399)

	_ = AcquireIpWithRange(u, mac, true, poolA)
	ip6A := acquireIpV6(u, mac, true, poolA)
	assert.NotNil(ip6A)
	_, v6Net, _ := net.ParseCIDR("2001:db8:3::/120")
	assert.True(v6Net.Contains(ip6A), "组v6应回退全局池: %s", ip6A)

	_ = AcquireIpWithRange(u, mac, true, poolB)
	ip6B := acquireIpV6(u, mac, true, poolB)
	assert.NotNil(ip6B)
	assert.True(v6Net.Contains(ip6B))
	assert.False(ip6A.Equal(ip6B), "跨组应分配不同 v6 地址")

	rows := rowsByMac(t, mac)
	assert.Len(rows, 2)
	v6ByGroup := map[string]string{}
	for _, r := range rows {
		v6ByGroup[r.Group] = r.IpAddr6
	}
	assert.Equal(ip6A.String(), v6ByGroup["g_10.0.6.0/24"])
	assert.Equal(ip6B.String(), v6ByGroup["g_10.0.66.0/24"])

	// 仅释放组A的 v6，组B的应保留
	ReleaseIp(nil, ip6A, mac)
	assert.Empty(rowByGroupMac(t, mac, "g_10.0.6.0/24").IpAddr6, "释放组A v6 后该组行 IpAddr6 应清空")
	assert.Equal(ip6B.String(), rowByGroupMac(t, mac, "g_10.0.66.0/24").IpAddr6)
	ReleaseIp(nil, ip6B, mac)
}

// 未配置自定义段的组应回退全局池，GroupName 为空，绑定落在全局段
func TestGlobalPoolGroupNameEmpty(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)
	ipActive = map[string]bool{}
	groupPoolCache = map[string]*ipPoolConfig{}

	g := &dbdata.Group{Name: "noCustom"}
	p := GetGroupIpPool(g)
	assert.Equal(IpPool, p)
	assert.Equal("", p.GroupName)
	ip := AcquireIpWithRange(getTestUser(9500), getTestMacAddr(9500), true, p)
	assert.NotNil(ip)
	_, gNet, _ := net.ParseCIDR("192.168.3.0/24")
	assert.True(gNet.Contains(ip))
	rows := rowsByMac(t, getTestMacAddr(9500))
	assert.Len(rows, 1)
	assert.Equal("", rows[0].Group)
}

// 释放 v4 后 ipActive 应清除
func TestReleaseIpV4(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)
	ipActive = map[string]bool{}
	groupPoolCache = map[string]*ipPoolConfig{}

	mac := getTestMacAddr(9400)
	ip := AcquireIp(getTestUser(9400), mac, true)
	assert.NotNil(ip)
	assert.True(ipActive[ip.String()])
	ReleaseIp(ip, nil, mac)
	assert.False(ipActive[ip.String()], "释放后 ipActive 应清除")
}

func getTestMacAddr(i int) string {
	// 前缀mac
	macAddr := "02:00:00:00:00"
	return fmt.Sprintf("%s:%x", macAddr, i)
}

// testAuthStub 无状态认证桩，避免循环依赖 auth/authsrv
type testAuthStub struct {
	name string
}

func (a *testAuthStub) Name() string { return a.name }
func (a *testAuthStub) Authenticate(*auth.Context) (auth.StepResult, error) {
	return auth.StepPass, nil
}

// ========== 组级 v6 池（V2） ==========

func TestInitV6_GatewayAndBounds(t *testing.T) {
	p := &ipPoolConfig{}
	assert.Nil(t, p.initV6("2001:db8:5::/120"))
	// gw = 网络地址 + 1
	assert.True(t, p.Ipv6Gateway.Equal(net.ParseIP("2001:db8:5::1")))
	// start = 网络地址 + 2
	assert.Equal(t, 0, p.ipv6Start.Cmp(ipToBig(net.ParseIP("2001:db8:5::2"))))
	// end = 网段末地址
	_, net120, _ := net.ParseCIDR("2001:db8:5::/120")
	end := new(big.Int).Add(ipToBig(net120.IP.Mask(net120.Mask)),
		new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(128-120)), big.NewInt(1)))
	assert.Equal(t, 0, p.ipv6End.Cmp(end))
	// 前缀 = 128 无分配空间，应报错
	assert.Error(t, p.initV6("2001:db8:6::/128"))
	// v4 CIDR 应报错
	assert.Error(t, p.initV6("10.0.0.0/24"))
}

func TestGroupV6Pool_Isolated(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)
	ipActive = map[string]bool{}
	groupPoolCache = map[string]*ipPoolConfig{}

	g := &dbdata.Group{
		Name:          "v6group",
		ClientCidr:    "10.0.9.0/24",
		ClientStart:   "10.0.9.100",
		ClientEnd:     "10.0.9.150",
		ClientGateway: "10.0.9.1",
		ClientCidr6:   "2001:db8:9::/120",
	}
	p := GetGroupIpPool(g)
	assert.NotNil(p.Ipv6IPNet)
	assert.True(p.Ipv6IPNet.Contains(net.ParseIP("2001:db8:9::5")))
	// 组级 v6 网关优先
	assert.True(p.Ipv6Gateway.Equal(net.ParseIP("2001:db8:9::1")))
	// 不回退全局网关
	assert.False(p.V6Gateway().Equal(IpPool.Ipv6Gateway))
}

func TestGroupV6Pool_FallbackGateway(t *testing.T) {
	assert := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)
	ipActive = map[string]bool{}
	groupPoolCache = map[string]*ipPoolConfig{}

	g := &dbdata.Group{
		Name:          "nov6group",
		ClientCidr:    "10.0.8.0/24",
		ClientStart:   "10.0.8.100",
		ClientEnd:     "10.0.8.150",
		ClientGateway: "10.0.8.1",
	}
	p := GetGroupIpPool(g)
	assert.Nil(p.Ipv6IPNet) // 组池本身无 v6 字段
	// V6Gateway 回退全局池
	assert.True(p.V6Gateway().Equal(IpPool.Ipv6Gateway))
	// 用该组池分配 v6 应回退全局池
	ip := acquireIpV6("u-fb", "mac-fb", true, p)
	assert.NotNil(ip)
	assert.True(IpPool.Ipv6IPNet.Contains(ip), "v6 应回退全局池分配: %s", ip)
	ReleaseIp(nil, ip, "mac-fb")
}
