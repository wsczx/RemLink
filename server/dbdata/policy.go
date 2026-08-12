package dbdata

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/songgao/water/waterutil"
	"github.com/wsczx/remlink/base"
)

// 根据 ID 加载策略并预处理所有字段
func LoadPolicyById(id int) (*Policy, error) {
	p := &Policy{}
	err := One("Id", id, p)
	if err != nil {
		return nil, fmt.Errorf("策略加载失败: %w", err)
	}
	p.AddFakeDNSRules()
	return p, nil
}

// 返回所有启用策略的 ID+名称列表（供下拉选择）
func GetPolicyNames() []GroupNameId {
	var datas []Policy
	err := FindWhere(&datas, 0, 0, "status=1")
	if err != nil {
		base.Error(err)
		return nil
	}
	var names []GroupNameId
	for _, v := range datas {
		names = append(names, GroupNameId{Id: v.Id, Name: v.Name})
	}
	return names
}

// 返回所有策略的 ID+名称列表（含停用的）
func GetAllPolicyNames() []GroupNameId {
	var datas []Policy
	err := Find(&datas, 0, 0)
	if err != nil {
		base.Error(err)
		return nil
	}
	var names []GroupNameId
	for _, v := range datas {
		names = append(names, GroupNameId{Id: v.Id, Name: v.Name})
	}
	return names
}

// 返回引用了指定策略的用户组列表
func PolicyUsedByGroups(policyId int) []Group {
	var groups []Group
	err := FindWhere(&groups, 0, 0, "policy_id=?", policyId)
	if err != nil {
		base.Error(err)
		return nil
	}
	return groups
}

// 返回引用了指定策略的用户列表
func PolicyUsedByUsers(policyId int) []User {
	var users []User
	err := FindWhere(&users, 0, 0, "policy_id=?", policyId)
	if err != nil {
		base.Error(err)
		return nil
	}
	return users
}

