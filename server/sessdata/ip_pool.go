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

var (
	IpPool   = &ipPoolConfig{}
	ipActive = map[string]bool{}
	// ipKeep and ipLease  ipAddr => macAddr
	// ipKeep    = map[string]string{}
	ipPoolMux sync.Mutex

	// 组池缓存
	groupPoolCache = map[string]*ipPoolConfig{}
	groupPoolMux   sync.Mutex
)

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

func GetGroupIpPool(group *dbdata.Group) *ipPoolConfig {
	// 优先使用组级别配置
	if group != nil && group.ClientCidr != "" && group.ClientStart != "" && group.ClientEnd != "" && group.ClientGateway != "" {
		// 相同网段的不同组若共享池对象，GroupName 会错乱，
		// 导致 IpMap 按 (mac_addr, ip_group) 定位到错误记录
		cacheKey := group.Name + "|" + group.ClientCidr + "|" + group.ClientStart + "|" + group.ClientEnd + "|" + group.ClientGateway + "|" + group.ClientCidr6

		groupPoolMux.Lock()
		defer groupPoolMux.Unlock()
		if p, ok := groupPoolCache[cacheKey]; ok {
			return p // 复用，游标得以跨连接累积
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
				p := &ipPoolConfig{
					Ipv4Gateway: gateway,
					Ipv4Mask:    net.IP(ipNet.Mask),
					Ipv4IPNet:   ipNet,
					IpLongMin:   utils.Ip2long(start),
					IpLongMax:   utils.Ip2long(end),
					GroupName:   group.Name,
				}
				p.loopCurIp = p.IpLongMin
				// 组级 v6 段：受全局 Ipv6CIDR 总开关约束，未开总开关则忽略组 v6
				if group.ClientCidr6 != "" && base.GetCfg().Ipv6CIDR != "" {
					if err6 := p.initV6(group.ClientCidr6); err6 != nil {
						base.Warn("组", group.Name, "IPv6网段配置无效，该组使用全局 v6 池:", err6)
					}
				}
				groupPoolCache[cacheKey] = p
				return p
			}
			base.Warn("组", group.Name, "IP范围不在网段内，回退到全局")
		}
	}

	// 回退到全局配置
	return IpPool
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

// 返回本池的 v6 网关；组池未配置 v6 段时回退全局池（客户端 v6 从全局池分配）
func (p *ipPoolConfig) V6Gateway() net.IP {
	if p != nil && p.Ipv6Gateway != nil {
		return p.Ipv6Gateway
	}
	return IpPool.Ipv6Gateway
}

