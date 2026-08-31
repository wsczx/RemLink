package sessdata

import (
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
)

func TestMain(m *testing.M) {
	base.Test()
	os.Exit(m.Run())
}

// 用于测试，不实际操作内核防火墙
type mockFirewall struct {
	mu           sync.Mutex
	natRules     map[string]string // fakeIP -> realIP
	addFail      bool
	delFail      bool
	addCallCount int
	delCallCount int
}

func newMockFirewall() *mockFirewall {
	return &mockFirewall{natRules: make(map[string]string)}
}

func (m *mockFirewall) CreateChains(vpnCIDR, fakeIPRange string) error                   { return nil }
func (m *mockFirewall) SetupGlobalNAT(vpnCIDR, masterDev string, inContainer bool) error { return nil }
func (m *mockFirewall) SetupGlobalNAT6(vpnCIDR6, masterDev string, inContainer bool) error {
	return nil
}
func (m *mockFirewall) AddGroupNAT(groupCIDR, masterDev string, inContainer bool) error { return nil }
func (m *mockFirewall) AddGroupNAT6(groupCIDR6, masterDev string, inContainer bool) error {
	return nil
}
func (m *mockFirewall) DelGroupNAT(groupCIDR, masterDev string, inContainer bool) error {
	return nil
}
func (m *mockFirewall) DelGroupNAT6(groupCIDR6, masterDev string, inContainer bool) error {
	return nil
}
func (m *mockFirewall) CleanupFakeIP() error { return nil }
func (m *mockFirewall) CleanupGlobal() error { return nil }

func (m *mockFirewall) AddNatRule(fakeIP, realIP string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addCallCount++
	if m.addFail {
		return errMockFail
	}
	m.natRules[fakeIP] = realIP
	return nil
}

func (m *mockFirewall) DelNatRule(fakeIP, realIP string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delCallCount++
	if m.delFail {
		return errMockFail
	}
	delete(m.natRules, fakeIP)
	return nil
}

var errMockFail = &mockError{"mock firewall error"}

type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }

func newTestManager(t *testing.T) *FakeDNSManager {
	m := &FakeDNSManager{
		active:       make(map[string]*fakeIPEntry),
		activeV6:     make(map[string]*fakeIPEntry),
		domainToIP:   make(map[string]string),
		domainToIPV6: make(map[string]string),
		ipToRealIP:   make(map[string]string),
		dnsCache:     make(map[string]*dnsCache),
		stopChan:     make(chan struct{}),
		fw:           newMockFirewall(),
	}
	_, ipNet, err := net.ParseCIDR("100.64.0.0/10")
	assert.Nil(t, err)
	ipLongMin := bigEndianUint32(ipNet.IP)
	mask := bigEndianUint32(ipNet.Mask)
	ipLongMax := (ipLongMin & mask) | (^mask)
	m.pool = &fakeIPPool{
		IPNet:     ipNet,
		IpLongMin: ipLongMin + 1,
		IpLongMax: ipLongMax - 1,
	}
	return m
}

func bigEndianUint32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func TestAcquireFakeIP_NewDomain(t *testing.T) {
	m := newTestManager(t)
	ip := m.AcquireFakeIP("example.com")
	assert.NotNil(t, ip)
	assert.True(t, m.IsFakeIP(ip))

	domain := m.GetDomain(ip.String())
	assert.Equal(t, "example.com", domain)
}

func TestAcquireFakeIP_SameDomainReturnsSameIP(t *testing.T) {
	m := newTestManager(t)
	ip1 := m.AcquireFakeIP("example.com")
	ip2 := m.AcquireFakeIP("example.com")
	assert.Equal(t, ip1.String(), ip2.String())
}

func TestAcquireFakeIP_DifferentDomainsGetDifferentIPs(t *testing.T) {
	m := newTestManager(t)
	ip1 := m.AcquireFakeIP("a.example.com")
	ip2 := m.AcquireFakeIP("b.example.com")
	assert.NotEqual(t, ip1.String(), ip2.String())
}

