package auth

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/wsczx/remlink/base"
)

// 锁定信息（供管理后台查询）
type LockInfo struct {
	Description string     `json:"description"`
	Username    string     `json:"username"`
	Nickname    string     `json:"nickname"`
	IP          string     `json:"ip"`
	State       *LockState `json:"state"`
}

// 锁定状态
type LockState struct {
	Locked       bool      `json:"locked"`
	FailureCount int       `json:"attempts"`
	LockTime     time.Time `json:"lock_time"`
	LastAttempt  time.Time `json:"lastAttempt"`
}

// IP 名单类型
type IPListType int

const (
	IPWhiteList IPListType = iota
	IPBlackList
)

// 防暴力破解管理器（全局单例）
type LockManager struct {
	mu sync.Mutex

	ipLocks        map[string]*LockState
	userLocks      map[string]*LockState
	ipUserLocks    map[string]map[string]*LockState
	challengeLocks map[string]map[string]*LockState // ip -> username -> 挑战阶段（OTP 等二次验证）失败计数

	ipLists       map[IPListType][]ipListItem
	cleanupTicker *time.Ticker
	cleanupDone   chan struct{}
	cleanupOnce   sync.Once
	warnLimiter   *base.WarnLimiter
}

type ipListItem struct {
	IP   net.IP
	CIDR *net.IPNet
}

var (
	lm     *LockManager
	lmOnce sync.Once
)

// 获取全局 LockManager 单例
func GetLockManager() *LockManager {
	lmOnce.Do(func() {
		lm = &LockManager{
			ipLocks:        make(map[string]*LockState),
			userLocks:      make(map[string]*LockState),
			ipUserLocks:    make(map[string]map[string]*LockState),
			challengeLocks: make(map[string]map[string]*LockState),
			ipLists:        make(map[IPListType][]ipListItem),
			warnLimiter:    base.NewWarnLimiter(warnLogInterval),
		}
	})
	return lm
}

// 默认锁定过期时间（秒）
const defaultExpireTime = 3600

// 锁定模块内部告警日志的限频窗口：同一 key 在该窗口内最多输出一次
const warnLogInterval = 60 * time.Second

// 日志限频 key 前缀。仅作为 WarnLimiter 内部 map 的命名空间，用于区分不同类别的告警
// 需保证不同类别之间保持彼此不同（否则会共享同一个限流窗口）
const (
	warnKeyExtractIP  = "extract_ip:"
	warnKeyBlacklist  = "blacklist:"
	warnKeyGlobalIP   = "giplock:"
	warnKeyGlobalUser = "guplock:"
	warnKeyUserIP     = "uiplock:"
	warnKeyChallenge  = "chglock:"
)

// 初始化并启动清理协程（服务启动时调用一次）
func (m *LockManager) Init() {
	base.UpdateCfg(func(cfg *base.ServerConfig) {
		if cfg.GlobalLockStateExpirationTime <= 0 {
			cfg.GlobalLockStateExpirationTime = defaultExpireTime
		}
	})
	m.LoadIPList(IPWhiteList, base.GetCfg().IPWhiteList)
	m.LoadIPList(IPBlackList, base.GetCfg().IPBlackList)
	m.startCleanup()
}

// 热加载 IP 名单（配置变更时调用）
func (m *LockManager) LoadIPList(listType IPListType, config string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.ipLists, listType)

	for item := range strings.SplitSeq(config, ",") {
		// 同时支持换行分隔
		for line := range strings.SplitSeq(item, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			li := ipListItem{}
			if _, ipNet, err := net.ParseCIDR(line); err == nil {
				li.CIDR = ipNet
			} else if ip := net.ParseIP(line); ip != nil {
				li.IP = ip
			} else {
				continue
			}
			m.ipLists[listType] = append(m.ipLists[listType], li)
		}
	}
}

// 限频后输出日志：同一 key 在 warnLogInterval 内仅放行一次
func (m *LockManager) warnRateLimited(key string, fn func()) {
	if m.warnLimiter.Allow(key, time.Now()) {
		fn()
	}
}

