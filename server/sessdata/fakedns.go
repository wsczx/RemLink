package sessdata

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"github.com/wsczx/remlink/base"
)

// 管理 FakeDNS 和 FakeIP 功能
type FakeDNSManager struct {
	// FakeIP 池配置
	pool         *fakeIPPool
	poolV6       *fakeIPPoolV6           // IPv6 假地址池
	active       map[string]*fakeIPEntry // fakeIP(v4) -> entry
	activeV6     map[string]*fakeIPEntry // fakeIP(v6) -> entry
	domainToIP   map[string]string       // domain -> v4 fakeIP
	domainToIPV6 map[string]string       // domain -> v6 fakeIP
	ipToRealIP   map[string]string       // fakeIP(v4/v6) -> realIP
	mu           sync.RWMutex

	// DNS 缓存
	dnsCache   map[string]*dnsCache
	dnsCacheMu sync.RWMutex

	// 映射任务去重：同一 FakeIP 的异步映射/刷新只允许一个任务
	mappingTasks sync.Map
	// 服务端可信上游解析任务共享：同域名、同地址族、同上游只保留一个查询
	dnsResolveTasks sync.Map

	// 防火墙后端
	fw Firewall

	// 生命周期管理
	stopChan chan struct{}
	stopOnce sync.Once
	stopMu   sync.RWMutex
	wg       sync.WaitGroup
	stopped  atomic.Bool
}

type fakeIPEntry struct {
	Domain     string
	FakeIP     string
	LastAccess atomic.Int64
	RefreshAt  atomic.Int64 // 到期重新解析的时间点
}

func (e *fakeIPEntry) GetLastAccess() time.Time  { return time.Unix(0, e.LastAccess.Load()) }
func (e *fakeIPEntry) SetLastAccess(t time.Time) { e.LastAccess.Store(t.UnixNano()) }
func (e *fakeIPEntry) GetRefreshAt() time.Time {
	if v := e.RefreshAt.Load(); v != 0 {
		return time.Unix(0, v)
	}
	return time.Time{}
}
func (e *fakeIPEntry) SetRefreshAt(t time.Time) { e.RefreshAt.Store(t.UnixNano()) }

type fakeIPPool struct {
	IPNet     *net.IPNet
	IpLongMin uint32
	IpLongMax uint32
	counter   atomic.Uint32
}

// fakeIPPoolV6 用 big.Int 表示 128 位地址空间，支持 /64+ 的 v6 假地址段
type fakeIPPoolV6 struct {
	IPNet      *net.IPNet
	networkInt *big.Int // 网络地址（128 位）
	min        *big.Int // 第一个可用地址（网络地址 + 2，跳过网络地址与网络地址+1）
	max        *big.Int // 最后一个可用地址（网段末）
	cursor     *big.Int // 当前游标（相对 min 的偏移）
}

// 按游标递增分配一个 v6 假地址；到达 max 后回到 min
func (p *fakeIPPoolV6) next() net.IP {
	if p == nil || p.max == nil {
		return nil
	}
	if p.cursor == nil {
		p.cursor = new(big.Int)
	}
	candidate := new(big.Int).Add(p.min, p.cursor)
	if candidate.Cmp(p.max) > 0 {
		p.cursor = big.NewInt(0)
		candidate = new(big.Int).Set(p.min)
	} else {
		p.cursor.Add(p.cursor, big.NewInt(1))
	}
	b := candidate.Bytes()
	ip := make(net.IP, 16)
	copy(ip[16-len(b):], b)
	return ip
}

type dnsCache struct {
	RealIP   string
	TTL      uint32
	ExpireAt time.Time
}

type dnsResolveTask struct {
	done chan struct{}
	ip   string
	ttl  uint32
	err  error
}

// 全局单例
var GlobalFakeDNSManager *FakeDNSManager
var globalFakeDNSOnce sync.Once

const (
	DefaultFakeIPRange = "100.64.0.0/10"
	// DefaultFakeIPv6Range 固定 v6 FakeIP 假地址段：RFC3849 文档前缀，全球永不分配/路由，与 v4 的 100.64.0.0/10（CGNAT 保留段）等价
	DefaultFakeIPv6Range = "2001:db8::/32"
)