func TestAcquireFakeIP_UpdatesLastAccess(t *testing.T) {
	m := newTestManager(t)
	ip := m.AcquireFakeIP("example.com")
	time.Sleep(10 * time.Millisecond)
	m.AcquireFakeIP("example.com") // 再次获取应更新 LastAccess

	entry := m.active[ip.String()]
	assert.NotNil(t, entry)
	assert.True(t, time.Since(entry.GetLastAccess()) < 100*time.Millisecond)
}

func TestIsFakeIP_InRange(t *testing.T) {
	m := newTestManager(t)
	ip := net.ParseIP("100.64.0.5")
	assert.True(t, m.IsFakeIP(ip))
}

func TestIsFakeIP_OutOfRange(t *testing.T) {
	m := newTestManager(t)
	ip := net.ParseIP("8.8.8.8")
	assert.False(t, m.IsFakeIP(ip))
}

func TestIsFakeIP_NilPool(t *testing.T) {
	m := &FakeDNSManager{}
	assert.False(t, m.IsFakeIP(net.ParseIP("100.64.0.5")))
}

func TestGetRealIP_NotExists(t *testing.T) {
	m := newTestManager(t)
	assert.Equal(t, "", m.GetRealIP("100.64.0.99"))
}

func TestGetDomain_NotExists(t *testing.T) {
	m := newTestManager(t)
	assert.Equal(t, "", m.GetDomain("100.64.0.99"))
}

func TestAddMapping_NewMapping(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")
	err := m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")
	assert.Nil(t, err)
	assert.Equal(t, "1.2.3.4", m.GetRealIP(fakeIP.String()))
}

func TestAddMapping_SameMappingIdempotent(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")
	m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")
	err := m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")
	assert.Nil(t, err)
}

func TestAddMapping_FakeIPCleanedUp(t *testing.T) {
	m := newTestManager(t)
	err := m.AddMapping("100.64.0.99", "1.2.3.4", "example.com")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "cleaned up")
}

func TestAddMapping_ReplacedMapping(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")
	m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")
	err := m.AddMapping(fakeIP.String(), "5.6.7.8", "example.com")
	assert.Nil(t, err)
	assert.Equal(t, "5.6.7.8", m.GetRealIP(fakeIP.String()))
}

func TestAddMapping_AddNatRuleFailRollback(t *testing.T) {
	m := newTestManager(t)
	fw := m.fw.(*mockFirewall)
	fakeIP := m.AcquireFakeIP("example.com")
	m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")

	// 让 AddNatRule 失败
	fw.addFail = true
	err := m.AddMapping(fakeIP.String(), "5.6.7.8", "example.com")
	assert.NotNil(t, err)
	assert.Equal(t, "1.2.3.4", m.GetRealIP(fakeIP.String()))
}

func TestAddMapping_DelNatRuleFailRollback(t *testing.T) {
	m := newTestManager(t)
	fw := m.fw.(*mockFirewall)
	fakeIP := m.AcquireFakeIP("example.com")
	m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")

	// 让 DelNatRule 失败（替换映射时先删旧规则）
	fw.delFail = true
	err := m.AddMapping(fakeIP.String(), "5.6.7.8", "example.com")
	assert.NotNil(t, err)
	assert.Equal(t, "1.2.3.4", m.GetRealIP(fakeIP.String()))
}

func TestAddMapping_NewMappingAddFail(t *testing.T) {
	m := newTestManager(t)
	fw := m.fw.(*mockFirewall)
	fw.addFail = true
	fakeIP := m.AcquireFakeIP("example.com")
	err := m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")
	assert.NotNil(t, err)
	// 无旧映射，应删除
	assert.Equal(t, "", m.GetRealIP(fakeIP.String()))
}

func TestRemoveMapping_Exists(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")
	m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")
	err := m.RemoveMapping(fakeIP.String())
	assert.Nil(t, err)
	assert.Equal(t, "", m.GetRealIP(fakeIP.String()))
}

