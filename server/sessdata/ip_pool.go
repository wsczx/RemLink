package sessdata

import (
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

const ipPoolUserQueryLimit = 30 // IP 池按用户查询的行数上限（FindWhere 分页）

// 管理全局与组级 IPv4/IPv6 地址池的分配与回收，由 App 经 Pools 注入
type IPPool struct {
	mu         sync.Mutex
	active     map[string]bool // ip 字符串 -> 是否被活跃连接占用
	groupMux   sync.Mutex
	groupCache map[string]*ipPoolConfig // 组池配置缓存
	Global     *ipPoolConfig            // 全局地址池
}

func NewIPPool() *IPPool {
	return &IPPool{
		active:     map[string]bool{},
		groupCache: map[string]*ipPoolConfig{},
		Global:     &ipPoolConfig{},
	}
}

type ipPoolConfig struct {
	// 计算动态ip
	Ipv4Gateway net.IP
	Ipv4Mask    net.IP
	Ipv4IPNet   *net.IPNet
	IpLongMin   uint32
	IpLongMax   uint32
	loopCurIp   uint32        // 每池独立的循环游标
	loopFarIp   *dbdata.IpMap // 每池独立的最早登录记录
	GroupName   string        // 所属组名；全局池为空。用于 IpMap 按 (mac_addr, ip_group) 定位

	// IPv6 全局地址池（单 Ipv6CIDR 自动分配，/128 每客户端）
	Ipv6IPNet   *net.IPNet
	Ipv6Gateway net.IP
	ipv6Start   *big.Int
	ipv6End     *big.Int
	ipv6Cursor  *big.Int
}

func (p *IPPool) GetGroupPool(group *dbdata.Group) *ipPoolConfig {
	// 优先使用组级别配置
	if group != nil && group.ClientCidr != "" && group.ClientStart != "" && group.ClientEnd != "" && group.ClientGateway != "" {
		// 相同网段的不同组若共享池对象，GroupName 会错乱，
		// 导致 IpMap 按 (mac_addr, ip_group) 定位到错误记录
		cacheKey := group.Name + "|" + group.ClientCidr + "|" + group.ClientStart + "|" + group.ClientEnd + "|" + group.ClientGateway + "|" + group.ClientCidr6

		p.groupMux.Lock()
		defer p.groupMux.Unlock()
		if cfg, ok := p.groupCache[cacheKey]; ok {
			return cfg // 复用，游标得以跨连接累积
		}

		_, ipNet, err := net.ParseCIDR(group.ClientCidr)
		if err != nil {
			base.Warn("组", group.Name, "IP网段配置无效，回退到全局:", err)
		} else {
			start := net.ParseIP(group.ClientStart)
			end := net.ParseIP(group.ClientEnd)
			gateway := net.ParseIP(group.ClientGateway)
			if start != nil && end != nil && gateway != nil &&
				ipNet.Contains(start) && ipNet.Contains(end) && ipNet.Contains(gateway) {
				cfg := &ipPoolConfig{
					Ipv4Gateway: gateway,
					Ipv4Mask:    net.IP(ipNet.Mask),
					Ipv4IPNet:   ipNet,
					IpLongMin:   utils.Ip2long(start),
					IpLongMax:   utils.Ip2long(end),
					GroupName:   group.Name,
				}
				cfg.loopCurIp = cfg.IpLongMin
				// 组级 v6 段：受全局 Ipv6CIDR 总开关约束，未开总开关则忽略组 v6
				if group.ClientCidr6 != "" && base.GetCfg().Ipv6CIDR != "" {
					if err6 := cfg.initV6(group.ClientCidr6); err6 != nil {
						base.Warn("组", group.Name, "IPv6网段配置无效，该组使用全局 v6 池:", err6)
					}
				}
				p.groupCache[cacheKey] = cfg
				return cfg
			}
			base.Warn("组", group.Name, "IP范围不在网段内，回退到全局")
		}
	}

	// 回退到全局配置
	return p.Global
}

// 按单 CIDR 自动分配规则初始化池的 v6 字段（gw=网络+1, start=网络+2, end=网段末）
func (p *ipPoolConfig) initV6(cidr string) error {
	_, v6Net, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	ones, bits := v6Net.Mask.Size()
	if bits != 128 || ones >= 128 {
		return fmt.Errorf("IPv6 CIDR 前缀须 < 128 才有分配空间: %s", cidr)
	}
	network := v6Net.IP.Mask(v6Net.Mask)
	networkBig := ipToBig(network)
	hosts := new(big.Int).Lsh(big.NewInt(1), uint(128-ones))

	p.Ipv6IPNet = v6Net
	p.Ipv6Gateway = bigToIP(new(big.Int).Add(networkBig, big.NewInt(1)))
	p.ipv6Start = new(big.Int).Add(networkBig, big.NewInt(2))
	p.ipv6End = new(big.Int).Add(networkBig, new(big.Int).Sub(hosts, big.NewInt(1)))
	p.ipv6Cursor = new(big.Int).Set(p.ipv6Start)
	return nil
}

func (p *IPPool) Init() error {
	// 地址处理
	_, ipNet, err := net.ParseCIDR(base.GetCfg().Ipv4CIDR)
	if err != nil {
		return fmt.Errorf("IPv4 CIDR 配置错误(%s): %w", base.GetCfg().Ipv4CIDR, err)
	}
	p.Global.Ipv4IPNet = ipNet
	p.Global.Ipv4Mask = net.IP(ipNet.Mask)

	ipv4Gateway := net.ParseIP(base.GetCfg().Ipv4Gateway)
	ipStart := net.ParseIP(base.GetCfg().Ipv4Start)
	ipEnd := net.ParseIP(base.GetCfg().Ipv4End)
	if !ipNet.Contains(ipv4Gateway) || !ipNet.Contains(ipStart) || !ipNet.Contains(ipEnd) {
		return fmt.Errorf("IPv4 网关/起始/结束地址不在 CIDR(%s) 网段内，网关=%s 起始=%s 结束=%s",
			base.GetCfg().Ipv4CIDR, base.GetCfg().Ipv4Gateway, base.GetCfg().Ipv4Start, base.GetCfg().Ipv4End)
	}
	// ip地址池
	p.Global.Ipv4Gateway = ipv4Gateway
	p.Global.IpLongMin = utils.Ip2long(ipStart)
	p.Global.IpLongMax = utils.Ip2long(ipEnd)

	p.Global.loopCurIp = p.Global.IpLongMin
	p.Global.GroupName = "" // 全局池无组归属

	// IPv6 全局地址池（单 Ipv6CIDR 自动分配；gw=网络+1, start=网络+2, end=网段末）
	if cidr := base.GetCfg().Ipv6CIDR; cidr != "" {
		// IPv6 要求链路 MTU ≥ 1280，低于则自动上调并告警
		if m := base.GetCfg().Mtu; m > 0 && m < 1280 {
			base.Warn("IPv6 要求链路 MTU ≥ 1280，当前 Mtu=", m, " 已自动上调到 1280")
			base.GetCfg().Mtu = 1280
		}
		if err6 := p.Global.initV6(cidr); err6 != nil {
			return fmt.Errorf("IPv6 CIDR 配置错误(%s): %w", cidr, err6)
		}
		base.Debug("IPv6 地址池初始化: 网络=", p.Global.Ipv6IPNet.IP.String(),
			" 网关=", p.Global.Ipv6Gateway.String(),
			" 范围=", p.Global.ipv6Start.String(), "-", p.Global.ipv6End.String())
	}

	return nil
}

// IPv6 地址 <-> big.Int 转换（128 位，网络字节序）
func ipToBig(ip net.IP) *big.Int {
	return new(big.Int).SetBytes(ip.To16())
}

func bigToIP(b *big.Int) net.IP {
	buf := b.Bytes()
	ip := make(net.IP, 16)
	copy(ip[16-len(buf):], buf)
	return ip
}

// 获取 IPv6 动态地址。pool 为连接所属组池（未配组 v6 段则回退全局池）
// 向后兼容：全局未配置 Ipv6CIDR 且组无 v6 段时返回 nil
func (p *IPPool) acquireV6(username, macAddr string, uniqueMac bool, groupPool *ipPoolConfig) net.IP {
	// allocPool 为实际分配地址的段：组池未配置 v6 时回退到全局 Ipv6CIDR 池
	allocPool := groupPool
	if allocPool == nil || allocPool.Ipv6IPNet == nil {
		allocPool = p.Global // 组池无 v6 字段时回退全局地址段
	}
	if allocPool.Ipv6IPNet == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// groupPool 只用于携带组身份（GroupName），决定绑定记到哪个组，与地址来源无关
	return p.loopV6(username, macAddr, uniqueMac, groupPool, allocPool)
}

func (p *IPPool) loopV6(username, macAddr string, uniqueMac bool, groupPool *ipPoolConfig, allocPool *ipPoolConfig) net.IP {
	start := allocPool.ipv6Start
	end := allocPool.ipv6End
	if start == nil || end == nil {
		return nil
	}
	tNow := time.Now()
	leaseTime := time.Now().Add(-1 * time.Duration(base.GetCfg().IpLease) * time.Second)

	cursor := allocPool.ipv6Cursor
	if cursor.Cmp(start) < 0 || cursor.Cmp(end) > 0 {
		cursor = new(big.Int).Set(start)
	}

	// 两段扫描：cursor->end 与 start->cursor-1
	segs := [][2]*big.Int{
		{cursor, end},
		{start, new(big.Int).Sub(cursor, big.NewInt(1))},
	}
	for _, seg := range segs {
		lo, hi := seg[0], seg[1]
		if lo == nil || hi == nil || lo.Cmp(hi) > 0 {
			continue
		}
		cur := new(big.Int).Set(lo)
		for cur.Cmp(hi) <= 0 {
			ip := bigToIP(cur)
			ipStr := ip.String()
			// 内存去重（活跃连接，全局）
			if _, ok := p.active[ipStr]; ok {
				cur.Add(cur, big.NewInt(1))
				continue
			}
			// 该 v6 是否已被其它客户端占用（跨重启用 ip_addr6 列查）
			m6 := &dbdata.IpMap{}
			if err := dbdata.IpMaps.GetByIP6(ipStr, m6); err == nil {
				// 已占用：非本客户端或已过期则跳过；本客户端未过期则直接复用
				if m6.MacAddr != macAddr || m6.LastLogin.Before(leaseTime) {
					cur.Add(cur, big.NewInt(1))
					continue
				}
				p.active[ipStr] = true
				allocPool.ipv6Cursor = new(big.Int).Add(cur, big.NewInt(1))
				return ip
			} else if !utils.CheckErrNotFound(err) {
				base.Error("查询 v6 ip_map 失败:", err)
				return nil
			}
			// 空闲：写入本客户端的 ip_map 行（与 v4 同一行，按 mac_addr+ip_group 定位）
			row := &dbdata.IpMap{}
			e2 := dbdata.GetDB().GetWhere("mac_addr=? AND ip_group=?", row, macAddr, groupPool.GroupName)
			if e2 != nil && !utils.CheckErrNotFound(e2) {
				base.Error("查询 v6 所属 ip_map 行失败:", e2)
				return nil
			}
			if utils.CheckErrNotFound(e2) {
				// 理论上 v4 先分配，此行已存在；此处防御性新建（IpAddr 留空，仅存 v6）
				row = &dbdata.IpMap{IpAddr: "", MacAddr: macAddr, UniqueMac: uniqueMac, Username: username, LastLogin: tNow, IpAddr6: ipStr, Group: groupPool.GroupName}
				if err := dbdata.GetDB().Insert(row); err != nil {
					base.Error("IP池 v6 Add 失败:", err)
				}
			} else {
				row.IpAddr6 = ipStr
				row.Username = username
				row.LastLogin = tNow
				if err := dbdata.GetDB().Save(row); err != nil {
					base.Error("IP池 v6 Set 失败:", err)
				}
			}
			p.active[ipStr] = true
			allocPool.ipv6Cursor = new(big.Int).Add(cur, big.NewInt(1))
			return ip
		}
	}
	base.Warn("no ipv6 available, please see ip_map table row", username, macAddr)
	return nil
}

func (p *IPPool) inPool(ip net.IP, ipRange *ipPoolConfig) bool {
	if ipRange == nil {
		ipRange = p.GetGroupPool(nil) // fallback to global
	}
	if ip.To4() == nil {
		return false
	}
	ipLong := utils.Ip2long(ip)
	if ipLong >= ipRange.IpLongMin && ipLong <= ipRange.IpLongMax {
		return true
	}
	return false
}

// 获取动态ip（使用全局 IP 池，向后兼容）
func (p *IPPool) Acquire(username, macAddr string, uniqueMac bool) (newIp net.IP) {
	return p.AcquireWithRange(username, macAddr, uniqueMac, nil)
}

// 在指定 IP 范围内获取动态 ip
func (p *IPPool) AcquireWithRange(username, macAddr string, uniqueMac bool, ipRange *ipPoolConfig) (newIp net.IP) {
	base.Trace("AcquireIp start:", username, macAddr, uniqueMac)
	// 由 NewConn 的 LimitClient 回退路径处理，避免连接数泄漏
	defer func() {
		if r := recover(); r != nil {
			base.Error("AcquireIpWithRange panic:", username, macAddr, r)
			newIp = nil
		}
	}()
	p.mu.Lock()
	defer func() {
		p.mu.Unlock()
		base.Trace("AcquireIp end:", username, macAddr, uniqueMac, newIp)
		base.Debug("AcquireIp ip:", username, macAddr, uniqueMac, newIp)
	}()

	if ipRange == nil {
		ipRange = p.GetGroupPool(nil)
	}

	var (
		err  error
		tNow = time.Now()
	)

	// 获取到客户端 macAddr 的情况
	if uniqueMac {
		// 判断是否已经分配过：按 MAC + 组 精确匹配优先
		mi := &dbdata.IpMap{}
		err = dbdata.GetDB().GetWhere("mac_addr=? AND ip_group=?", mi, macAddr, ipRange.GroupName)
		if utils.CheckErrNotFound(err) && ipRange.GroupName != "" {
			err = dbdata.GetDB().GetWhere("mac_addr=? AND ip_group=''", mi, macAddr)
		}
		if err != nil {
			// 没有查询到数据
			if utils.CheckErrNotFound(err) {
				return p.loopIp(username, macAddr, uniqueMac, ipRange)
			}
			// 查询报错
			base.Error(err)
			return nil
		}

		// 存在ip记录
		base.Trace("uniqueMac:", username, mi)
		ipStr := mi.IpAddr
		ip := net.ParseIP(ipStr)
		// 跳过活跃连接
		_, ok := p.active[ipStr]
		// 检测原有ip是否在新的ip池内
		if !ok && p.inPool(ip, ipRange) {
			mi.Username = username
			mi.LastLogin = tNow
			mi.UniqueMac = uniqueMac
			mi.Group = ipRange.GroupName
			// 回写db数据
			if err = dbdata.GetDB().Save(mi); err != nil {
				base.Error("IP池 Set 失败:", err)
			}
			p.active[ipStr] = true
			return ip
		}

		// ip保留
		if mi.Keep {
			base.Error(username, macAddr, ipStr, "保留ip不匹配CIDR")
			return nil
		}

		// 删除当前macAddr
		if err = dbdata.GetDB().Delete(&dbdata.IpMap{Id: mi.Id}); err != nil {
			base.Error("IP池 Del(过期绑定) 失败:", err)
			return nil
		}
		return p.loopIp(username, macAddr, uniqueMac, ipRange)
	}

	// 没有获取到mac的情况（按 用户名 + 组 精确匹配优先）
	ipMaps := []dbdata.IpMap{}
	err = dbdata.GetDB().FindWhere(&ipMaps, ipPoolUserQueryLimit, 1, "username=? AND ip_group=?", username, ipRange.GroupName)
	if err == nil && len(ipMaps) == 0 && ipRange.GroupName != "" {
		err = dbdata.GetDB().FindWhere(&ipMaps, ipPoolUserQueryLimit, 1, "username=? AND ip_group=''", username)
	}
	if err != nil {
		// 没有查询到数据
		if utils.CheckErrNotFound(err) {
			return p.loopIp(username, macAddr, uniqueMac, ipRange)
		}
		// 查询报错
		base.Error(err)
		return nil
	}

	// 遍历mac记录
	for _, mi := range ipMaps {
		ipStr := mi.IpAddr
		ip := net.ParseIP(ipStr)

		// 跳过活跃连接；若是同一设备（同 MAC）重连，则直接复用其地址，
		if ok := p.active[ipStr]; ok && mi.MacAddr != macAddr {
			continue
		}
		// 跳过保留ip
		if mi.Keep {
			continue
		}
		if mi.UniqueMac {
			continue
		}

		// 没有mac的 不需要验证租期
		if p.inPool(ip, ipRange) {
			mi.Username = username
			mi.LastLogin = tNow
			mi.MacAddr = macAddr
			mi.UniqueMac = uniqueMac
			mi.Group = ipRange.GroupName
			if err = dbdata.GetDB().Save(mi); err != nil {
				base.Error("IP池 Set 失败:", err)
			}
			p.active[ipStr] = true
			return ip
		}
	}

	return p.loopIp(username, macAddr, uniqueMac, ipRange)
}

func (p *IPPool) loopIp(username, macAddr string, uniqueMac bool, ipRange *ipPoolConfig) net.IP {
	var (
		i  uint32
		ip net.IP
	)

	if ipRange == nil {
		ipRange = p.GetGroupPool(nil)
	}

	// 重新赋值
	ipRange.loopFarIp = &dbdata.IpMap{LastLogin: time.Now()}

	i, ip = p.loopLong(ipRange.loopCurIp, ipRange.IpLongMax, username, macAddr, uniqueMac, ipRange)
	if ip != nil {
		ipRange.loopCurIp = i
		return ip
	}

	i, ip = p.loopLong(ipRange.IpLongMin, ipRange.loopCurIp, username, macAddr, uniqueMac, ipRange)
	if ip != nil {
		ipRange.loopCurIp = i
		return ip
	}

	// ip分配完,从头开始
	ipRange.loopCurIp = ipRange.IpLongMin

	if ipRange.loopFarIp.Id > 0 {
		// 使用最早登陆的 ip（回收并重新分配给当前客户端）
		ipStr := ipRange.loopFarIp.IpAddr
		ip = net.ParseIP(ipStr)
		ipRange.loopFarIp.MacAddr = macAddr
		ipRange.loopFarIp.UniqueMac = uniqueMac
		ipRange.loopFarIp.Username = username
		ipRange.loopFarIp.LastLogin = time.Now()
		ipRange.loopFarIp.Group = ipRange.GroupName
		// 回写db数据
		if setErr := dbdata.GetDB().Save(ipRange.loopFarIp); setErr != nil {
			base.Error("IP池 Set(最早) 失败:", setErr)
		}
		p.active[ipStr] = true

		return ip
	}

	// 全都在线，没有数据可用
	base.Warn("no ip available, please see ip_map table row", username, macAddr)
	return nil
}

func (p *IPPool) loopLong(start, end uint32, username, macAddr string, uniqueMac bool, ipRange *ipPoolConfig) (uint32, net.IP) {
	var (
		err       error
		tNow      = time.Now()
		leaseTime = time.Now().Add(-1 * time.Duration(base.GetCfg().IpLease) * time.Second)
	)

	if ipRange == nil {
		ipRange = p.GetGroupPool(nil)
	}

	// 全局遍历超过租期和未保留的ip
	for i := start; i <= end; i++ {
		ip := utils.Long2ip(i)
		ipStr := ip.String()

		// 跳过网关地址，避免下发给客户端造成地址冲突
		if ipRange.Ipv4Gateway != nil && ip.Equal(ipRange.Ipv4Gateway) {
			continue
		}

		// 跳过活跃连接
		if _, ok := p.active[ipStr]; ok {
			continue
		}

		mi := &dbdata.IpMap{}
		err = dbdata.IpMaps.GetByIP(ipStr, mi)
		if err != nil {
			// 没有查询到数据
			if utils.CheckErrNotFound(err) {
				// 该ip没有被使用
				mi = &dbdata.IpMap{IpAddr: ipStr, MacAddr: macAddr, UniqueMac: uniqueMac, Username: username, LastLogin: tNow, Group: ipRange.GroupName}
				if err = dbdata.GetDB().Insert(mi); err != nil {
					base.Error("IP池 Add 失败:", err)
				}
				p.active[ipStr] = true
				return i, ip
			}
			// 查询报错
			base.Error(err)
			return 0, nil
		}

		// 查询到已经使用的ip
		// 跳过保留ip
		if mi.Keep {
			continue
		}
		// 判断租期
		if mi.LastLogin.Before(leaseTime) {
			// 存在记录，说明已经超过租期，可以直接使用
			mi.Username = username
			mi.LastLogin = tNow
			mi.MacAddr = macAddr
			mi.UniqueMac = uniqueMac
			mi.Group = ipRange.GroupName
			// 回写db数据
			if err = dbdata.GetDB().Save(mi); err != nil {
				base.Error("IP池 Set(租期) 失败:", err)
			}
			p.active[ipStr] = true
			return i, ip
		}
		// 其他情况判断最早登陆
		if mi.LastLogin.Before(ipRange.loopFarIp.LastLogin) {
			ipRange.loopFarIp = mi
		}
	}

	return 0, nil
}

// 回收ip（v4 + 可选 v6）
func (p *IPPool) Release(ip net.IP, ip6 net.IP, macAddr string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if ip != nil {
		delete(p.active, ip.String())
		mi := &dbdata.IpMap{}
		err := dbdata.IpMaps.GetByIP(ip.String(), mi)
		if err == nil {
			mi.LastLogin = time.Now()
			if err = dbdata.GetDB().Save(mi); err != nil {
				base.Error("IP池 ReleaseIp Set 失败:", err)
			}
		}
	}
	if ip6 != nil {
		delete(p.active, ip6.String())
		mi6 := &dbdata.IpMap{}
		err6 := dbdata.IpMaps.GetByIP6(ip6.String(), mi6)
		if err6 == nil {
			mi6.IpAddr6 = ""
			mi6.LastLogin = time.Now()
			if err6 = dbdata.GetDB().Save(mi6); err6 != nil {
				base.Error("IP池 ReleaseIp v6 Set 失败:", err6)
			}
		}
	}
}