// 检查用户名和 IP 是否被锁定。返回 true 允许继续
func (m *LockManager) Check(username, ipaddr string) bool {
	ip, _, err := net.SplitHostPort(ipaddr)
	if err != nil {
		m.warnRateLimited(warnKeyExtractIP+ipaddr, func() { base.Error("提取 IP 地址失败，拒绝访问:", ipaddr) })
		return false
	}

	// 黑名单优先于白名单：显式拒绝优先，避免管理员"加黑"被白名单覆盖无视
	if m.InList(ip, IPBlackList) {
		m.warnRateLimited(warnKeyBlacklist+ip, func() { base.Warn("IP", ip, "在黑名单中，拒绝访问") })
		return false
	}
	if m.InList(ip, IPWhiteList) {
		return true
	}

	if !base.GetCfg().AntiBruteForce {
		return true
	}

	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := base.GetCfg()
	if cfg.MaxGlobalIPBanCount > 0 && m.isIPLocked(ip, now) {
		m.warnRateLimited(warnKeyGlobalIP+ip, func() { base.Warn("IP", ip, "全局锁定") })
		return false
	}
	if username != "" && cfg.MaxGlobalUserBanCount > 0 && m.isUserLocked(username, now) {
		m.warnRateLimited(warnKeyGlobalUser+username, func() { base.Warn("用户", username, "全局锁定") })
		return false
	}
	if username != "" && cfg.MaxBanCount > 0 && m.isUserIPLocked(username, ip, now) {
		m.warnRateLimited(warnKeyUserIP+ip+":"+username, func() { base.Warn("IP", ip, "对用户", username, "已锁定") })
		return false
	}
	// 挑战阶段（OTP 等二次验证）锁定：独立于首因子计数
	// 首因子成功（StageSuccess）不清除，只有完整登录成功才清除
	if username != "" && cfg.MaxBanCount > 0 && m.isChallengeLocked(username, ip, now) {
		m.warnRateLimited(warnKeyChallenge+ip+":"+username, func() { base.Warn("IP", ip, "对用户", username, "挑战阶段已锁定") })
		return false
	}
	return true
}

// 客户端 IP 是否在黑名单中，供审计日志区分「IP 黑名单」与「IP 暴力锁定」
func (m *LockManager) InBlackList(ipaddr string) bool {
	host, _, err := net.SplitHostPort(ipaddr)
	if err != nil {
		host = ipaddr
	}
	return m.InList(host, IPBlackList)
}

// 登录成功后清除锁定计数（含挑战阶段计数）
func (m *LockManager) Success(username, ipaddr string) {
	m.update(username, ipaddr, true)
	m.clearChallenge(username, ipaddr)
}

// 阶段成功（首因子通过、进入二次挑战）：仅清首因子计数，不清挑战计数——
// 防已知密码者反复登录无限重置 OTP 尝试
func (m *LockManager) StageSuccess(username, ipaddr string) {
	m.update(username, ipaddr, true)
}

// 挑战阶段失败（如 OTP 验证码错误）：按 (ip, username) 独立计数
// 阈值与首因子阶段一致（MaxBanCount/BanResetTime/LockTime）
func (m *LockManager) ChallengeFail(username, ipaddr string) {
	ip, _, err := net.SplitHostPort(ipaddr)
	if err != nil {
		m.warnRateLimited(warnKeyExtractIP+ipaddr, func() { base.Error("提取 IP 地址失败:", ipaddr) })
		return
	}
	if username == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := base.GetCfg()
	if cfg.MaxBanCount <= 0 {
		return
	}
	ls := m.getChallengeLS(ip, username)
	bumpLockState(ls, cfg.MaxBanCount, cfg.BanResetTime, cfg.LockTime, time.Now())
}