func TestRemoveMapping_NotExists(t *testing.T) {
	m := newTestManager(t)
	err := m.RemoveMapping("100.64.0.99")
	assert.Nil(t, err) // 不存在不算错误
}

func TestLookupAndTouch_NoMapping(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")
	realIP, domain, needRefresh := m.LookupAndTouch(fakeIP.String())
	assert.Equal(t, "", realIP)
	assert.Equal(t, "example.com", domain)
	assert.False(t, needRefresh)
}

func TestLookupAndTouch_WithMapping(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")
	m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")
	realIP, domain, needRefresh := m.LookupAndTouch(fakeIP.String())
	assert.Equal(t, "1.2.3.4", realIP)
	assert.Equal(t, "example.com", domain)
	assert.False(t, needRefresh)
}

func TestLookupAndTouch_NotInActive(t *testing.T) {
	m := newTestManager(t)
	realIP, domain, needRefresh := m.LookupAndTouch("100.64.0.99")
	assert.Equal(t, "", realIP)
	assert.Equal(t, "", domain)
	assert.False(t, needRefresh)
}

func TestLookupAndTouch_RefreshExpired(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")
	m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")
	m.active[fakeIP.String()].SetRefreshAt(time.Now().Add(-1 * time.Hour))

	_, _, needRefresh := m.LookupAndTouch(fakeIP.String())
	assert.True(t, needRefresh)

	// 第二次不应再触发（RefreshAt 被顺延）
	_, _, needRefresh = m.LookupAndTouch(fakeIP.String())
	assert.False(t, needRefresh)
}

func TestLookupAndTouch_RefreshNotExpired(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")
	m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")
	m.setRefreshAt(fakeIP.String(), 300)

	_, _, needRefresh := m.LookupAndTouch(fakeIP.String())
	assert.False(t, needRefresh)
}

func TestUpdateAccess(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")
	old := m.active[fakeIP.String()].GetLastAccess()
	time.Sleep(10 * time.Millisecond)
	m.UpdateAccess(fakeIP.String())
	newTime := m.active[fakeIP.String()].GetLastAccess()
	assert.True(t, newTime.After(old))
}

func TestUpdateAccess_NotExists(t *testing.T) {
	m := newTestManager(t)
	m.UpdateAccess("100.64.0.99")
}

func TestUpdateDNSCache(t *testing.T) {
	m := newTestManager(t)
	m.updateDNSCache("example.com|8.8.8.8:53", "1.2.3.4", 60)
	m.dnsCacheMu.RLock()
	entry, exists := m.dnsCache["example.com|8.8.8.8:53"]
	m.dnsCacheMu.RUnlock()
	assert.True(t, exists)
	assert.Equal(t, "1.2.3.4", entry.RealIP)
	assert.Equal(t, uint32(60), entry.TTL)
	assert.True(t, entry.ExpireAt.After(time.Now()))
}

func TestDNSCache_MinTTL(t *testing.T) {
	m := newTestManager(t)
	// TTL=5 应被提升到 minRefreshInterval(60s)
	m.updateDNSCache("test.com|8.8.8.8:53", "1.2.3.4", 5)
	m.dnsCacheMu.RLock()
	entry := m.dnsCache["test.com|8.8.8.8:53"]
	m.dnsCacheMu.RUnlock()
	assert.True(t, time.Until(entry.ExpireAt) >= 55*time.Second)
}

func TestCleanupExpiredDNSCache(t *testing.T) {
	m := newTestManager(t)
	// 写入一个已过期的缓存
	m.dnsCacheMu.Lock()
	m.dnsCache["expired.com|8.8.8.8:53"] = &dnsCache{
		RealIP:   "1.2.3.4",
		TTL:      60,
		ExpireAt: time.Now().Add(-1 * time.Hour),
	}
	m.dnsCache["valid.com|8.8.8.8:53"] = &dnsCache{
		RealIP:   "5.6.7.8",
		TTL:      60,
		ExpireAt: time.Now().Add(1 * time.Hour),
	}
	m.dnsCacheMu.Unlock()

	m.cleanupExpiredDNSCache()

	m.dnsCacheMu.RLock()
	_, expiredExists := m.dnsCache["expired.com|8.8.8.8:53"]
	_, validExists := m.dnsCache["valid.com|8.8.8.8:53"]
	m.dnsCacheMu.RUnlock()

	assert.False(t, expiredExists)
	assert.True(t, validExists)
}