// 最小重新解析间隔
const minRefreshInterval = 60 * time.Second

// AAAA 同步探测失败的负缓存 TTL（RealIP="" 的 dnsCache 条目即负缓存）
const (
	negCacheAuthTTL   = 60 * time.Second // 上游明确回负响应（无 AAAA/NXDOMAIN）
	negCacheNetErrTTL = 10 * time.Second // 网络错误/超时，短缓存避免反复阻塞读循环
)

// 获取全局 FakeDNS 管理器单例
func GetFakeDNSManager() *FakeDNSManager {
	globalFakeDNSOnce.Do(func() {
		var err error
		GlobalFakeDNSManager, err = newFakeDNSManager(DefaultFakeIPRange)
		if err != nil {
			base.Error("Failed to initialize FakeDNS manager:", err)
			return
		}
		// 初始化 v6 假地址池：受 Ipv6CIDR 总开关约束（空=纯 v4 字节级不变）；
		// v6 FakeIP 段固定为常量（与 v4 一致不可配），仅当开启 IPv6 双栈（Ipv6CIDR 非空）时初始化
		if base.GetCfg().Ipv6CIDR != "" {
			if err := GlobalFakeDNSManager.initV6Pool(DefaultFakeIPv6Range); err != nil {
				base.Error("Failed to init v6 FakeDNS pool:", err)
			}
		}
		GlobalFakeDNSManager.Start()
		base.Info("Global FakeDNS manager initialized")
	})
	return GlobalFakeDNSManager
}

// 创建 FakeDNS 管理器
func newFakeDNSManager(ipRange string) (*FakeDNSManager, error) {
	m := &FakeDNSManager{
		active:       make(map[string]*fakeIPEntry),
		activeV6:     make(map[string]*fakeIPEntry),
		domainToIP:   make(map[string]string),
		domainToIPV6: make(map[string]string),
		ipToRealIP:   make(map[string]string),
		dnsCache:     make(map[string]*dnsCache),
		stopChan:     make(chan struct{}),
	}

	// 解析 IP 范围
	_, ipNet, err := net.ParseCIDR(ipRange)
	if err != nil {
		return nil, fmt.Errorf("invalid IP range: %v", err)
	}

	ipLongMin := binary.BigEndian.Uint32(ipNet.IP)
	mask := binary.BigEndian.Uint32(ipNet.Mask)
	ipLongMax := (ipLongMin & mask) | (^mask)

	m.pool = &fakeIPPool{
		IPNet:     ipNet,
		IpLongMin: ipLongMin + 1,
		IpLongMax: ipLongMax - 1,
	}

	// 初始化防火墙后端
	m.fw = GetFirewall()
	if m.fw == nil {
		return nil, fmt.Errorf("failed to initialize firewall: firewall is nil")
	}

	// 创建防火墙链
	if err := m.fw.CreateChains(base.GetCfg().Ipv4CIDR, ipNet.String()); err != nil {
		return nil, err
	}

	base.Info("FakeDNS manager initialized:", ipRange)
	return m, nil
}

// 启动后台清理任务
func (m *FakeDNSManager) Start() {
	m.wg.Add(2)
	go m.cleanupFakeIPLoop()
	go m.cleanupDNSCacheLoop()
}

// 停止管理器
func (m *FakeDNSManager) Stop() {
	m.stopOnce.Do(func() {
		m.stopMu.Lock()
		m.stopped.Store(true)
		close(m.stopChan)
		m.stopMu.Unlock()
	})
	m.wg.Wait()
	if m.fw != nil {
		m.fw.CleanupFakeIP()
	}
}

// 为域名分配 FakeIP
func (m *FakeDNSManager) AcquireFakeIP(domain string) net.IP {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已分配
	if fakeIP, exists := m.domainToIP[domain]; exists {
		if entry, ok := m.active[fakeIP]; ok {
			entry.SetLastAccess(time.Now())
			return net.ParseIP(fakeIP)
		}
	}

	// 分配新的 FakeIP
	counter := m.pool.counter.Add(1)
	ipLong := m.pool.IpLongMin + (counter % (m.pool.IpLongMax - m.pool.IpLongMin + 1))

	fakeIP := make(net.IP, 4)
	binary.BigEndian.PutUint32(fakeIP, ipLong)

	fakeIPStr := fakeIP.String()
	entry := &fakeIPEntry{
		Domain: domain,
		FakeIP: fakeIPStr,
	}
	entry.SetLastAccess(time.Now())

	m.active[fakeIPStr] = entry
	m.domainToIP[domain] = fakeIPStr

	base.Debug("Allocated FakeIP:", domain, "->", fakeIPStr)
	return fakeIP
}