// 手动解锁用户
func (m *LockManager) UnlockUser(username string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.userLocks, username)
	// 挑战阶段计数跨 IP 一并清除
	for ip, um := range m.challengeLocks {
		delete(um, username)
		if len(um) == 0 {
			delete(m.challengeLocks, ip)
		}
	}
}

// 登录失败后增加锁定计数
func (m *LockManager) Fail(username, ipaddr string) {
	m.update(username, ipaddr, false)
}

// 手动解锁 IP（仅解「全局IP锁定」）
func (m *LockManager) UnlockIP(ipaddr string) {
	host, _, _ := net.SplitHostPort(ipaddr)
	if host == "" {
		host = ipaddr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.ipLocks, host)
}

// 手动解锁指定用户在指定 IP 上的锁定（单用户IP锁定 + 挑战阶段锁定），
func (m *LockManager) UnlockUserIP(username, ipaddr string) {
	host, _, _ := net.SplitHostPort(ipaddr)
	if host == "" {
		host = ipaddr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if um, ok := m.ipUserLocks[host]; ok {
		delete(um, username)
		if len(um) == 0 {
			delete(m.ipUserLocks, host)
		}
	}
	if um, ok := m.challengeLocks[host]; ok {
		delete(um, username)
		if len(um) == 0 {
			delete(m.challengeLocks, host)
		}
	}
}

// 返回所有锁定状态（供管理后台查询）
func (m *LockManager) LockInfo() []LockInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := base.GetCfg()
	var infos []LockInfo
	if cfg.MaxGlobalIPBanCount > 0 {
		for ip, s := range m.ipLocks {
			if s.Locked {
				infos = append(infos, LockInfo{
					Description: "全局IP锁定", IP: ip,
					State: s.copy(),
				})
			}
		}
	}
	if cfg.MaxGlobalUserBanCount > 0 {
		for user, s := range m.userLocks {
			if s.Locked {
				infos = append(infos, LockInfo{
					Description: "全局用户锁定", Username: user,
					State: s.copy(),
				})
			}
		}
	}
	if cfg.MaxBanCount > 0 {
		for ip, userStates := range m.ipUserLocks {
			for username, s := range userStates {
				if s.Locked {
					infos = append(infos, LockInfo{
						Description: "单用户IP锁定", Username: username, IP: ip,
						State: s.copy(),
					})
				}
			}
		}
		// 挑战阶段锁定（OTP 等二次验证失败累积）
		for ip, userStates := range m.challengeLocks {
			for username, s := range userStates {
				if s.Locked {
					infos = append(infos, LockInfo{
						Description: "挑战阶段锁定", Username: username, IP: ip,
						State: s.copy(),
					})
				}
			}
		}
	}
	return infos
}

// 检查 IP 是否在指定名单中
func (m *LockManager) InList(ip string, listType IPListType) bool {
	clientIP := net.ParseIP(ip)
	if clientIP == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, item := range m.ipLists[listType] {
		if item.CIDR != nil && item.CIDR.Contains(clientIP) {
			return true
		}
		if item.IP != nil && item.IP.Equal(clientIP) {
			return true
		}
	}
	return false
}

