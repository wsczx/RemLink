package sessdata

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

var (
	// session_token -> SessUser
	sessions = make(map[string]*Session)
	// dtlsId -> session_token
	dtlsIds = make(map[string]string)
	sessMux sync.RWMutex
)

// 连接sess
type ConnSession struct {
	Sess                *Session
	MasterSecret        string        // dtls协议的 master_secret
	IpAddr              net.IP        // 分配的ip地址
	IpAddr6             net.IP        // 分配的 IPv6 地址（连通优先版，单 Ipv6CIDR 池；nil 表示纯 v4）
	IpPool              *ipPoolConfig // 组自定义 IP 池
	LocalIp             net.IP
	MacHw               net.HardwareAddr // 客户端mac地址,从Session取出
	Username            string
	Nickname            string
	RemoteAddr          string
	Mtu                 int
	IfName              string
	Client              string        // 客户端  mobile pc
	UserAgent           string        // 客户端信息
	userLogoutCode      atomic.Uint32 // 登出原因码，多 goroutine 并发写，只保留首次设置的值
	CstpDpd             int
	Group               *dbdata.Group
	Policy              *dbdata.Policy // 运行时生效策略（由 NewConn 解析）
	Limit               *LimitRater    // 下行限速器
	LimitUp             *LimitRater    // 上行限速器
	FakeDNS             *FakeDNSManager
	BandwidthUp         atomic.Uint64 // 本周期上行字节数（每 BandwidthPeriodSec 清零）
	BandwidthDown       atomic.Uint64 // 本周期下行字节数
	BandwidthUpPeriod   atomic.Uint64 // 上一周期每秒上行速率
	BandwidthDownPeriod atomic.Uint64 // 上一周期每秒下行速率
	BandwidthUpAll      atomic.Uint64 // 会话累计上行字节
	BandwidthDownAll    atomic.Uint64 // 会话累计下行字节
	trafficSettled      atomic.Uint64 // 已结算到 DB 的流量累计
	closeOnce           sync.Once
	rateWg              sync.WaitGroup // ratePeriod goroutine 退出信号
	CloseChan           chan struct{}
	LastDataTime        atomic.Int64 // 最后数据传输时间
	PayloadIn           chan *Payload
	PayloadOutCstp      chan *Payload // Cstp的数据
	PayloadOutDtls      chan *Payload // Dtls的数据
	// dSess *DtlsSession
	dSess *atomic.Value
	// compress
	CstpPickCmp CmpEncoding
	DtlsPickCmp CmpEncoding
}

type DtlsSession struct {
	isActive  int32
	CloseChan chan struct{}
	closeOnce sync.Once
	IpAddr    net.IP
}

type Session struct {
	mux             sync.RWMutex
	Sid             string // auth返回的 session-id
	Token           string // session信息的唯一token
	DtlsSid         string // dtls协议的 session_id
	MacAddr         string // 客户端mac地址
	UniqueIdGlobal  string // 客户端唯一标示
	MacHw           net.HardwareAddr
	UniqueMac       bool   // 客户端获取到真实设备mac
	Username        string // 用户名
	Group           string
	AuthStep        string
	AuthPass        string
	RemoteAddr      string
	UserAgent       string
	DeviceType      string
	PlatformVersion string

	LastLogin time.Time
	IsActive  bool
	LimitTime *time.Time // 用户过期时间，登录时从 user 表缓存；nil 表示永不过期

	// 开启 link 后设置
	CSess *ConnSession
}

func checkSession() {
	// 检测过期的session
	go func() {
		tick := time.NewTicker(time.Second * 60)
		for range tick.C {
			timeoutSeconds := base.GetCfg().SessionTimeout
			timeout := time.Duration(timeoutSeconds) * time.Second

			timeoutToken := []string{} // 会话超时
			expiredToken := []string{} // 用户已到期
			sessMux.RLock()
			t := time.Now()
			for k, v := range sessions {
				v.mux.RLock()
				if !v.IsActive {
					if timeoutSeconds != 0 && t.Sub(v.LastLogin) > timeout {
						timeoutToken = append(timeoutToken, k)
					}
				} else if v.LimitTime != nil && t.After(*v.LimitTime) {
					// 活跃会话：用户在线期间到期，踢下线
					expiredToken = append(expiredToken, k)
				}
				v.mux.RUnlock()
			}
			sessMux.RUnlock()

			// 删除过期session
			for _, v := range timeoutToken {
				CloseSess(v, dbdata.UserLogoutTimeout)
			}
			for _, v := range expiredToken {
				CloseSess(v, dbdata.UserLogoutExpire)
			}
		}
	}()
}
func UpdateUserLimitTime(username string, limitTime *time.Time) {
	expiredToken := []string{}
	sessMux.RLock()
	now := time.Now()
	for k, v := range sessions {
		v.mux.Lock()
		if v.Username == username {
			v.LimitTime = limitTime
			if v.IsActive && limitTime != nil && now.After(*limitTime) {
				expiredToken = append(expiredToken, k)
			}
		}
		v.mux.Unlock()
	}
	sessMux.RUnlock()

	for _, token := range expiredToken {
		CloseSess(token, dbdata.UserLogoutExpire)
	}
}