// 为域名分配一个 v6 假地址
func (m *FakeDNSManager) AcquireFakeIPv6(domain string) net.IP {
	if m.poolV6 == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if fakeIP, exists := m.domainToIPV6[domain]; exists {
		if entry, ok := m.activeV6[fakeIP]; ok {
			entry.SetLastAccess(time.Now())
			return net.ParseIP(fakeIP)
		}
	}

	ip := m.poolV6.next()
	if ip == nil {
		base.Error("v6 FakeIP pool exhausted")
		return nil
	}
	fakeIPStr := ip.String()
	entry := &fakeIPEntry{
		Domain: domain,
		FakeIP: fakeIPStr,
	}
	entry.SetLastAccess(time.Now())

	m.activeV6[fakeIPStr] = entry
	m.domainToIPV6[domain] = fakeIPStr

	base.Debug("Allocated v6 FakeIP:", domain, "->", fakeIPStr)
	return ip
}

//  1. 查 fakeIP 对应的 realIP 和 domain
//  2. 刷新访问时间 LastAccess
//  3. 判断映射是否到期需要重新解析; 若需要, 立即把 RefreshAt 顺延一个
//     最小周期, 避免刷新完成前同一窗口内每个包都触发 RenewMapping
//
// 返回 realIP(空表示尚无映射)、domain(空表示 fakeIP 不在映射表)、needRefresh
func (m *FakeDNSManager) LookupAndTouch(fakeIP string) (realIP, domain string, needRefresh bool) {
	now := time.Now()
	// v6 假地址走 activeV6
	if strings.Contains(fakeIP, ":") {
		m.mu.RLock()
		defer m.mu.RUnlock()
		entry, ok := m.activeV6[fakeIP]
		if !ok {
			return "", "", false
		}
		entry.SetLastAccess(now)
		domain = entry.Domain
		realIP = m.ipToRealIP[fakeIP]
		if realIP == "" {
			return "", domain, false
		}
		refreshAt := entry.GetRefreshAt()
		if !refreshAt.IsZero() && now.After(refreshAt) {
			entry.SetRefreshAt(now.Add(minRefreshInterval))
			needRefresh = true
		}
		return realIP, domain, needRefresh
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.active[fakeIP]
	if !ok {
		return "", "", false
	}
	entry.SetLastAccess(now)
	domain = entry.Domain

	realIP = m.ipToRealIP[fakeIP]
	if realIP == "" {
		// 映射尚未就绪, 交给首解析路径处理
		return "", domain, false
	}

	refreshAt := entry.GetRefreshAt()
	if !refreshAt.IsZero() && now.After(refreshAt) {
		// 顺延一个最小周期, 防止刷新完成前每个包重复触发
		entry.SetRefreshAt(now.Add(minRefreshInterval))
		needRefresh = true
	}
	return realIP, domain, needRefresh
}

// 同步解析域名并写入 fakeIP->realIP 映射；用于 DNS 响应前确保首个连接可转发
func (m *FakeDNSManager) ResolveAndMappingSync(fakeIP, domain, upstreamDNS string) error {
	if m.GetRealIP(fakeIP) != "" {
		return nil
	}
	// 内部按域名、地址族和上游共享任务
	realIP, ttl, err := m.resolveWithCache(domain, upstreamDNS, strings.Contains(fakeIP, ":"))
	if err != nil {
		return err
	}
	if err := m.AddMapping(fakeIP, realIP, domain); err != nil {
		return err
	}
	m.setRefreshAt(fakeIP, ttl)
	base.Debug("Resolved & mapped synchronously:", domain, "->", realIP)
	return nil
}

// 异步解析域名并将 fakeIP->realIP 映射写入并配置 NAT 规则
func (m *FakeDNSManager) ResolveAndMapping(fakeIP, domain, upstreamDNS string) {
	// 已有映射直接返回
	if m.GetRealIP(fakeIP) != "" {
		return
	}
	// 去重: 同一域名同族(A/AAAA)只允许一个在解析；v4/v6 互不阻塞
	mappingTasksKey := domain
	if strings.Contains(fakeIP, ":") {
		mappingTasksKey += "|AAAA"
	}
	if _, loaded := m.mappingTasks.LoadOrStore(mappingTasksKey, struct{}{}); loaded {
		return
	}

	m.stopMu.RLock()
	if m.stopped.Load() {
		m.stopMu.RUnlock()
		m.mappingTasks.Delete(mappingTasksKey)
		return
	}
	m.wg.Add(1)
	m.stopMu.RUnlock()

	go func() {
		defer m.wg.Done()
		defer m.mappingTasks.Delete(mappingTasksKey)

		// v6 fakeIP 解析 AAAA，v4 解析 A
		realIP, ttl, err := m.resolveWithCache(domain, upstreamDNS, strings.Contains(fakeIP, ":"))
		if err != nil {
			base.Debug("Failed to resolve domain:", domain, "error:", err)
			return
		}
		if err := m.AddMapping(fakeIP, realIP, domain); err != nil {
			base.Debug("Failed to add FakeIP mapping:", err)
			return
		}
		m.setRefreshAt(fakeIP, ttl)
		base.Debug("Resolved & mapped:", domain, "->", realIP)
	}()
}

// 映射到期时异步重新解析并替换 DNAT。
func (m *FakeDNSManager) RenewMapping(fakeIP, domain, upstreamDNS string) {
	isV6 := strings.Contains(fakeIP, ":")
	mappingTasksKey := domain
	if isV6 {
		mappingTasksKey += "|AAAA"
	}
	if _, loaded := m.mappingTasks.LoadOrStore(mappingTasksKey, struct{}{}); loaded {
		return
	}
	m.stopMu.RLock()
	if m.stopped.Load() {
		m.stopMu.RUnlock()
		m.mappingTasks.Delete(mappingTasksKey)
		return
	}
	m.wg.Add(1)
	m.stopMu.RUnlock()

	go func() {
		defer m.wg.Done()
		defer m.mappingTasks.Delete(mappingTasksKey)

		// 绕过缓存重新解析，配合 queryDNS 随机选更快节点
		var (
			realIP string
			ttl    uint32
			err    error
		)
		if isV6 {
			realIP, ttl, err = m.ResolveDomainAAAA(domain, upstreamDNS)
		} else {
			realIP, ttl, err = m.ResolveDomain(domain, upstreamDNS)
		}
		if err != nil {
			base.Debug("Refresh resolve failed:", domain, "error:", err)
			m.setRefreshAt(fakeIP, 0) // 0 经 max() 兜底为 minRefreshInterval, 稍后重试
			return
		}
		m.updateDNSCache(dnsCacheKey(domain, upstreamDNS, isV6), realIP, ttl)
		if err := m.AddMapping(fakeIP, realIP, domain); err != nil {
			base.Debug("Refresh AddMapping failed:", err)
			return
		}
		m.setRefreshAt(fakeIP, ttl)
		base.Debug("Refreshed mapping:", domain, "->", realIP)
	}()
}

// 根据 FakeIP 获取真实 IP
func (m *FakeDNSManager) GetRealIP(fakeIP string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if realIP, exists := m.ipToRealIP[fakeIP]; exists {
		return realIP
	}
	return ""
}

// 根据 FakeIP 获取域名（v4/v6 双池）
func (m *FakeDNSManager) GetDomain(fakeIP string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if entry, exists := m.active[fakeIP]; exists {
		return entry.Domain
	}
	if entry, exists := m.activeV6[fakeIP]; exists {
		return entry.Domain
	}
	return ""
}

// 判断 IP 是否在 FakeIP 池中
func (m *FakeDNSManager) IsFakeIP(ip net.IP) bool {
	if m.pool != nil && m.pool.IPNet.Contains(ip) {
		return true
	}
	if m.poolV6 != nil && m.poolV6.IPNet.Contains(ip) {
		return true
	}
	return false
}

// 双栈（v6 FakeIP 池已初始化）才接管 AAAA 查询；否则放行真实 AAAA 解析
func (m *FakeDNSManager) IsV6Enabled() bool {
	return m.poolV6 != nil
}

// 该域名上游已确认无 AAAA（异步 AAAA 解析失败后写入的负缓存）
func (m *FakeDNSManager) IsAAAANegative(domain, upstreamDNS string) bool {
	cacheKey := dnsCacheKey(domain, upstreamDNS, true)
	m.dnsCacheMu.RLock()
	defer m.dnsCacheMu.RUnlock()
	if e, ok := m.dnsCache[cacheKey]; ok && time.Now().Before(e.ExpireAt) {
		return e.RealIP == ""
	}
	return false
}

// 该域名上游 AAAA 已确认可达（异步解析成功写入的正缓存）；
// 仅此时才在「IPv6 优先」下抑制 A 查询，真正引导双栈应用走 v6，避免无谓黑洞
func (m *FakeDNSManager) IsAAAAPositive(domain, upstreamDNS string) bool {
	cacheKey := dnsCacheKey(domain, upstreamDNS, true)
	m.dnsCacheMu.RLock()
	defer m.dnsCacheMu.RUnlock()
	if e, ok := m.dnsCache[cacheKey]; ok && time.Now().Before(e.ExpireAt) {
		return e.RealIP != ""
	}
	return false
}

// 初始化 v6 假地址池；rangeStr 必须是 IPv6 CIDR
func (m *FakeDNSManager) initV6Pool(rangeStr string) error {
	_, ipNet, err := net.ParseCIDR(rangeStr)
	if err != nil {
		return fmt.Errorf("invalid v6 FakeIP range: %v", err)
	}
	if ipNet.IP.To4() != nil {
		return fmt.Errorf("v6 FakeIP range must be an IPv6 CIDR, got %s", rangeStr)
	}
	networkInt := new(big.Int).SetBytes(ipNet.IP.To16())
	ones, bits := ipNet.Mask.Size()
	hostBits := uint(bits - ones)
	size := new(big.Int).Lsh(big.NewInt(1), hostBits)
	min := new(big.Int).Add(networkInt, big.NewInt(2)) // 跳过网络地址与网络地址+1
	max := new(big.Int).Add(networkInt, size)
	max.Sub(max, big.NewInt(1)) // 网段末地址

	m.poolV6 = &fakeIPPoolV6{
		IPNet:      ipNet,
		networkInt: networkInt,
		min:        min,
		max:        max,
		cursor:     new(big.Int),
	}

	// 建 v6 DNAT 链：仅当已启用 IPv6 双栈（有客户端 v6 源段）时限制跳转来源
	if vpnCIDR6 := base.GetCfg().Ipv6CIDR; vpnCIDR6 != "" {
		if err := m.fw.CreateChains(vpnCIDR6, rangeStr); err != nil {
			return err
		}
	}
	base.Info("FakeDNS v6 pool initialized:", rangeStr)
	return nil
}

// 更新 FakeIP 访问时间
func (m *FakeDNSManager) UpdateAccess(fakeIP string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if entry, exists := m.active[fakeIP]; exists {
		entry.SetLastAccess(time.Now())
	}
}

// 添加 FakeIP -> RealIP 映射并创建防火墙规则
func (m *FakeDNSManager) AddMapping(fakeIP, realIP, domain string) error {
	m.mu.Lock()

	// fakeIP 已被清理,放弃添加（v4/v6 各自的活跃表）
	if _, exists := m.entryFor(fakeIP); !exists {
		m.mu.Unlock()
		base.Debug("FakeIP already cleaned up, skip mapping:", fakeIP, domain)
		return fmt.Errorf("fakeIP %s already cleaned up", fakeIP)
	}

	// 检查是否已存在相同映射
	existing, exists := m.ipToRealIP[fakeIP]
	if exists && existing == realIP {
		m.mu.Unlock()
		return nil
	}

	// 记录旧映射
	var oldRealIP string
	if exists {
		oldRealIP = existing
	}

	// 添加新映射
	m.ipToRealIP[fakeIP] = realIP
	m.mu.Unlock()

	// 如果有旧映射,先删除旧的防火墙规则
	if oldRealIP != "" {
		if err := m.fw.DelNatRule(fakeIP, oldRealIP); err != nil {
			// 回滚到旧映射
			m.mu.Lock()
			m.ipToRealIP[fakeIP] = oldRealIP
			m.mu.Unlock()
			return err
		}
	}

	// 添加新的防火墙规则
	if err := m.fw.AddNatRule(fakeIP, realIP); err != nil {
		// 回滚: 有旧映射则恢复旧值，无旧映射则删除
		m.mu.Lock()
		if oldRealIP != "" {
			m.ipToRealIP[fakeIP] = oldRealIP
		} else {
			delete(m.ipToRealIP, fakeIP)
		}
		m.mu.Unlock()
		return err
	}
	base.Debug("Added FakeIP mapping:", fakeIP, "->", realIP, "for domain:", domain)
	return nil
}

// 删除 FakeIP -> RealIP 映射和防火墙规则
func (m *FakeDNSManager) RemoveMapping(fakeIP string) error {
	m.mu.Lock()
	realIP, exists := m.ipToRealIP[fakeIP]
	if !exists {
		m.mu.Unlock()
		return nil
	}
	delete(m.ipToRealIP, fakeIP)
	m.mu.Unlock()

	// 删除防火墙规则
	if err := m.fw.DelNatRule(fakeIP, realIP); err != nil {
		return err
	}

	base.Debug("Removed FakeIP mapping:", fakeIP, "->", realIP)
	return nil
}

// 生成 DNS 缓存键；A/AAAA 分开缓存，避免同域名 v4/v6 结果互相污染
func dnsCacheKey(domain, upstreamDNS string, v6 bool) string {
	if v6 {
		return "AAAA|" + domain + "|" + upstreamDNS
	}
	return domain + "|" + upstreamDNS
}

// 带缓存的解析，同时返回 ttl；v6=true 解析 AAAA
func (m *FakeDNSManager) resolveWithCache(domain, upstreamDNS string, v6 bool) (string, uint32, error) {
	cacheKey := dnsCacheKey(domain, upstreamDNS, v6)
	m.dnsCacheMu.RLock()
	if entry, exists := m.dnsCache[cacheKey]; exists && time.Now().Before(entry.ExpireAt) {
		ip, ttl := entry.RealIP, entry.TTL
		m.dnsCacheMu.RUnlock()
		if ip == "" { // 负缓存条目（AAAA 探测失败写入），不能当成功返回，否则 AddMapping("") 建坏映射
			return "", 0, fmt.Errorf("%w: negative cache", errDNSAuthoritative)
		}
		base.Debug("DNS cache hit:", domain, "->", ip)
		return ip, ttl, nil
	}
	m.dnsCacheMu.RUnlock()

	taskKey := dnsCacheKey(domain, upstreamDNS, v6)
	task := &dnsResolveTask{done: make(chan struct{})}
	actual, loaded := m.dnsResolveTasks.LoadOrStore(taskKey, task)
	if loaded {
		task = actual.(*dnsResolveTask)
		<-task.done
		return task.ip, task.ttl, task.err
	}
	defer m.dnsResolveTasks.Delete(taskKey)
	defer close(task.done)

	var (
		realIP string
		ttl    uint32
		err    error
	)
	if v6 {
		realIP, ttl, err = m.ResolveDomainAAAA(domain, upstreamDNS)
	} else {
		realIP, ttl, err = m.ResolveDomain(domain, upstreamDNS)
	}
	task.ip, task.ttl, task.err = realIP, ttl, err
	if err != nil {
		// A/AAAA 均写入短时负缓存，避免失效 CDN 节点在客户端重试时反复查询并阻塞
		negTTL := negCacheNetErrTTL
		if errors.Is(err, errDNSAuthoritative) {
			negTTL = negCacheAuthTTL
		}
		m.dnsCacheMu.Lock()
		m.dnsCache[cacheKey] = &dnsCache{RealIP: "", TTL: 0, ExpireAt: time.Now().Add(negTTL)}
		m.dnsCacheMu.Unlock()
		return "", 0, err
	}
	m.updateDNSCache(cacheKey, realIP, ttl)
	return realIP, ttl, nil
}

func (m *FakeDNSManager) updateDNSCache(cacheKey, realIP string, ttl uint32) {
	cacheTTL := max(time.Duration(ttl)*time.Second, minRefreshInterval)
	m.dnsCacheMu.Lock()
	m.dnsCache[cacheKey] = &dnsCache{
		RealIP:   realIP,
		TTL:      ttl,
		ExpireAt: time.Now().Add(cacheTTL),
	}
	m.dnsCacheMu.Unlock()
}

// 根据 ttl 设置下次刷新时间
func (m *FakeDNSManager) setRefreshAt(fakeIP string, ttl uint32) {
	d := max(time.Duration(ttl)*time.Second, minRefreshInterval)
	m.mu.RLock()
	entry, ok := m.entryFor(fakeIP)
	m.mu.RUnlock()
	if ok {
		entry.SetRefreshAt(time.Now().Add(d))
	}
}

// 按 fakeIP 地址族取对应活跃表的 entry；调用方须持有 m.mu
func (m *FakeDNSManager) entryFor(fakeIP string) (*fakeIPEntry, bool) {
	if strings.Contains(fakeIP, ":") {
		e, ok := m.activeV6[fakeIP]
		return e, ok
	}
	e, ok := m.active[fakeIP]
	return e, ok
}

var errDNSAuthoritative = errors.New("authoritative negative response")

// 上游 DNS 地址：无端口时补 :53；IPv6 地址需加方括号（[..]:53）
// 支持 IPv6 上游（如 2001:db8::1 或 [2001:db8::1]:53）与自定义端口
func FormatUpstream(addr string) string {
	if addr == "" {
		return addr
	}
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr // 已带端口
	}
	if ip := net.ParseIP(addr); ip != nil && ip.To4() == nil {
		return "[" + addr + "]:53"
	}
	return addr + ":53"
}