func initIpPool() error {
	// 地址处理
	_, ipNet, err := net.ParseCIDR(base.GetCfg().Ipv4CIDR)
	if err != nil {
		return fmt.Errorf("IPv4 CIDR 配置错误(%s): %v", base.GetCfg().Ipv4CIDR, err)
	}
	IpPool.Ipv4IPNet = ipNet
	IpPool.Ipv4Mask = net.IP(ipNet.Mask)

	ipv4Gateway := net.ParseIP(base.GetCfg().Ipv4Gateway)
	ipStart := net.ParseIP(base.GetCfg().Ipv4Start)
	ipEnd := net.ParseIP(base.GetCfg().Ipv4End)
	if !ipNet.Contains(ipv4Gateway) || !ipNet.Contains(ipStart) || !ipNet.Contains(ipEnd) {
		return fmt.Errorf("IPv4 网关/起始/结束地址不在 CIDR(%s) 网段内，网关=%s 起始=%s 结束=%s",
			base.GetCfg().Ipv4CIDR, base.GetCfg().Ipv4Gateway, base.GetCfg().Ipv4Start, base.GetCfg().Ipv4End)
	}
	// ip地址池
	IpPool.Ipv4Gateway = ipv4Gateway
	IpPool.IpLongMin = utils.Ip2long(ipStart)
	IpPool.IpLongMax = utils.Ip2long(ipEnd)

	IpPool.loopCurIp = IpPool.IpLongMin
	IpPool.GroupName = "" // 全局池无组归属

	// IPv6 全局地址池（单 Ipv6CIDR 自动分配；gw=网络+1, start=网络+2, end=网段末）
	if cidr := base.GetCfg().Ipv6CIDR; cidr != "" {
		// IPv6 要求链路 MTU ≥ 1280，低于则自动上调并告警
		if m := base.GetCfg().Mtu; m > 0 && m < 1280 {
			base.Warn("IPv6 要求链路 MTU ≥ 1280，当前 Mtu=", m, " 已自动上调到 1280")
			base.GetCfg().Mtu = 1280
		}
		if err6 := IpPool.initV6(cidr); err6 != nil {
			return fmt.Errorf("IPv6 CIDR 配置错误(%s): %v", cidr, err6)
		}
		base.Debug("IPv6 地址池初始化: 网络=", IpPool.Ipv6IPNet.IP.String(),
			" 网关=", IpPool.Ipv6Gateway.String(),
			" 范围=", IpPool.ipv6Start.String(), "-", IpPool.ipv6End.String())
	}

	// 网络地址零值
	// zero := binary.BigEndian.Uint32(ip.Mask(mask))
	// 广播地址
	// one, _ := ipNet.Mask.Size()
	// max := min | uint32(math.Pow(2, float64(32-one))-1)

	// 获取IpLease数据
	// go cronIpLease()

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
func acquireIpV6(username, macAddr string, uniqueMac bool, groupPool *ipPoolConfig) net.IP {
	// allocPool 为实际分配地址的段：组池未配置 v6 时回退到全局 Ipv6CIDR 池
	allocPool := groupPool
	if allocPool == nil || allocPool.Ipv6IPNet == nil {
		allocPool = IpPool // 组池无 v6 字段时回退全局地址段
	}
	if allocPool.Ipv6IPNet == nil {
		return nil
	}
	ipPoolMux.Lock()
	defer ipPoolMux.Unlock()
	// groupPool 只用于携带组身份（GroupName），决定绑定记到哪个组，与地址来源无关
	return loopIpV6(username, macAddr, uniqueMac, groupPool, allocPool)
}

func loopIpV6(username, macAddr string, uniqueMac bool, groupPool *ipPoolConfig, allocPool *ipPoolConfig) net.IP {
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
			if _, ok := ipActive[ipStr]; ok {
				cur.Add(cur, big.NewInt(1))
				continue
			}
			// 该 v6 是否已被其它客户端占用（跨重启用 ip_addr6 列查）
			m6 := &dbdata.IpMap{}
			if err := dbdata.One("ip_addr6", ipStr, m6); err == nil {
				// 已占用：非本客户端或已过期则跳过；本客户端未过期则直接复用
				if m6.MacAddr != macAddr || m6.LastLogin.Before(leaseTime) {
					cur.Add(cur, big.NewInt(1))
					continue
				}
				ipActive[ipStr] = true
				allocPool.ipv6Cursor = new(big.Int).Add(cur, big.NewInt(1))
				return ip
			} else if !dbdata.CheckErrNotFound(err) {
				base.Error("查询 v6 ip_map 失败:", err)
				return nil
			}
			// 空闲：写入本客户端的 ip_map 行（与 v4 同一行，按 mac_addr+ip_group 定位）
			row := &dbdata.IpMap{}
			e2 := dbdata.OneWhere("mac_addr=? AND ip_group=?", row, macAddr, groupPool.GroupName)
			if e2 != nil && !dbdata.CheckErrNotFound(e2) {
				base.Error("查询 v6 所属 ip_map 行失败:", e2)
				return nil
			}
			if dbdata.CheckErrNotFound(e2) {
				// 理论上 v4 先分配，此行已存在；此处防御性新建（IpAddr 留空，仅存 v6）
				row = &dbdata.IpMap{IpAddr: "", MacAddr: macAddr, UniqueMac: uniqueMac, Username: username, LastLogin: tNow, IpAddr6: ipStr, Group: groupPool.GroupName}
				if err := dbdata.Add(row); err != nil {
					base.Error("IP池 v6 Add 失败:", err)
				}
			} else {
				row.IpAddr6 = ipStr
				row.Username = username
				row.LastLogin = tNow
				if err := dbdata.Set(row); err != nil {
					base.Error("IP池 v6 Set 失败:", err)
				}
			}
			ipActive[ipStr] = true
			allocPool.ipv6Cursor = new(big.Int).Add(cur, big.NewInt(1))
			return ip
		}
	}
	base.Warn("no ipv6 available, please see ip_map table row", username, macAddr)
	return nil
}