func NewSession(token string) *Session {
	if token == "" {
		token = utils.RandomHex(32)
	}

	// 生成 dtlsn session_id
	sess := &Session{
		Sid:       fmt.Sprintf("%d", time.Now().Unix()),
		Token:     token,
		DtlsSid:   utils.RandomHex(32),
		LastLogin: time.Now(),
	}

	sessMux.Lock()
	sessions[token] = sess
	dtlsIds[sess.DtlsSid] = token
	sessMux.Unlock()
	return sess
}

func (s *Session) NewConn() *ConnSession {
	s.mux.RLock()
	active := s.IsActive
	macAddr := s.MacAddr
	macHw := s.MacHw
	username := s.Username
	uniqueMac := s.UniqueMac
	s.mux.RUnlock()
	if active {
		// 同一 session token 重连，旧链接被顶替，不是真正的掉线
		s.CSess.SetLogoutCode(dbdata.UserLogoutReconn)
		s.CSess.Close()
	}

	limit := LimitClient(username, false)
	if !limit {
		base.Warn("limit is full", username)
		return nil
	}

	// 查询group信息
	group := &dbdata.Group{}
	err := dbdata.One("Name", s.Group, group)
	if err != nil {
		base.Error(err)
		LimitClient(username, true)
		return nil
	}

	// 获取组的 IP 池（组未配置则回退全局）
	ipPool := GetGroupIpPool(group)
	ip := AcquireIpWithRange(username, macAddr, uniqueMac, ipPool)
	if ip == nil {
		LimitClient(username, true)
		return nil
	}
	// IPv6 地址（组配置了 v6 段则从组池分配，否则全局 Ipv6CIDR 池；分配失败则降级为纯 v4）
	var ip6 net.IP
	if base.GetCfg().Ipv6CIDR != "" {
		ip6 = acquireIpV6(username, macAddr, uniqueMac, ipPool)
		if ip6 == nil {
			base.Warn("IPv6 地址池分配失败，客户端将以纯 v4 接入:", username)
		}
	}
	// 查询user信息
	user := &dbdata.User{}
	dbdata.One("username", username, user) // 外部用户可能不存在于本地 DB
	// 不判断错误

	cSess := &ConnSession{
		Sess:           s,
		MacHw:          macHw,
		Username:       username,
		Nickname:       user.Nickname,
		Mtu:            user.Mtu,
		IpAddr:         ip,
		IpAddr6:        ip6,
		IpPool:         ipPool,
		closeOnce:      sync.Once{},
		CloseChan:      make(chan struct{}),
		PayloadIn:      make(chan *Payload, 256),
		PayloadOutCstp: make(chan *Payload, 256),
		PayloadOutDtls: make(chan *Payload, 256),
		dSess:          &atomic.Value{},
	}
	// IPv6 要求链路 MTU ≥ 1280，否则触发 v6 PMTU 黑洞（见 ipv6-dual-stack-design.md §4）。
	// 用户级 Mtu 覆盖（非 0）会绕过下方 link_tunnel 的 SetMtu，故此处单独强制下限。
	if base.GetCfg().Ipv6CIDR != "" && cSess.Mtu != 0 && cSess.Mtu < 1280 {
		base.Warn("用户级 Mtu=", cSess.Mtu, " 低于 IPv6 要求下限 1280，已自动上调到 1280 (user=", username, ")")
		cSess.Mtu = 1280
	}

	cSess.LastDataTime.Store(time.Now().Unix())

	dSess := &DtlsSession{
		isActive: -1,
	}
	cSess.dSess.Store(dSess)

	cSess.Group = group

	// 加载策略：用户个人策略优先于组策略
	cSess.Policy = dbdata.ApplyPolicy(username, group)
	if cSess.Policy == nil {
		base.Error("策略加载失败，拒绝连接:", username, group.Name)
		ReleaseIp(ip, ip6, macAddr)
		LimitClient(username, true)
		return nil
	}

	if exceeded, used := dbdata.QuotaExceeded(username, cSess.Policy); exceeded {
		base.Warn("流量配额已超，拒绝连接:", username, "used:", used, "quota:", cSess.Policy.TrafficQuota)
		ReleaseIp(ip, ip6, macAddr)
		LimitClient(username, true)
		return nil
	}

	if cSess.Policy.Bandwidth > 0 {
		cSess.Limit = NewLimitRater(cSess.Policy.Bandwidth)
	}
	if cSess.Policy.BandwidthUp > 0 {
		cSess.LimitUp = NewLimitRater(cSess.Policy.BandwidthUp)
	}
	if cSess.Policy.EnableFakeDNS {
		cSess.FakeDNS = GetFakeDNSManager()
	}
	cSess.rateWg.Add(1)
	go cSess.ratePeriod()

	s.mux.Lock()
	s.MacAddr = macAddr
	s.IsActive = true
	s.CSess = cSess
	s.mux.Unlock()
	return cSess
}