func TestCleanupExpiredFakeIPs(t *testing.T) {
	m := newTestManager(t)
	// 活跃的（不应被清理）
	ip1 := m.AcquireFakeIP("active.com")
	m.AddMapping(ip1.String(), "1.2.3.4", "active.com")

	// 过期的（应被清理）
	ip2 := m.AcquireFakeIP("expired.com")
	m.AddMapping(ip2.String(), "5.6.7.8", "expired.com")
	m.active[ip2.String()].SetLastAccess(time.Now().Add(-3 * time.Hour))

	m.cleanupExpiredFakeIPs()

	// active.com 应还在
	assert.Equal(t, "active.com", m.GetDomain(ip1.String()))
	assert.Equal(t, "1.2.3.4", m.GetRealIP(ip1.String()))

	// expired.com 应被清理
	assert.Equal(t, "", m.GetDomain(ip2.String()))
	assert.Equal(t, "", m.GetRealIP(ip2.String()))
}

func TestSetRefreshAt(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")

	// 未调用 setRefreshAt 前, GetRefreshAt 应返回 zero time (IsZero()=true)
	entry := m.active[fakeIP.String()]
	assert.True(t, entry.GetRefreshAt().IsZero(), "未设置时 GetRefreshAt 应返回 zero time")

	// ttl=300 → refreshAt = now + 300s
	m.setRefreshAt(fakeIP.String(), 300)
	refreshAt := entry.GetRefreshAt()
	assert.False(t, refreshAt.IsZero(), "设置后 GetRefreshAt 不应为 zero time")
	assert.True(t, refreshAt.After(time.Now().Add(290*time.Second)))
	assert.True(t, refreshAt.Before(time.Now().Add(310*time.Second)))
}

func TestSetRefreshAt_MinInterval(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")

	// ttl=5 → 应被 max() 提升到 minRefreshInterval(60s)
	m.setRefreshAt(fakeIP.String(), 5)
	entry := m.active[fakeIP.String()]
	refreshAt := entry.GetRefreshAt()
	assert.True(t, refreshAt.After(time.Now().Add(55*time.Second)))
}

func TestSetRefreshAt_NotInActive(t *testing.T) {
	m := newTestManager(t)
	m.setRefreshAt("100.64.0.99", 60)
}

func TestStop_CleansUpFirewall(t *testing.T) {
	m := newTestManager(t)
	m.Start()
	ip := m.AcquireFakeIP("example.com")
	m.AddMapping(ip.String(), "1.2.3.4", "example.com")

	fw := m.fw.(*mockFirewall)
	assert.Equal(t, 1, len(fw.natRules))

	m.Stop()

	// 应被调用（mock 里是空操作，但 fw 不应为 nil）
	// 后 stopped 应为 true
	assert.True(t, m.stopped.Load())
}

func TestStop_Idempotent(t *testing.T) {
	m := newTestManager(t)
	m.Start()
	m.Stop()
	// 再次 Stop 不应 panic
	m.Stop()
}

func TestResolveAndMapping_AlreadyMapped(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")
	m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")

	// 已有映射，应直接返回不触发解析
	m.ResolveAndMapping(fakeIP.String(), "example.com", "8.8.8.8:53")
	// 映射任务中不应有 example.com
	_, loaded := m.mappingTasks.Load("example.com")
	assert.False(t, loaded)
}

func TestResolveAndMapping_Stopped(t *testing.T) {
	m := newTestManager(t)
	m.Start()
	m.Stop()

	fakeIP := m.AcquireFakeIP("example.com")
	m.ResolveAndMapping(fakeIP.String(), "example.com", "8.8.8.8:53")
	// stopped 后不应触发解析
	_, loaded := m.mappingTasks.Load("example.com")
	assert.False(t, loaded)
}