// func cronIpLease() {
// 	getIpLease()
// 	tick := time.NewTicker(time.Minute * 30)
// 	for range tick.C {
// 		getIpLease()
// 	}
// }
//
// func getIpLease() {
// 	xdb := dbdata.GetXdb()
// 	keepIpMaps := []dbdata.IpMap{}
// 	// sNow := time.Now().Add(-1 * time.Duration(base.GetCfg().IpLease) * time.Second)
// 	err := xdb.Cols("ip_addr", "mac_addr").Where("keep=?", true).Find(&keepIpMaps)
// 	if err != nil {
// 		base.Error(err)
// 	}
// 	log.Println(keepIpMaps)
// 	ipPoolMux.Lock()
// 	ipKeep = map[string]string{}
// 	for _, v := range keepIpMaps {
// 		ipKeep[v.IpAddr] = v.MacAddr
// 	}
// 	ipPoolMux.Unlock()
// }

func ipInPool(ip net.IP, ipRange *ipPoolConfig) bool {
	if ipRange == nil {
		ipRange = GetGroupIpPool(nil) // fallback to global
	}
	ipLong := utils.Ip2long(ip)
	if ipLong >= ipRange.IpLongMin && ipLong <= ipRange.IpLongMax {
		return true
	}
	return false
}

// 获取动态ip（使用全局 IP 池，向后兼容）
func AcquireIp(username, macAddr string, uniqueMac bool) (newIp net.IP) {
	return AcquireIpWithRange(username, macAddr, uniqueMac, nil)
}