// SetLogoutCode 记录登出原因。只有首次设置生效：具体原因（主动断开、配额超限等）
// 总是先于 defer 里的兜底原因写入，后到的兜底不应覆盖它。
func (cs *ConnSession) SetLogoutCode(code uint8) {
	// code+1 存储，使零值可区分「未设置」与 UserLogoutLose(0)
	cs.userLogoutCode.CompareAndSwap(0, uint32(code)+1)
}

// LogoutCode 返回登出原因码，未设置过则返回 UserLogoutLose。
func (cs *ConnSession) LogoutCode() uint8 {
	v := cs.userLogoutCode.Load()
	if v == 0 {
		return dbdata.UserLogoutLose
	}
	return uint8(v - 1)
}

func (cs *ConnSession) Close() {
	cs.closeOnce.Do(func() {
		base.Debug("closeOnce:", cs.IpAddr)
		loginTime := cs.Sess.LastLogin

		cs.Sess.mux.Lock()
		close(cs.CloseChan)
		cs.Sess.IsActive = false
		cs.Sess.LastLogin = time.Now()
		cs.Sess.CSess = nil
		cs.Sess.mux.Unlock()

		dSess := cs.GetDtlsSession()
		if dSess != nil {
			dSess.Close()
		}

		ReleaseIp(cs.IpAddr, cs.IpAddr6, cs.Sess.MacAddr)
		LimitClient(cs.Username, true)
		cs.rateWg.Wait() // 确保 ratePeriod 退出后再结算，避免流量丢失
		cs.settleTraffic()
		AddUserActLog(cs, loginTime)
	})
}

// 创建dtls链接
func (cs *ConnSession) NewDtlsConn() *DtlsSession {
	ds := cs.dSess.Load().(*DtlsSession)
	isActive := atomic.LoadInt32(&ds.isActive)
	if isActive > 0 {
		return nil // 已有活跃 DTLS 连接
	}

	dSess := &DtlsSession{
		isActive:  1,
		CloseChan: make(chan struct{}),
		closeOnce: sync.Once{},
		IpAddr:    cs.IpAddr,
	}
	cs.dSess.Store(dSess)
	return dSess
}

// 关闭dtls链接
func (ds *DtlsSession) Close() {
	ds.closeOnce.Do(func() {
		base.Debug("closeOnce dtls:", ds.IpAddr)
		atomic.StoreInt32(&ds.isActive, -1)
		close(ds.CloseChan)
	})
}

func (cs *ConnSession) GetDtlsSession() *DtlsSession {
	if cs.dSess == nil || cs.dSess.Load() == nil {
		return nil
	}
	ds := cs.dSess.Load().(*DtlsSession)
	isActive := atomic.LoadInt32(&ds.isActive)
	if isActive > 0 {
		return ds
	}
	return nil
}

const BandwidthPeriodSec = 10 // 流量速率统计周期(秒)
const QuotaCheckSec = 60      // 流量配额检查周期(秒)