// 带重试的 DNS 解析
func (m *FakeDNSManager) ResolveDomain(domain, upstreamDNS string) (string, uint32, error) {
	if upstreamDNS != "" {
		upstreamDNS = FormatUpstream(upstreamDNS)
	}

	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), dns.TypeA)

	const maxRetries = 2
	var lastErr error
	for i := range maxRetries {
		realIP, ttl, err := m.queryDNS(msg, upstreamDNS)
		if err == nil {
			return realIP, ttl, nil
		}
		// 无记录直接返回
		if errors.Is(err, errDNSAuthoritative) {
			return "", 0, err
		}
		lastErr = err
		if i < maxRetries-1 {
			base.Debug("DNS query failed, retrying:", upstreamDNS, "error:", err)
		}
	}
	return "", 0, fmt.Errorf("DNS query failed for %s after %d attempts: %v", domain, maxRetries, lastErr)
}

// 解析域名的 AAAA（IPv6）真实地址（V2）
func (m *FakeDNSManager) ResolveDomainAAAA(domain, upstreamDNS string) (string, uint32, error) {
	if upstreamDNS != "" {
		upstreamDNS = FormatUpstream(upstreamDNS)
	}

	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), dns.TypeAAAA)

	const maxRetries = 2
	var lastErr error
	for i := range maxRetries {
		realIP, ttl, err := m.queryDNS(msg, upstreamDNS)
		if err == nil {
			return realIP, ttl, nil
		}
		if errors.Is(err, errDNSAuthoritative) {
			return "", 0, err
		}
		lastErr = err
		if i < maxRetries-1 {
			base.Debug("DNS AAAA query failed, retrying:", upstreamDNS, "error:", err)
		}
	}
	return "", 0, fmt.Errorf("DNS AAAA query failed for %s after %d attempts: %v", domain, maxRetries, lastErr)
}