// 在指定 IP 范围内获取动态 ip
func AcquireIpWithRange(username, macAddr string, uniqueMac bool, ipRange *ipPoolConfig) (newIp net.IP) {
	base.Trace("AcquireIp start:", username, macAddr, uniqueMac)
	ipPoolMux.Lock()
	defer func() {
		ipPoolMux.Unlock()
		base.Trace("AcquireIp end:", username, macAddr, uniqueMac, newIp)
		base.Debug("AcquireIp ip:", username, macAddr, uniqueMac, newIp)
	}()

	if ipRange == nil {
		ipRange = GetGroupIpPool(nil)
	}

	var (
		err  error
		tNow = time.Now()
	)

	// 获取到客户端 macAddr 的情况
	if uniqueMac {
		// 判断是否已经分配过：按 MAC + 组 精确匹配优先
		mi := &dbdata.IpMap{}
		err = dbdata.OneWhere("mac_addr=? AND ip_group=?", mi, macAddr, ipRange.GroupName)
		if dbdata.CheckErrNotFound(err) && ipRange.GroupName != "" {
			err = dbdata.OneWhere("mac_addr=? AND ip_group=''", mi, macAddr)
		}
		if err != nil {
			// 没有查询到数据
			if dbdata.CheckErrNotFound(err) {
				return loopIp(username, macAddr, uniqueMac, ipRange)
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
		_, ok := ipActive[ipStr]
		// 检测原有ip是否在新的ip池内
		if !ok && ipInPool(ip, ipRange) {
			mi.Username = username
			mi.LastLogin = tNow
			mi.UniqueMac = uniqueMac
			mi.Group = ipRange.GroupName
			// 回写db数据
			if err = dbdata.Set(mi); err != nil {
				base.Error("IP池 Set 失败:", err)
			}
			ipActive[ipStr] = true
			return ip
		}

		// ip保留
		if mi.Keep {
			base.Error(username, macAddr, ipStr, "保留ip不匹配CIDR")
			return nil
		}

		// 删除当前macAddr
		if err = dbdata.Del(&dbdata.IpMap{Id: mi.Id}); err != nil {
			base.Error("IP池 Del(过期绑定) 失败:", err)
			return nil
		}
		return loopIp(username, macAddr, uniqueMac, ipRange)
	}

	// 没有获取到mac的情况（按 用户名 + 组 精确匹配优先）
	ipMaps := []dbdata.IpMap{}
	err = dbdata.FindWhere(&ipMaps, 30, 1, "username=? AND ip_group=?", username, ipRange.GroupName)
	if err == nil && len(ipMaps) == 0 && ipRange.GroupName != "" {
		err = dbdata.FindWhere(&ipMaps, 30, 1, "username=? AND ip_group=''", username)
	}
	if err != nil {
		// 没有查询到数据
		if dbdata.CheckErrNotFound(err) {
			return loopIp(username, macAddr, uniqueMac, ipRange)
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
		if ok := ipActive[ipStr]; ok && mi.MacAddr != macAddr {
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
		if ipInPool(ip, ipRange) {
			mi.Username = username
			mi.LastLogin = tNow
			mi.MacAddr = macAddr
			mi.UniqueMac = uniqueMac
			mi.Group = ipRange.GroupName
			if err = dbdata.Set(mi); err != nil {
				base.Error("IP池 Set 失败:", err)
			}
			ipActive[ipStr] = true
			return ip
		}
	}

	return loopIp(username, macAddr, uniqueMac, ipRange)
}

func loopIp(username, macAddr string, uniqueMac bool, ipRange *ipPoolConfig) net.IP {
	var (
		i  uint32
		ip net.IP
	)

	if ipRange == nil {
		ipRange = GetGroupIpPool(nil)
	}

	// 重新赋值
	ipRange.loopFarIp = &dbdata.IpMap{LastLogin: time.Now()}

	i, ip = loopLong(ipRange.loopCurIp, ipRange.IpLongMax, username, macAddr, uniqueMac, ipRange)
	if ip != nil {
		ipRange.loopCurIp = i
		return ip
	}

	i, ip = loopLong(ipRange.IpLongMin, ipRange.loopCurIp, username, macAddr, uniqueMac, ipRange)
	if ip != nil {
		ipRange.loopCurIp = i
		return ip
	}

	// ip分配完,从头开始
	ipRange.loopCurIp = ipRange.IpLongMin

	if ipRange.loopFarIp.Id > 0 {
		// 使用最早登陆的 ip（回收并重新分配给当前客户端）。
		ipStr := ipRange.loopFarIp.IpAddr
		ip = net.ParseIP(ipStr)
		ipRange.loopFarIp.MacAddr = macAddr
		ipRange.loopFarIp.UniqueMac = uniqueMac
		ipRange.loopFarIp.Username = username
		ipRange.loopFarIp.LastLogin = time.Now()
		ipRange.loopFarIp.Group = ipRange.GroupName
		// 回写db数据
		if setErr := dbdata.Set(ipRange.loopFarIp); setErr != nil {
			base.Error("IP池 Set(最早) 失败:", setErr)
		}
		ipActive[ipStr] = true

		return ip
	}

	// 全都在线，没有数据可用
	base.Warn("no ip available, please see ip_map table row", username, macAddr)
	return nil
}

func loopLong(start, end uint32, username, macAddr string, uniqueMac bool, ipRange *ipPoolConfig) (uint32, net.IP) {
	var (
		err       error
		tNow      = time.Now()
		leaseTime = time.Now().Add(-1 * time.Duration(base.GetCfg().IpLease) * time.Second)
	)

	if ipRange == nil {
		ipRange = GetGroupIpPool(nil)
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
		if _, ok := ipActive[ipStr]; ok {
			continue
		}

		mi := &dbdata.IpMap{}
		err = dbdata.One("ip_addr", ipStr, mi)
		if err != nil {
			// 没有查询到数据
			if dbdata.CheckErrNotFound(err) {
				// 该ip没有被使用
				mi = &dbdata.IpMap{IpAddr: ipStr, MacAddr: macAddr, UniqueMac: uniqueMac, Username: username, LastLogin: tNow, Group: ipRange.GroupName}
				if err = dbdata.Add(mi); err != nil {
					base.Error("IP池 Add 失败:", err)
				}
				ipActive[ipStr] = true
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
			if err = dbdata.Set(mi); err != nil {
				base.Error("IP池 Set(租期) 失败:", err)
			}
			ipActive[ipStr] = true
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
func ReleaseIp(ip net.IP, ip6 net.IP, macAddr string) {
	ipPoolMux.Lock()
	defer ipPoolMux.Unlock()

	if ip != nil {
		delete(ipActive, ip.String())
		mi := &dbdata.IpMap{}
		err := dbdata.One("ip_addr", ip.String(), mi)
		if err == nil {
			mi.LastLogin = time.Now()
			if err = dbdata.Set(mi); err != nil {
				base.Error("IP池 ReleaseIp Set 失败:", err)
			}
		}
	}
	if ip6 != nil {
		delete(ipActive, ip6.String())
		mi6 := &dbdata.IpMap{}
		err6 := dbdata.One("ip_addr6", ip6.String(), mi6)
		if err6 == nil {
			mi6.IpAddr6 = ""
			mi6.LastLogin = time.Now()
			if err6 = dbdata.Set(mi6); err6 != nil {
				base.Error("IP池 ReleaseIp v6 Set 失败:", err6)
			}
		}
	}
}