// 新增或更新策略配置
func SetPolicy(p *Policy) error {
	var err error
	if p.Name == "" {
		return errors.New("策略名称不能为空")
	}

	if p.Bandwidth < 0 {
		return errors.New("下行带宽不能小于0")
	}
	if p.BandwidthUp < 0 {
		return errors.New("上行带宽不能小于0")
	}
	if p.TrafficQuota < 0 {
		return errors.New("流量配额不能小于0")
	}
	switch p.TrafficReset {
	case "", "daily", "weekly", "monthly":
	default:
		return errors.New("流量配额重置周期错误，可选: daily/weekly/monthly")
	}
	if p.TrafficQuota > 0 && p.TrafficReset == "" {
		p.TrafficReset = "monthly"
	}
	if p.TrafficQuota == 0 {
		p.TrafficReset = ""
	}

	// 包含路由验证
	routeInclude := []ValData{}
	for _, v := range p.RouteInclude {
		if v.Val != "" {
			if v.Val == ALL {
				routeInclude = append(routeInclude, v)
				continue
			}
			ipMask, ipNet, err := parseIpNet(v.Val)
			if err != nil {
				return errors.New("RouteInclude 错误" + err.Error())
			}
			if strings.Split(ipMask, "/")[0] != ipNet.IP.String() {
				return errors.New("RouteInclude 错误: " + fmt.Sprintf("网络地址错误，建议： %s 改为 %s", v.Val, ipNet))
			}
			v.IpMask = ipMask
			routeInclude = append(routeInclude, v)
		}
	}
	p.RouteInclude = routeInclude

	// 排除路由验证
	routeExclude := []ValData{}
	for _, v := range p.RouteExclude {
		if v.Val != "" {
			ipMask, ipNet, err := parseIpNet(v.Val)
			if err != nil {
				return errors.New("RouteExclude 错误" + err.Error())
			}
			if strings.Split(ipMask, "/")[0] != ipNet.IP.String() {
				return errors.New("RouteExclude 错误: " + fmt.Sprintf("网络地址错误，建议： %s 改为 %s", v.Val, ipNet))
			}
			v.IpMask = ipMask
			routeExclude = append(routeExclude, v)
		}
	}
	p.RouteExclude = routeExclude

	// LinkAcl 验证
	linkAcl := []GroupLinkAcl{}
	for _, v := range p.LinkAcl {
		if v.Val != "" {
			_, ipNet, err := parseIpNet(v.Val)
			if err != nil {
				return errors.New("GroupLinkAcl 错误" + err.Error())
			}
			v.IpNet = ipNet

			switch v.Protocol {
			case TCP:
				v.IpProto = waterutil.TCP
			case UDP:
				v.IpProto = waterutil.UDP
			case ICMP:
				v.IpProto = waterutil.ICMP
			default:
				v.Protocol = ALL
			}

			portsStr := v.Port
			v.Port = strings.TrimSpace(portsStr)

			if regexp.MustCompile(`^\d{1,5}(-\d{1,5})?(,\d{1,5}(-\d{1,5})?)*$`).MatchString(portsStr) {
				for pt := range strings.SplitSeq(portsStr, ",") {
					if pt == "" {
						continue
					}
					if regexp.MustCompile(`^\d{1,5}-\d{1,5}$`).MatchString(pt) {
						rp := strings.Split(pt, "-")
						portfrom, err := strconv.ParseUint(rp[0], 10, 16)
						if err != nil {
							return errors.New("端口:" + rp[0] + " 格式错误, " + err.Error())
						}
						portto, err := strconv.ParseUint(rp[1], 10, 16)
						if err != nil {
							return errors.New("端口:" + rp[1] + " 格式错误, " + err.Error())
						}
						if portfrom > portto {
							return errors.New("端口范围错误: 起始端口 " + rp[0] + " 不能大于结束端口 " + rp[1])
						}
					} else {
						if _, err := strconv.ParseUint(pt, 10, 16); err != nil {
							return errors.New("端口:" + pt + " 格式错误, " + err.Error())
						}
					}
				}
				linkAcl = append(linkAcl, v)
			} else {
				return errors.New("端口: " + portsStr + " 格式错误,请用逗号分隔的端口,比如: 22,80,443 连续端口用-,比如:1234-5678")
			}
		}
	}
	p.LinkAcl = linkAcl

	// DNS 判断
	clientDns := []ValData{}
	for _, v := range p.ClientDns {
		v.Val = strings.TrimSpace(v.Val)
		if v.Val != "" {
			ip := net.ParseIP(v.Val)
			if ip.String() != v.Val {
				return errors.New("DNS IP 错误")
			}
			clientDns = append(clientDns, v)
		}
	}
	isDefRoute := len(routeInclude) == 0 || (len(routeInclude) == 1 && routeInclude[0].Val == "all")
	if isDefRoute && len(clientDns) == 0 {
		return errors.New("默认路由，必须设置一个DNS")
	}
	p.ClientDns = clientDns

	// 域名拆分隧道，不能同时填写
	p.DsIncludeDomains = strings.TrimSpace(p.DsIncludeDomains)
	p.DsExcludeDomains = strings.TrimSpace(p.DsExcludeDomains)
	if p.DsIncludeDomains != "" && p.DsExcludeDomains != "" {
		return errors.New("包含/排除域名不能同时填写")
	}
	err = CheckDomainNames(p.DsIncludeDomains)
	if err != nil {
		return errors.New("包含域名有误：" + err.Error())
	}
	err = CheckDomainNames(p.DsExcludeDomains)
	if err != nil {
		return errors.New("排除域名有误：" + err.Error())
	}
	if isDefRoute && p.DsIncludeDomains != "" {
		return errors.New("默认路由, 不允许设置\"包含域名\", 请重新配置")
	}

	// FakeDNS 配置验证
	if p.EnableFakeDNS {
		if len(clientDns) == 0 && p.FakeDNSUpstream == "" {
			return errors.New("未配置客户端 DNS 时,必须配置 FakeDNS 上游 DNS,否则 FakeDNS 功能无法生效")
		}
		p.FakeDNSUpstream = strings.TrimSpace(p.FakeDNSUpstream)
		if p.FakeDNSUpstream != "" {
			// 允许 host:port 形式（IPv6 需 [..]:port），仅校验 host 部分为合法 IP，
			// 支持 IPv6 上游 DNS 服务器与自定义端口
			host := p.FakeDNSUpstream
			if h, _, err := net.SplitHostPort(p.FakeDNSUpstream); err == nil {
				host = h
			}
			if net.ParseIP(host) == nil {
				return errors.New("FakeDNS 上游 DNS 格式错误（应为 IP[:port]）")
			}
		}
		p.FakeDNSInclude = strings.TrimSpace(p.FakeDNSInclude)
		p.FakeDNSExclude = strings.TrimSpace(p.FakeDNSExclude)

		if p.FakeDNSInclude != "" && p.FakeDNSExclude != "" {
			return errors.New("FakeDNS 包含/排除域名不能同时填写")
		}
		if p.FakeDNSInclude == "" && p.FakeDNSExclude == "" {
			return errors.New("启用 FakeDNS 时必须至少填写\"包含域名\"或\"排除域名\"")
		}
		err = CheckFakeDNSDomains(p.FakeDNSInclude)
		if err != nil {
			return errors.New("FakeDNS 包含域名有误：" + err.Error())
		}
		err = CheckFakeDNSDomains(p.FakeDNSExclude)
		if err != nil {
			return errors.New("FakeDNS 排除域名有误：" + err.Error())
		}
	} else {
		// 禁用时清空
		p.FakeDNSInclude = ""
		p.FakeDNSExclude = ""
		p.FakeDNSUpstream = ""
		p.PreferIPv6 = false
	}

	p.UpdatedAt = time.Now()
	if p.Id > 0 {
		var old Policy
		if e := One("Id", p.Id, &old); e == nil {
			if old.TrafficReset != p.TrafficReset || old.TrafficQuota != p.TrafficQuota {
				clearTrafficResetAt(p.Id)
			}
		}
		err = Set(p)
	} else {
		err = Add(p)
	}
	return err
}