func (cs *ConnSession) ratePeriod() {
	defer cs.rateWg.Done()
	tick := time.NewTicker(time.Second * BandwidthPeriodSec)
	defer tick.Stop()
	var quotaTick int

	for range tick.C {
		select {
		case <-cs.CloseChan:
			return
		default:
		}

		// 速率统计：清零本周期计数，更新每秒速率，累加会话总量
		rtUp := cs.BandwidthUp.Swap(0)
		rtDown := cs.BandwidthDown.Swap(0)
		cs.BandwidthUpPeriod.Swap(rtUp / BandwidthPeriodSec)
		cs.BandwidthDownPeriod.Swap(rtDown / BandwidthPeriodSec)
		cs.BandwidthUpAll.Add(rtUp)
		cs.BandwidthDownAll.Add(rtDown)

		// 配额检查：独立周期（QuotaCheckSec），减少 DB 压力
		if cs.Policy == nil || cs.Policy.TrafficQuota <= 0 {
			continue
		}
		quotaTick++
		if quotaTick*BandwidthPeriodSec < QuotaCheckSec {
			continue
		}
		quotaTick = 0
		cs.settleTraffic()
		if exceeded, _ := dbdata.QuotaExceeded(cs.Username, cs.Policy); exceeded {
			base.Warn("流量配额超限，强制下线:", cs.Username)
			go CloseSess(cs.Sess.Token, dbdata.UserLogoutQuota) // 异步避免死锁
			return
		}
	}
}

// 把未入库的流量写回 DB，CAS 保证并发安全
func (cs *ConnSession) settleTraffic() {
	if cs.Policy == nil || cs.Policy.TrafficQuota <= 0 {
		return
	}
	totalAll := cs.BandwidthUpAll.Load() + cs.BandwidthDownAll.Load()
	for {
		settled := cs.trafficSettled.Load()
		if totalAll <= settled {
			return
		}
		if cs.trafficSettled.CompareAndSwap(settled, totalAll) {
			dbdata.AddTrafficUsed(cs.Username, int64(totalAll-settled))
			return
		}
	}
}

var MaxMtu = 1460

func (cs *ConnSession) SetMtu(mtu string) {
	if base.GetCfg().Mtu > 0 {
		MaxMtu = base.GetCfg().Mtu
	}
	cs.Mtu = MaxMtu

	mi, err := strconv.Atoi(mtu)
	if err != nil || mi < 100 {
		enforceMtuFloorV6(cs)
		return
	}

	if mi < MaxMtu {
		cs.Mtu = mi
	}
	enforceMtuFloorV6(cs)
}

// enforceMtuFloorV6 在双栈开启时保证链路 MTU 不低于 IPv6 要求的 1280。
// 纯 v4 时返回 0 下限，保持字节级不变（客户端可请求更低 MTU）。
func enforceMtuFloorV6(cs *ConnSession) {
	if base.GetCfg().Ipv6CIDR == "" {
		return
	}
	if cs.Mtu < 1280 {
		base.Warn("链路 MTU=", cs.Mtu, " 低于 IPv6 要求下限 1280，已自动上调到 1280")
		cs.Mtu = 1280
	}
}

func (cs *ConnSession) SetIfName(name string) {
	cs.Sess.mux.Lock()
	defer cs.Sess.mux.Unlock()
	cs.IfName = name
}

func (cs *ConnSession) RateLimit(byt int, isUp bool) error {
	if isUp {
		cs.BandwidthUp.Add(uint64(byt))
		if cs.LimitUp != nil {
			return cs.LimitUp.Wait(byt)
		}
		return nil
	}
	cs.BandwidthDown.Add(uint64(byt))
	if cs.Limit == nil {
		return nil
	}
	return cs.Limit.Wait(byt)
}

func (cs *ConnSession) SetPickCmp(cate, encoding string) (string, bool) {
	var cmpName string
	if !base.GetCfg().Compression {
		return cmpName, false
	}
	var cmp CmpEncoding
	switch {
	// case strings.Contains(encoding, "oc-lz4"):
	// 	cmpName = "oc-lz4"
	// 	cmp = Lz4Cmp{}
	case strings.Contains(encoding, "lzs"):
		cmpName = "lzs"
		cmp = LzsgoCmp{}
	default:
		return cmpName, false
	}
	if cate == "cstp" {
		cs.CstpPickCmp = cmp
	} else {
		cs.DtlsPickCmp = cmp
	}
	return cmpName, true
}