// 单次 DNS 查询
func (m *FakeDNSManager) queryDNS(msg *dns.Msg, server string) (string, uint32, error) {
	c := new(dns.Client)
	c.Timeout = 3 * time.Second
	r, _, err := c.Exchange(msg, server)
	if err != nil {
		return "", 0, err // 网络错误/超时,可重试
	}
	// NXDOMAIN 或其他非成功，不重试
	if r.Rcode != dns.RcodeSuccess {
		return "", 0, fmt.Errorf("%w: rcode=%d", errDNSAuthoritative, r.Rcode)
	}
	// 只收集请求类型对应的地址，避免 AAAA 查询误选 A 记录导致 DNAT6 失败
	// 多地址时随机选择一条，避免 CDN 长期钉死第一个节点
	var ips []string
	var ttl uint32
	qType := dns.TypeA
	if len(msg.Question) > 0 {
		qType = msg.Question[0].Qtype
	}
	for _, ans := range r.Answer {
		switch a := ans.(type) {
		case *dns.A:
			if qType != dns.TypeA {
				continue
			}
			ips = append(ips, a.A.String())
			if ttl == 0 {
				ttl = a.Hdr.Ttl
			}
		case *dns.AAAA:
			if qType != dns.TypeAAAA {
				continue
			}
			ips = append(ips, a.AAAA.String())
			if ttl == 0 {
				ttl = a.Hdr.Ttl
			}
		}
	}
	if len(ips) == 0 {
		return "", 0, fmt.Errorf("%w: no IPv4/IPv6 address in response", errDNSAuthoritative)
	}
	return ips[rand.Intn(len(ips))], ttl, nil
}