// 清除引用指定策略的用户的 TrafficResetAt
// 策略配额配置变更后调用，让 QuotaExceeded 重新初始化重置时间
func clearTrafficResetAt(policyId int) {
	usernames := map[string]struct{}{}

	var users []User
	if err := FindWhere(&users, 0, 0, "policy_id=?", policyId); err == nil {
		for _, u := range users {
			usernames[u.Username] = struct{}{}
		}
	}

	var groups []Group
	if err := FindWhere(&groups, 0, 0, "policy_id=?", policyId); err == nil {
		for _, g := range groups {
			var groupUsers []User
			// groups 字段是 JSON 数组，转义 LIKE 特殊字符后精确匹配组名
			escaped := escapeLike(g.Name)
			if err := FindWhere(&groupUsers, 0, 0, "groups like ?", `%"`+escaped+`"%`); err == nil {
				for _, u := range groupUsers {
					usernames[u.Username] = struct{}{}
				}
			}
		}
	}

	for username := range usernames {
		_, _ = xdb.Where("username=?", username).
			Cols("traffic_reset_at").
			Update(&User{})
	}
}

// 转义 SQL LIKE 模式中的特殊字符
func escapeLike(s string) string {
	r := make([]byte, 0, len(s)*2)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '%', '_':
			r = append(r, '\\')
		}
		r = append(r, s[i])
	}
	return string(r)
}

// 加载生效策略，优先级：用户策略 > 组策略
// 返回 nil 表示加载失败或策略已停用
func ApplyPolicy(username string, g *Group) *Policy {
	user := &User{}
	One("Username", username, user)

	if user.PolicyId > 0 {
		userPT, err := LoadPolicyById(user.PolicyId)
		if err != nil {
			base.Error("加载用户策略失败:", username, err)
			return nil
		}
		if userPT.Status != 1 {
			base.Warn(username, "个人策略已停用(", userPT.Name, ")，拒绝连接")
			return nil
		}
		base.Debug(username + " 使用个人策略: " + userPT.Name)
		return userPT
	}

	if g.PolicyId <= 0 {
		base.Warn("组未分配策略，拒绝连接:", g.Name)
		return nil
	}

	groupPT, err := LoadPolicyById(g.PolicyId)
	if err != nil {
		base.Error("加载组策略失败:", g.Name, err)
		return nil
	}
	if groupPT.Status != 1 {
		base.Warn("组策略已停用(", groupPT.Name, ")，拒绝连接")
		return nil
	}
	return groupPT
}

// 预处理 FakeDNS 域名规则为 HashMap
func (p *Policy) AddFakeDNSRules() {
	p.FakeDNSIncludeSet = addDomainSet(p.FakeDNSInclude)
	p.FakeDNSExcludeSet = addDomainSet(p.FakeDNSExclude)
}