func SToken2Sess(stoken string) *Session {
	stoken = strings.TrimSpace(stoken)
	sarr := strings.SplitN(stoken, "@", 2)
	if len(sarr) != 2 || sarr[1] == "" {
		return nil
	}

	return Token2Sess(sarr[1])
}

func Token2Sess(token string) *Session {
	sessMux.RLock()
	defer sessMux.RUnlock()
	return sessions[token]
}

func Dtls2Sess(did string) *Session {
	sessMux.RLock()
	defer sessMux.RUnlock()
	token := dtlsIds[did]
	return sessions[token]
}

func Dtls2CSess(did string) *ConnSession {
	sessMux.RLock()
	defer sessMux.RUnlock()
	token := dtlsIds[did]
	sess := sessions[token]
	if sess == nil {
		return nil
	}

	sess.mux.RLock()
	defer sess.mux.RUnlock()
	return sess.CSess
}

func Dtls2MasterSecret(did string) string {
	// 全程持有 sessMux 读锁，避免释放后会话被并发关闭
	sessMux.RLock()
	defer sessMux.RUnlock()
	token := dtlsIds[did]
	sess := sessions[token]
	if sess == nil {
		return ""
	}

	sess.mux.RLock()
	defer sess.mux.RUnlock()
	if sess.CSess == nil {
		return ""
	}
	return sess.CSess.MasterSecret
}

func CloseSess(token string, code ...uint8) {
	sessMux.Lock()
	defer sessMux.Unlock()
	sess, ok := sessions[token]
	if !ok {
		return
	}

	delete(sessions, token)
	delete(dtlsIds, sess.DtlsSid)

	if sess.CSess != nil {
		if len(code) > 0 {
			sess.CSess.SetLogoutCode(code[0])
		}
		sess.CSess.Close()
		return
	}
	AddUserActLogBySess(sess, code...)
}

func CloseCSess(token string) {
	sessMux.RLock()
	defer sessMux.RUnlock()
	sess, ok := sessions[token]
	if !ok {
		return
	}

	if sess.CSess != nil {
		sess.CSess.Close()
	}
}

func DelSessByStoken(stoken string) {
	stoken = strings.TrimSpace(stoken)
	sarr := strings.SplitN(stoken, "@", 2)
	if len(sarr) != 2 || sarr[1] == "" {
		return
	}
	CloseSess(sarr[1], dbdata.UserLogoutBanner)
}

// 记录用户下线日志，包含登出原因、上下行流量和在线时长
func AddUserActLog(cs *ConnSession, loginTime time.Time) {
	ua := dbdata.UserActLog{
		Username:        cs.Sess.Username,
		GroupName:       cs.Sess.Group,
		IpAddr:          cs.IpAddr.String(),
		RemoteAddr:      cs.RemoteAddr,
		DeviceType:      cs.Sess.DeviceType,
		PlatformVersion: cs.Sess.PlatformVersion,
		Status:          dbdata.UserLogout,
	}
	info := dbdata.UserActLogIns.GetInfoOpsById(cs.LogoutCode())
	// 追加流量和在线时长信息
	duration := time.Since(loginTime)
	upStr := utils.HumanByte(cs.BandwidthUpAll.Load())
	downStr := utils.HumanByte(cs.BandwidthDownAll.Load())
	durStr := formatDuration(duration)
	info = fmt.Sprintf("%s | 上行:%s 下行:%s | 在线:%s", info, upStr, downStr, durStr)
	ua.Info = info
	dbdata.UserActLogIns.Add(ua, cs.UserAgent)
}

func AddUserActLogBySess(sess *Session, code ...uint8) {
	ua := dbdata.UserActLog{
		Username:        sess.Username,
		GroupName:       sess.Group,
		IpAddr:          "",
		RemoteAddr:      sess.RemoteAddr,
		DeviceType:      sess.DeviceType,
		PlatformVersion: sess.PlatformVersion,
		Status:          dbdata.UserLogout,
	}
	ua.Info = dbdata.UserActLogIns.GetInfoOpsById(dbdata.UserLogoutBanner)
	if len(code) > 0 {
		ua.Info = dbdata.UserActLogIns.GetInfoOpsById(code[0])
	}
	dbdata.UserActLogIns.Add(ua, sess.UserAgent)
}

// 将时长格式化为可读字符串
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", h, m)
}