// 定期清理过期的 FakeIP
func (m *FakeDNSManager) cleanupFakeIPLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanupExpiredFakeIPs()
		case <-m.stopChan:
			return
		}
	}
}

// 清理过期的 FakeIP 映射
func (m *FakeDNSManager) cleanupExpiredFakeIPs() {
	var toCleanup []string

	// 收集需要清理的 FakeIP（v4 + v6）
	m.mu.RLock()
	now := time.Now()
	for fakeIP, entry := range m.active {
		if now.Sub(entry.GetLastAccess()) > 2*time.Hour {
			toCleanup = append(toCleanup, fakeIP)
		}
	}
	for fakeIP, entry := range m.activeV6 {
		if now.Sub(entry.GetLastAccess()) > 2*time.Hour {
			toCleanup = append(toCleanup, fakeIP)
		}
	}
	m.mu.RUnlock()

	// 逐个清理
	for _, fakeIP := range toCleanup {
		m.mu.Lock()
		var entry *fakeIPEntry
		var exists bool
		if strings.Contains(fakeIP, ":") {
			entry, exists = m.activeV6[fakeIP]
		} else {
			entry, exists = m.active[fakeIP]
		}
		if !exists {
			m.mu.Unlock()
			continue
		}
		// 再次检查
		if time.Since(entry.GetLastAccess()) <= 2*time.Hour {
			m.mu.Unlock()
			continue
		}
		domain := entry.Domain
		if strings.Contains(fakeIP, ":") {
			delete(m.activeV6, fakeIP)
			delete(m.domainToIPV6, domain)
		} else {
			delete(m.active, fakeIP)
			delete(m.domainToIP, domain)
		}
		m.mu.Unlock()

		// 删除映射和防火墙规则
		if err := m.RemoveMapping(fakeIP); err != nil {
			base.Warn("Failed to remove FakeIP mapping:", err)
		}

		base.Debug("Cleaned up FakeIP:", fakeIP, "for domain:", domain)
	}
}

// 定期清理过期的 DNS 缓存
func (m *FakeDNSManager) cleanupDNSCacheLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanupExpiredDNSCache()
		case <-m.stopChan:
			return
		}
	}
}

// 清理过期的 DNS 缓存
func (m *FakeDNSManager) cleanupExpiredDNSCache() {
	var expired []string

	// 收集过期的 DNS 缓存
	m.dnsCacheMu.RLock()
	now := time.Now()
	for key, entry := range m.dnsCache {
		if now.After(entry.ExpireAt) {
			expired = append(expired, key)
		}
	}
	m.dnsCacheMu.RUnlock()

	if len(expired) == 0 {
		return
	}

	// 批量删除
	m.dnsCacheMu.Lock()
	for _, key := range expired {
		// 再次检查
		if entry, exists := m.dnsCache[key]; exists {
			if time.Now().After(entry.ExpireAt) {
				delete(m.dnsCache, key)
				base.Debug("Cleaned expired DNS cache:", key)
			}
		}
	}
	m.dnsCacheMu.Unlock()
}
