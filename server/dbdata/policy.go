package dbdata

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/songgao/water/waterutil"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
)

// 策略与流量配额的存储入口
type PolicyStore struct{ *table[Policy] }

// 策略与流量配额的存储入口实例，由 SetDB 注入持久化实现
var Policies = &PolicyStore{}

// 端口规则校验用的预编译正则，避免每次解析策略时重复编译
var (
	rePortsAll  = regexp.MustCompile(`^\d{1,5}(-\d{1,5})?(,\d{1,5}(-\d{1,5})?)*$`)
	rePortRange = regexp.MustCompile(`^\d{1,5}-\d{1,5}$`)
)

// 按用户名隔离流量记录锁，避免 SQLite 单写者锁下并发写丢失更新；不同用户互不阻塞
var trafficMu sync.Map

func userTrafficMu(username string) *sync.Mutex {
	m, _ := trafficMu.LoadOrStore(username, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// 清除引用指定策略的用户的 TrafficResetAt
// 策略配额配置变更后调用，让 QuotaExceeded 重新初始化重置时间
func (st *PolicyStore) clearTrafficResetAt(policyId int) {
	usernames := map[string]struct{}{}

	if users, err := Users.FindWhere(0, 0, "policy_id=?", policyId); err == nil {
		for _, u := range users {
			usernames[u.Username] = struct{}{}
		}
	}

	if groups, err := Groups.FindWhere(0, 0, "policy_id=?", policyId); err == nil {
		if len(groups) > 0 {
			groupNames := make([]string, 0, len(groups))
			for _, g := range groups {
				groupNames = append(groupNames, g.Name)
			}
			if groupUsers, err := Users.InGroups(groupNames); err == nil {
				for _, u := range groupUsers {
					usernames[u.Username] = struct{}{}
				}
			}
		}
	}

	for username := range usernames {
		if _, err := GetDB().Engine().Where("username=?", username).
			Cols("traffic_reset_at").
			Update(&User{}); err != nil {
			base.Error("清除流量重置标记失败:", username, err)
		}
	}
}

// 预处理 FakeDNS 域名规则为 HashMap
func (p *Policy) AddFakeDNSRules() {
	p.FakeDNSIncludeSet = addDomainSet(p.FakeDNSInclude)
	p.FakeDNSExcludeSet = addDomainSet(p.FakeDNSExclude)
}

// 返回 FakeDNS 上游 DNS：优先专用配置，其次客户端 DNS，最后内置默认值
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

// 根据 ID 加载策略并补齐 FakeDNS 规则
func (st *PolicyStore) LoadPolicyById(id int) (*Policy, error) {
	p := &Policy{}
	err := st.Get(id, p)
	if err != nil {
		return nil, fmt.Errorf("策略加载失败: %w", err)
	}
	p.AddFakeDNSRules()
	return p, nil
}

// 返回所有启用策略的 ID+名称列表（供下拉选择）
func (st *PolicyStore) GetPolicyNames() []GroupNameId {
	datas, err := st.FindWhere(0, 0, "status=1")
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
func (st *PolicyStore) GetAllPolicyNames() []GroupNameId {
	datas, err := st.Find(0, 0)
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
func (st *PolicyStore) UsedByGroups(policyId int) []Group {
	groups, err := Groups.FindWhere(0, 0, "policy_id=?", policyId)
	if err != nil {
		base.Error(err)
		return nil
	}
	return groups
}

// 返回引用了指定策略的用户列表
func (st *PolicyStore) UsedByUsers(policyId int) []User {
	users, err := Users.FindWhere(0, 0, "policy_id=?", policyId)
	if err != nil {
		base.Error(err)
		return nil
	}
	return users
}

// 新增或更新策略配置
func (st *PolicyStore) SetPolicy(p *Policy) error {
	if err := p.validateBasics(); err != nil {
		return err
	}
	isDefRoute, err := p.normalizeRoutes()
	if err != nil {
		return err
	}
	if err := p.normalizeAcl(); err != nil {
		return err
	}
	if err := p.normalizeDns(isDefRoute); err != nil {
		return err
	}
	if err := p.validateDomains(isDefRoute); err != nil {
		return err
	}
	return st.persistPolicy(p)
}

// 基础字段校验与流量重置周期的规范化
func (p *Policy) validateBasics() error {
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
	return nil
}

// 规范化一条路由列表，返回剔除空值与补全掩码后的结果
func normalizeRouteList(items []ValData, field string, allowAll bool) ([]ValData, error) {
	out := []ValData{}
	for _, v := range items {
		if v.Val == "" {
			continue
		}
		if allowAll && v.Val == ALL {
			out = append(out, v)
			continue
		}
		ipMask, ipNet, err := parseIpNet(v.Val)
		if err != nil {
			return nil, fmt.Errorf("%s 错误%w", field, err)
		}
		if strings.Split(ipMask, "/")[0] != ipNet.IP.String() {
			return nil, errors.New(field + " 错误: " + fmt.Sprintf("网络地址错误，建议： %s 改为 %s", v.Val, ipNet))
		}
		v.IpMask = ipMask
		out = append(out, v)
	}
	return out, nil
}

// 规范化包含/排除路由，并判定是否为默认路由
func (p *Policy) normalizeRoutes() (bool, error) {
	include, err := normalizeRouteList(p.RouteInclude, "RouteInclude", true)
	if err != nil {
		return false, err
	}
	exclude, err := normalizeRouteList(p.RouteExclude, "RouteExclude", false)
	if err != nil {
		return false, err
	}
	p.RouteInclude, p.RouteExclude = include, exclude
	return len(include) == 0 || (len(include) == 1 && include[0].Val == "all"), nil
}

// 校验端口表达式：逗号分隔的端口或端口区间
func validatePolicyPorts(portsStr string) error {
	if !rePortsAll.MatchString(portsStr) {
		return errors.New("端口: " + portsStr + " 格式错误,请用逗号分隔的端口,比如: 22,80,443 连续端口用-,比如:1234-5678")
	}
	for pt := range strings.SplitSeq(portsStr, ",") {
		if pt == "" {
			continue
		}
		if rePortRange.MatchString(pt) {
			rp := strings.Split(pt, "-")
			portfrom, err := strconv.ParseUint(rp[0], 10, 16)
			if err != nil {
				return fmt.Errorf("端口:%s 格式错误, %w", rp[0], err)
			}
			portto, err := strconv.ParseUint(rp[1], 10, 16)
			if err != nil {
				return fmt.Errorf("端口:%s 格式错误, %w", rp[1], err)
			}
			if portfrom > portto {
				return errors.New("端口范围错误: 起始端口 " + rp[0] + " 不能大于结束端口 " + rp[1])
			}
		} else if _, err := strconv.ParseUint(pt, 10, 16); err != nil {
			return fmt.Errorf("端口:%s 格式错误, %w", pt, err)
		}
	}
	return nil
}

// 规范化链路 ACL：解析网段、协议与端口
func (p *Policy) normalizeAcl() error {
	linkAcl := []GroupLinkAcl{}
	for _, v := range p.LinkAcl {
		if v.Val == "" {
			continue
		}
		_, ipNet, err := parseIpNet(v.Val)
		if err != nil {
			return fmt.Errorf("GroupLinkAcl 错误%w", err)
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
		if err := validatePolicyPorts(portsStr); err != nil {
			return err
		}
		linkAcl = append(linkAcl, v)
	}
	p.LinkAcl = linkAcl
	return nil
}

// 规范化客户端 DNS；默认路由必须配置 DNS
func (p *Policy) normalizeDns(isDefRoute bool) error {
	clientDns := []ValData{}
	for _, v := range p.ClientDns {
		v.Val = strings.TrimSpace(v.Val)
		if v.Val == "" {
			continue
		}
		ip := net.ParseIP(v.Val)
		if ip.String() != v.Val {
			return errors.New("DNS IP 错误")
		}
		clientDns = append(clientDns, v)
	}
	if isDefRoute && len(clientDns) == 0 {
		return errors.New("默认路由，必须设置一个DNS")
	}
	p.ClientDns = clientDns
	return nil
}

// 校验 FakeDNS 配置并规范化；未启用时清空相关字段
func (p *Policy) normalizeFakeDns() error {
	if !p.EnableFakeDNS {
		p.FakeDNSInclude = ""
		p.FakeDNSExclude = ""
		p.FakeDNSUpstream = ""
		p.PreferIPv6 = false
		return nil
	}
	if len(p.ClientDns) == 0 && p.FakeDNSUpstream == "" {
		return errors.New("未配置客户端 DNS 时,必须配置 FakeDNS 上游 DNS,否则 FakeDNS 功能无法生效")
	}
	p.FakeDNSUpstream = strings.TrimSpace(p.FakeDNSUpstream)
	if p.FakeDNSUpstream != "" {
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
	if err := CheckFakeDNSDomains(p.FakeDNSInclude); err != nil {
		return fmt.Errorf("FakeDNS 包含域名有误：%w", err)
	}
	if err := CheckFakeDNSDomains(p.FakeDNSExclude); err != nil {
		return fmt.Errorf("FakeDNS 排除域名有误：%w", err)
	}
	return nil
}

// 校验域名拆分隧道与 FakeDNS 配置
func (p *Policy) validateDomains(isDefRoute bool) error {
	p.DsIncludeDomains = strings.TrimSpace(p.DsIncludeDomains)
	p.DsExcludeDomains = strings.TrimSpace(p.DsExcludeDomains)
	if p.DsIncludeDomains != "" && p.DsExcludeDomains != "" {
		return errors.New("包含/排除域名不能同时填写")
	}
	if err := CheckDomainNames(p.DsIncludeDomains); err != nil {
		return fmt.Errorf("包含域名有误：%w", err)
	}
	if err := CheckDomainNames(p.DsExcludeDomains); err != nil {
		return fmt.Errorf("排除域名有误：%w", err)
	}
	if isDefRoute && p.DsIncludeDomains != "" {
		return errors.New("默认路由, 不允许设置\"包含域名\", 请重新配置")
	}
	return p.normalizeFakeDns()
}

// 持久化策略；流量配额周期变更时清除重置时间
func (st *PolicyStore) persistPolicy(p *Policy) error {
	p.UpdatedAt = time.Now()
	if p.Id == 0 {
		return st.Insert(p)
	}
	var old Policy
	if err := st.Get(p.Id, &old); err == nil {
		if old.TrafficReset != p.TrafficReset || old.TrafficQuota != p.TrafficQuota {
			st.clearTrafficResetAt(p.Id)
		}
	}
	return st.Save(p)
}

// 加载生效策略，优先级：用户策略 > 组策略
// 返回 nil 表示加载失败或策略已停用
func (st *PolicyStore) ApplyPolicy(username string, g *Group) *Policy {
	user := &User{}
	_ = Users.GetByUsername(username, user)

	if user.PolicyId > 0 {
		userPT, err := st.LoadPolicyById(user.PolicyId)
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

	groupPT, err := st.LoadPolicyById(g.PolicyId)
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

// 检查用户流量配额是否超出
// 自动处理周期重置和首次初始化，使用乐观锁避免并发 lost update
func (st *PolicyStore) QuotaExceeded(username string, p *Policy) (bool, int64) {
	if p == nil || p.TrafficQuota <= 0 {
		return false, 0
	}
	u := &User{}
	if err := Users.GetByUsername(username, u); err != nil {
		return false, 0
	}
	now := time.Now()
	if u.TrafficResetAt == nil {

		next := nextTrafficReset(p.TrafficReset, now)
		if !next.IsZero() {

			if _, err := GetDB().Engine().Where("username=? AND traffic_reset_at IS NULL", username).
				Cols("traffic_reset_at").
				Update(&User{TrafficResetAt: &next}); err != nil {
				base.Error("初始化流量重置时间失败:", username, err)
			}
		}
	} else if now.After(*u.TrafficResetAt) {

		next := nextTrafficReset(p.TrafficReset, now)
		if !next.IsZero() {

			if _, err := GetDB().Engine().Where("username=? AND traffic_reset_at=?", username, u.TrafficResetAt.Format("2006-01-02 15:04:05")).
				Cols("traffic_used", "traffic_reset_at").
				Update(&User{TrafficUsed: 0, TrafficResetAt: &next}); err != nil {
				base.Error("重置流量计数失败:", username, err)
			}
		}
		return false, 0
	}
	return u.TrafficUsed >= p.TrafficQuota, u.TrafficUsed
}

// 手动重置用户流量计数
func (st *PolicyStore) ResetTrafficUsed(username string, p *Policy) error {
	if p == nil || p.TrafficReset == "" {
		return nil
	}
	next := nextTrafficReset(p.TrafficReset, time.Now())
	if next.IsZero() {
		return nil
	}
	mu := userTrafficMu(username)
	mu.Lock()
	defer mu.Unlock()
	_, err := GetDB().Engine().Where("username=?", username).
		Cols("traffic_used", "traffic_reset_at").
		Update(&User{TrafficUsed: 0, TrafficResetAt: &next})
	return err
}

// 原子自增用户已用流量
func (st *PolicyStore) AddTrafficUsed(username string, delta int64) {
	if delta <= 0 {
		return
	}
	mu := userTrafficMu(username)
	mu.Lock()
	defer mu.Unlock()
	var lastErr error
	for attempt := range 5 {
		_, err := GetDB().Engine().Exec("UPDATE user SET traffic_used = traffic_used + ? WHERE username = ?", delta, username)
		if err == nil {
			return
		}
		lastErr = err

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

// 按 ID 加载策略（不附加 FakeDNS 规则，区别于 LoadPolicyById）
func (st *PolicyStore) Get(id int, dest *Policy) error {
	return GetDB().Get("Id", id, dest)
}

// 按名称加载策略
func (st *PolicyStore) ByName(name string, dest *Policy) error {
	return GetDB().Get("Name", name, dest)
}

// 按 ID 删除策略
func (st *PolicyStore) Delete(id int) error {
	return GetDB().Delete(&Policy{Id: id})
}

// 返回启用状态的策略数量
func (st *PolicyStore) CountActive() int {
	return GetDB().CountWhere(&Policy{}, "status=1")
}

// 返回全量策略列表
func (st *PolicyStore) FindAll(dest *[]Policy) error {
	return GetDB().Find(dest, 0, 0)
}

var ErrPolicyNotFound = errors.New("策略不存在")

// 按 ID 加载策略
func (st *PolicyStore) GetByID(id int) (*Policy, error) {
	p := &Policy{}
	if err := st.Get(id, p); err != nil {
		if utils.CheckErrNotFound(err) {
			return nil, ErrPolicyNotFound
		}
		return nil, fmt.Errorf("加载策略 %d 失败: %w", id, err)
	}
	return p, nil
}