func (m *LockManager) update(username, ipaddr string, success bool) {
	ip, _, err := net.SplitHostPort(ipaddr)
	if err != nil {
		m.warnRateLimited(warnKeyExtractIP+ipaddr, func() { base.Error("提取 IP 地址失败:", ipaddr) })
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if success {
		// 登录成功，清除锁定计数
		m.clearUserIP(ip, username)
		if s := m.ipLocks[ip]; s != nil {
			s.FailureCount = 0
			s.Locked = false
			s.LockTime = time.Time{}
		}
		if username != "" {
			if s := m.userLocks[username]; s != nil {
				s.FailureCount = 0
				s.Locked = false
				s.LockTime = time.Time{}
			}
		}
		return
	}

	now := time.Now()
	cfg := base.GetCfg()
	if cfg.MaxBanCount > 0 {
		m.incrLock(ip, username, cfg.MaxBanCount, cfg.BanResetTime, cfg.LockTime, now, true)
	}
	if cfg.MaxGlobalUserBanCount > 0 {
		m.incrLock("", username, cfg.MaxGlobalUserBanCount, cfg.GlobalUserBanResetTime, cfg.GlobalUserLockTime, now, false)
	}
	if cfg.MaxGlobalIPBanCount > 0 {
		m.incrLock(ip, "", cfg.MaxGlobalIPBanCount, cfg.GlobalIPBanResetTime, cfg.GlobalIPLockTime, now, false)
	}
}

func (m *LockManager) incrLock(ip, username string, maxCount, resetTime, lockTime int, now time.Time, isUserIP bool) {
	var ls *LockState

	if isUserIP {
		if _, ok := m.ipUserLocks[ip]; !ok {
			m.ipUserLocks[ip] = make(map[string]*LockState)
		}
		ls = m.ipUserLocks[ip][username]
		if ls == nil {
			ls = &LockState{}
			m.ipUserLocks[ip][username] = ls
		}
	} else if username != "" {
		ls = m.userLocks[username]
		if ls == nil {
			ls = &LockState{}
			m.userLocks[username] = ls
		}
	} else {
		ls = m.ipLocks[ip]
		if ls == nil {
			ls = &LockState{}
			m.ipLocks[ip] = ls
		}
	}

	bumpLockState(ls, maxCount, resetTime, lockTime, now)
}

// 累加一个 LockState 的失败计数（锁定状态机，首因子与挑战阶段共用）
// 锁定到期自动重置；锁定期间忽略；超过 resetTime 未尝试则计数归零
// 达到 maxCount 则锁定 lockTime 秒
func bumpLockState(ls *LockState, maxCount, resetTime, lockTime int, now time.Time) {
	if ls.Locked && !ls.LockTime.IsZero() && now.After(ls.LockTime) {
		ls.FailureCount = 0
		ls.Locked = false
		ls.LockTime = time.Time{}
	}

	if ls.Locked || ls.LockTime.After(now) {
		return
	}
	if now.Sub(ls.LastAttempt) > time.Duration(resetTime)*time.Second {
		ls.FailureCount = 0
	}
	ls.FailureCount++
	ls.LastAttempt = now
	if ls.FailureCount >= maxCount {
		ls.LockTime = now.Add(time.Duration(lockTime) * time.Second)
		ls.Locked = true
	}
}

func (m *LockManager) isUserLocked(username string, now time.Time) bool {
	return m.checkState(m.userLocks[username], now, base.GetCfg().GlobalUserBanResetTime)
}

func (m *LockManager) isIPLocked(ip string, now time.Time) bool {
	return m.checkState(m.ipLocks[ip], now, base.GetCfg().GlobalIPBanResetTime)
}

func (m *LockManager) isUserIPLocked(username, ip string, now time.Time) bool {
	if um, ok := m.ipUserLocks[ip]; ok {
		return m.checkState(um[username], now, base.GetCfg().BanResetTime)
	}
	return false
}

func (m *LockManager) isChallengeLocked(username, ip string, now time.Time) bool {
	if um, ok := m.challengeLocks[ip]; ok {
		return m.checkState(um[username], now, base.GetCfg().BanResetTime)
	}
	return false
}

// 获取（不存在则创建）某 (ip, username) 的挑战阶段计数。调用方须持有 m.mu
func (m *LockManager) getChallengeLS(ip, username string) *LockState {
	um, ok := m.challengeLocks[ip]
	if !ok {
		um = make(map[string]*LockState)
		m.challengeLocks[ip] = um
	}
	ls := um[username]
	if ls == nil {
		ls = &LockState{}
		um[username] = ls
	}
	return ls
}

// 清除某 (ip, username) 的挑战阶段计数（完整登录成功时调用）
func (m *LockManager) clearChallenge(username, ipaddr string) {
	ip, _, err := net.SplitHostPort(ipaddr)
	if err != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if um, ok := m.challengeLocks[ip]; ok {
		delete(um, username)
		if len(um) == 0 {
			delete(m.challengeLocks, ip)
		}
	}
}

func (m *LockManager) checkState(ls *LockState, now time.Time, resetTime int) bool {
	if ls == nil || ls.LastAttempt.IsZero() {
		return false
	}
	if ls.Locked && !ls.LockTime.IsZero() && now.After(ls.LockTime) {
		ls.FailureCount = 0
		ls.Locked = false
		ls.LockTime = time.Time{}
		return false
	}
	if now.Sub(ls.LastAttempt) > time.Duration(resetTime)*time.Second && !ls.Locked {
		ls.FailureCount = 0
	}
	return ls.Locked
}

func (m *LockManager) clearUserIP(ip, username string) {
	if um, ok := m.ipUserLocks[ip]; ok {
		if ls := um[username]; ls != nil {
			ls.FailureCount = 0
			ls.Locked = false
			ls.LockTime = time.Time{}
		}
	}
}

func (s *LockState) copy() *LockState {
	c := *s
	return &c
}

func (m *LockManager) startCleanup() {
	m.cleanupOnce.Do(func() {
		m.mu.Lock()
		ticker := time.NewTicker(time.Minute)
		done := make(chan struct{})
		m.cleanupTicker = ticker
		m.cleanupDone = done
		m.mu.Unlock()
		base.SafeGo(func() {
			for {
				select {
				case <-ticker.C:
					m.cleanup()
				case <-done:
					ticker.Stop()
					return
				}
			}
		})
	})
}
func (m *LockManager) Stop() {
	// 加锁读取 cleanupDone：与 startCleanup 的写构成并发，无锁读有数据竞争
	m.mu.Lock()
	done := m.cleanupDone
	m.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		default:
			close(done)
		}
	}
}