// 返回 FakeDNS 上游 DNS 服务器地址
func (p *Policy) GetUpstreamDNS() string {
	if p.FakeDNSUpstream != "" {
		return p.FakeDNSUpstream
	}
	if len(p.ClientDns) > 0 {
		return p.ClientDns[0].Val
	}
	return "8.8.8.8"
}

func addDomainSet(raw string) map[string]struct{} {
	m := make(map[string]struct{})
	if raw == "" {
		return m
	}
	for d := range strings.SplitSeq(raw, ",") {
		d = strings.TrimSpace(strings.ToLower(d))
		if d != "" {
			m[d] = struct{}{}
		}
	}
	return m
}

// 计算下一次重置时间
// daily=次日0点，weekly=下周一0点，monthly=下月1日0点
func nextTrafficReset(period string, from time.Time) time.Time {
	switch period {
	case "daily":
		y, m, d := from.Date()
		return time.Date(y, m, d+1, 0, 0, 0, 0, from.Location())
	case "weekly":
		y, m, d := from.Date()
		daysToMonday := (int(time.Monday) - int(from.Weekday()) + 7) % 7
		if daysToMonday == 0 {
			daysToMonday = 7
		}
		return time.Date(y, m, d+daysToMonday, 0, 0, 0, 0, from.Location())
	case "monthly":
		y, m, _ := from.Date()
		return time.Date(y, m+1, 1, 0, 0, 0, 0, from.Location())
	}
	return time.Time{}
}

// 检查用户流量配额是否超出
// 自动处理周期重置和首次初始化，使用乐观锁避免并发 lost update
func QuotaExceeded(username string, p *Policy) (bool, int64) {
	if p == nil || p.TrafficQuota <= 0 {
		return false, 0
	}
	u := &User{}
	if err := One("Username", username, u); err != nil {
		return false, 0
	}
	now := time.Now()
	if u.TrafficResetAt == nil {
		// 首次：只初始化重置时间，不重置计数
		next := nextTrafficReset(p.TrafficReset, now)
		if !next.IsZero() {
			// 乐观锁：仅当 traffic_reset_at 仍为 nil 时更新
			_, _ = xdb.Where("username=? AND traffic_reset_at IS NULL", username).
				Cols("traffic_reset_at").
				Update(&User{TrafficResetAt: &next})
		}
	} else if now.After(*u.TrafficResetAt) {
		// 周期到达：重置计数并设置下一次重置时间
		next := nextTrafficReset(p.TrafficReset, now)
		if !next.IsZero() {
			// 乐观锁：仅当 traffic_reset_at 仍为旧值时更新，避免与 AddTrafficUsed 产生 lost update
			_, _ = xdb.Where("username=? AND traffic_reset_at=?", username, u.TrafficResetAt.Format("2006-01-02 15:04:05")).
				Cols("traffic_used", "traffic_reset_at").
				Update(&User{TrafficUsed: 0, TrafficResetAt: &next})
		}
		return false, 0
	}
	return u.TrafficUsed >= p.TrafficQuota, u.TrafficUsed
}

// 手动重置用户流量计数
func ResetTrafficUsed(username string, p *Policy) error {
	if p == nil || p.TrafficReset == "" {
		return nil
	}
	next := nextTrafficReset(p.TrafficReset, time.Now())
	if next.IsZero() {
		return nil
	}
	_, err := xdb.Where("username=?", username).
		Cols("traffic_used", "traffic_reset_at").
		Update(&User{TrafficUsed: 0, TrafficResetAt: &next})
	return err
}

// 原子自增用户已用流量
func AddTrafficUsed(username string, delta int64) {
	if delta <= 0 {
		return
	}
	var lastErr error
	for attempt := range 5 {
		_, err := xdb.Exec("UPDATE user SET traffic_used = traffic_used + ? WHERE username = ?", delta, username)
		if err == nil {
			return
		}
		lastErr = err
		// SQLite 并发写锁竞争为瞬时错误，退避重试避免流量更新静默丢失
		if strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "database is busy") {
			time.Sleep(20 * time.Millisecond * time.Duration(attempt+1))
			continue
		}
		break
	}
	if lastErr != nil {
		base.Error("更新用户流量失败:", username, lastErr)
	}
}