func TestAcquireFakeIP_Concurrent(t *testing.T) {
	m := newTestManager(t)
	var wg sync.WaitGroup
	ips := make([]string, 100)
	for i := range 100 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ip := m.AcquireFakeIP("example.com")
			ips[idx] = ip.String()
		}(i)
	}
	wg.Wait()

	// 所有并发请求应拿到同一个 IP
	first := ips[0]
	for i := 1; i < 100; i++ {
		assert.Equal(t, first, ips[i], "concurrent AcquireFakeIP should return same IP")
	}
}

func TestAddMapping_ConcurrentSameFakeIP(t *testing.T) {
	m := newTestManager(t)
	fakeIP := m.AcquireFakeIP("example.com")

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			m.AddMapping(fakeIP.String(), "1.2.3.4", "example.com")
		})
	}
	wg.Wait()

	assert.Equal(t, "1.2.3.4", m.GetRealIP(fakeIP.String()))
}

func TestAcquireFakeIPv6_NewDomain(t *testing.T) {
	m := newTestManager(t)
	assert.Nil(t, m.initV6Pool("fd00::/112"))
	ip := m.AcquireFakeIPv6("example.com")
	assert.NotNil(t, ip)
	assert.Equal(t, 16, len(ip)) // 128 位
	assert.True(t, m.IsFakeIP(ip))
	assert.Equal(t, "example.com", m.GetDomain(ip.String()))
}

func TestAcquireFakeIPv6_SameDomainReturnsSameIP(t *testing.T) {
	m := newTestManager(t)
	assert.Nil(t, m.initV6Pool("fd00::/112"))
	ip1 := m.AcquireFakeIPv6("example.com")
	ip2 := m.AcquireFakeIPv6("example.com")
	assert.Equal(t, ip1.String(), ip2.String())
}

func TestAcquireFakeIPv6_DifferentDomainsGetDifferentIPs(t *testing.T) {
	m := newTestManager(t)
	assert.Nil(t, m.initV6Pool("fd00::/112"))
	ip1 := m.AcquireFakeIPv6("a.example.com")
	ip2 := m.AcquireFakeIPv6("b.example.com")
	assert.NotEqual(t, ip1.String(), ip2.String())
}

func TestAcquireFakeIPv6_AllInRange(t *testing.T) {
	m := newTestManager(t)
	assert.Nil(t, m.initV6Pool("fd00::/112"))
	_, v6Net, _ := net.ParseCIDR("fd00::/112")
	for i := range 20 {
		ip := m.AcquireFakeIPv6(fmt.Sprintf("d%d.example.com", i))
		assert.NotNil(t, ip)
		assert.True(t, v6Net.Contains(ip), "v6 fakeIP not in range: %s", ip)
	}
}

func TestAcquireFakeIPv6_PoolNil(t *testing.T) {
	m := newTestManager(t) // 未初始化 v6 池
	assert.Nil(t, m.AcquireFakeIPv6("example.com"))
}

func TestAcquireFakeIPv6_AddMapping(t *testing.T) {
	m := newTestManager(t)
	assert.Nil(t, m.initV6Pool("fd00::/112"))
	fakeIP := m.AcquireFakeIPv6("example.com")
	assert.Nil(t, m.AddMapping(fakeIP.String(), "2001:db8::1", "example.com"))
	assert.Equal(t, "2001:db8::1", m.GetRealIP(fakeIP.String()))
	assert.True(t, m.IsFakeIP(fakeIP))
}