func (m *LockManager) cleanup() {
	now := time.Now()
	expireTime := time.Duration(base.GetCfg().GlobalLockStateExpirationTime) * time.Second

	m.mu.Lock()
	defer m.mu.Unlock()

	for ip, s := range m.ipLocks {
		if s.Locked && !s.LockTime.IsZero() && now.After(s.LockTime) {
			s.FailureCount = 0
			s.Locked = false
			s.LockTime = time.Time{}
			continue
		}
		if now.Sub(s.LastAttempt) > expireTime && !s.Locked {
			delete(m.ipLocks, ip)
		}
	}
	for user, s := range m.userLocks {
		if s.Locked && !s.LockTime.IsZero() && now.After(s.LockTime) {
			s.FailureCount = 0
			s.Locked = false
			s.LockTime = time.Time{}
			continue
		}
		if now.Sub(s.LastAttempt) > expireTime && !s.Locked {
			delete(m.userLocks, user)
		}
	}
	for ip, userMap := range m.ipUserLocks {
		for username, s := range userMap {
			if s.Locked && !s.LockTime.IsZero() && now.After(s.LockTime) {
				s.FailureCount = 0
				s.Locked = false
				s.LockTime = time.Time{}
				continue
			}
			if now.Sub(s.LastAttempt) > expireTime && !s.Locked {
				delete(userMap, username)
			}
		}
		if len(userMap) == 0 {
			delete(m.ipUserLocks, ip)
		}
	}
	for ip, userMap := range m.challengeLocks {
		for username, s := range userMap {
			if s.Locked && !s.LockTime.IsZero() && now.After(s.LockTime) {
				s.FailureCount = 0
				s.Locked = false
				s.LockTime = time.Time{}
				continue
			}
			if now.Sub(s.LastAttempt) > expireTime && !s.Locked {
				delete(userMap, username)
			}
		}
		if len(userMap) == 0 {
			delete(m.challengeLocks, ip)
		}
	}
	// 回收日志限频表，防止不同 key 持续累积导致内存增长
	m.warnLimiter.Clear(now)
}