// startTestAAAAServer 起一个本地 UDP DNS 服务器，对 example.com 的 AAAA 查询返回 aaaaIP；
// nodata=true 时返回空 Answer（模拟上游过滤 AAAA 的负响应），用于验证回退逻辑。
func startTestAAAAServer(t *testing.T, aaaaIP string, nodata bool) string {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	assert.Nil(t, err)
	addr := pc.LocalAddr().String()
	srv := &dns.Server{PacketConn: pc}
	dns.HandleFunc("example.com.", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		if nodata {
			_ = w.WriteMsg(m)
			return
		}
		for _, q := range r.Question {
			if q.Qtype == dns.TypeAAAA && aaaaIP != "" {
				m.Answer = append(m.Answer, &dns.AAAA{
					Hdr:  dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 60},
					AAAA: net.ParseIP(aaaaIP),
				})
			}
		}
		_ = w.WriteMsg(m)
	})
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	return addr
}

// 验证「DNS 层 IPv6 优先」乐观分配：双栈开启时 AcquireFakeIPv6 立即分配
// v6 fakeIP（不依赖同步探测）；ResolveAndMapping 异步解析 AAAA 并写入映射，随后 IsAAAAPositive 为真。
func TestPreferV6Optimistic(t *testing.T) {
	m := newTestManager(t)
	assert.Nil(t, m.initV6Pool("fd00::/112"))
	upstream := startTestAAAAServer(t, "2001:db8::1", false)
	ip := m.AcquireFakeIPv6("example.com")
	assert.NotNil(t, ip)
	assert.Equal(t, 16, len(ip))
	// 同一域名应返回同一 v6 fakeIP（幂等）
	assert.Equal(t, ip.String(), m.AcquireFakeIPv6("example.com").String())
	assert.Equal(t, "example.com", m.GetDomain(ip.String()))
	assert.True(t, m.IsFakeIP(ip))
	// 异步解析并建映射
	m.ResolveAndMapping(ip.String(), "example.com", upstream)
	assert.Eventually(t, func() bool {
		return m.GetRealIP(ip.String()) == "2001:db8::1"
	}, 3*time.Second, 20*time.Millisecond)
	assert.True(t, m.IsAAAAPositive("example.com", upstream))
}

// 验证上游过滤/不支持 AAAA 时，异步解析回填负缓存：
// IsAAAANegative 为真、无映射、不黑洞。
func TestPreferV6_AAAAFilteredNegative(t *testing.T) {
	m := newTestManager(t)
	assert.Nil(t, m.initV6Pool("fd00::/112"))
	upstream := startTestAAAAServer(t, "", true) // 返回空 Answer（负响应）
	ip := m.AcquireFakeIPv6("example.com")
	assert.NotNil(t, ip)
	m.ResolveAndMapping(ip.String(), "example.com", upstream)
	assert.Eventually(t, func() bool {
		return m.IsAAAANegative("example.com", upstream)
	}, 3*time.Second, 20*time.Millisecond)
	assert.Equal(t, "", m.GetRealIP(ip.String()))
}

func TestPreferV6_NoV6Pool(t *testing.T) {
	m := newTestManager(t) // 未初始化 v6 池（等价于未开双栈）
	assert.False(t, m.IsV6Enabled())
	assert.Nil(t, m.AcquireFakeIPv6("example.com"))
}

func TestIsFakeIP_V6Range(t *testing.T) {
	m := newTestManager(t)
	assert.Nil(t, m.initV6Pool("fd00::/112"))
	// 在 v6 假地址池内
	assert.True(t, m.IsFakeIP(net.ParseIP("fd00::5")))
	// 不在 v6 池、也不在 v4 池（100.64.0.0/10）之外，应判否
	assert.False(t, m.IsFakeIP(net.ParseIP("2001:db8::5")))
}

func TestResolveAndMapping_V6AAAA(t *testing.T) {
	m := newTestManager(t)
	assert.Nil(t, m.initV6Pool("fd00::/112"))
	fakeIP := m.AcquireFakeIPv6("example.com")
	// 异步解析键带 |AAAA，与 v4 互不阻塞
	m.ResolveAndMapping(fakeIP.String(), "example.com", "8.8.8.8:53")
	_, loaded := m.mappingTasks.Load("example.com|AAAA")
	assert.True(t, loaded)
}
