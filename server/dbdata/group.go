package dbdata

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/songgao/water/waterutil"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// 用户组与外部认证源同步的存储入口
type GroupStore struct{ *table[Group] }

// 用户组与外部认证源同步的存储入口实例，由 SetDB 注入持久化实现
var Groups = &GroupStore{}

const (
	Allow = "allow"
	Deny  = "deny"
	ALL   = "all"
	TCP   = "tcp"
	UDP   = "udp"
	ICMP  = "icmp"
)

// 域名分流最大字符2万
const DsMaxLen = 20000

// 自上而下匹配，默认 allow * *
type GroupLinkAcl struct {
	Action   string               `json:"action"`      // allow、deny
	Protocol string               `json:"protocol"`    // 支持 ALL、TCP、UDP、ICMP 协议
	IpProto  waterutil.IPProtocol `json:"ip_protocol"` // 判断协议使用
	Val      string               `json:"val"`
	Port     string               `json:"port"` // 逗号分隔的端口或端口区间
	Ports    map[uint16]int8      `json:"-"`    // 运行时匹配用，不序列化进库（避免大端口范围撑爆 TEXT 列）
	IpNet    *net.IPNet           `json:"ip_net"`
	Note     string               `json:"note"`
}

type ValData struct {
	Val    string `json:"val"`
	IpMask string `json:"ip_mask"`
	Note   string `json:"note"`
}

type GroupNameId struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

// 判断两个 CIDR 是否重叠（任一网段网络地址落在另一网段内即视为重叠）
func cidrOverlaps(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

// 校验组自定义网段 ipNet 是否与全局 VPN 池或其他组的自定义网段重叠
func (g *Group) checkCidrOverlap(ipNet *net.IPNet, isV6 bool) error {

	globalCIDR := base.GetCfg().Ipv4CIDR
	if isV6 {
		globalCIDR = base.GetCfg().Ipv6CIDR
	}
	if globalCIDR != "" {
		if _, gNet, err := net.ParseCIDR(globalCIDR); err == nil && cidrOverlaps(gNet, ipNet) {
			return fmt.Errorf("组网段 %s 与全局 VPN 池 %s 重叠", ipNet.String(), globalCIDR)
		}
	}

	// 与其他组自定义网段重叠检测
	var others []Group
	if err := GetDB().Find(&others, 0, 0); err != nil {

		base.Warn("检查组网段重叠失败:", err)
		return nil
	}
	for _, og := range others {
		if og.Id == g.Id {
			continue
		}
		c := og.ClientCidr
		if isV6 {
			c = og.ClientCidr6
		}
		if c == "" {
			continue
		}
		if _, oNet, err := net.ParseCIDR(c); err == nil && cidrOverlaps(oNet, ipNet) {
			return fmt.Errorf("组网段 %s 与组 %q 的网段 %s 重叠", ipNet.String(), og.Name, c)
		}
	}

	if masterDev := base.GetCfg().MasterDev; masterDev != "" {
		if iface, err := net.InterfaceByName(masterDev); err == nil {
			if addrs, err := iface.Addrs(); err == nil {
				for _, a := range addrs {
					if _, mNet, err := net.ParseCIDR(a.String()); err == nil {

						isMv6 := mNet.IP.To4() == nil
						if isMv6 != isV6 {
							continue
						}
						if cidrOverlaps(mNet, ipNet) {
							return fmt.Errorf("组网段 %s 与母网卡 %s 物理网段 %s 重叠（macvtap/ipvtap 模式下会冲突）",
								ipNet.String(), masterDev, mNet.String())
						}
					}
				}
			}
		}
	}
	return nil
}

var (
	certAuthCacheMux sync.RWMutex
	certAuthCached   bool
	certAuthCacheAt  time.Time
	certAuthCacheTTL = 30 * time.Second
)

func parseIpNet(s string) (string, *net.IPNet, error) {
	ip, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		return "", nil, err
	}

	mask := net.IP(ipNet.Mask)
	ipMask := fmt.Sprintf("%s/%s", ip, mask)

	return ipMask, ipNet, nil
}

// 校验域名拆分规则格式
func CheckDomainNames(domains string) error {
	if domains == "" {
		return nil
	}
	strLen := 0
	strSlice := strings.SplitSeq(domains, ",")
	for val := range strSlice {
		if val == "" {
			return errors.New(val + " 请以逗号分隔域名")
		}
		if !utils.ValidateDomainName(val) {
			return errors.New(val + " 域名有误")
		}
		strLen += len(val)
	}
	if strLen > DsMaxLen {
		p := message.NewPrinter(language.English)
		return fmt.Errorf("字符长度超出限制，最大%s个(不包含逗号), 请删减一些域名", p.Sprintf("%d", DsMaxLen))
	}
	return nil
}

// 校验 FakeDNS 域名规则格式
func CheckFakeDNSDomains(domains string) error {
	if domains == "" {
		return nil
	}
	strSlice := strings.SplitSeq(domains, ",")
	for val := range strSlice {
		if val == "" {
			return errors.New(val + " 请以逗号分隔域名")
		}
		if !utils.ValidateDomainName(val) {
			return errors.New(val + " 域名有误")
		}
	}
	return nil
}

// 返回所有用户组名称
func (st *GroupStore) GetGroupNames() []string {
	var datas []Group
	err := GetDB().Find(&datas, 0, 0)
	if err != nil {
		base.Error(err)
		return nil
	}
	if len(datas) == 0 {
		return []string{}
	}
	var names []string
	for _, v := range datas {
		names = append(names, v.Name)
	}
	return names
}

// 返回所有启用状态且对客户端可见的用户组名称
// 用户客户端证书登录场景，hidden=1 的组不返回
func (st *GroupStore) GetGroupNamesNormal() []string {
	var datas []Group
	err := GetDB().FindWhere(&datas, 0, 0, "status=1")
	if err != nil {
		base.Error(err)
		return nil
	}
	if len(datas) == 0 {
		return []string{}
	}
	var names []string
	for _, v := range datas {
		if v.Hidden != 0 {
			continue
		}
		names = append(names, v.Name)
	}
	if len(names) == 0 {
		return []string{}
	}
	return names
}

// 返回所有启用状态的用户组
func (st *GroupStore) GetAllGroups() ([]Group, error) {
	var groups []Group
	if err := GetDB().FindWhere(&groups, 0, 0, "status=1"); err != nil {
		base.Error(err)
		return nil, err
	}
	return groups, nil
}

// 返回所有用户组的 ID-名称映射
func (st *GroupStore) GetGroupNamesIds() []GroupNameId {
	var datas []Group
	err := GetDB().Find(&datas, 0, 0)
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

// 新增或更新用户组配置（仅持久化；组员会话吊销等联动须由 service.SaveGroup 触发）
func (st *GroupStore) SetGroup(g *Group) error {
	if err := g.validateBasics(); err != nil {
		return err
	}
	if err := g.normalizeSplitDns(); err != nil {
		return err
	}
	if err := g.validateNetwork(); err != nil {
		return err
	}
	if err := g.validateAuthProfile(); err != nil {
		return err
	}
	return st.persistGroup(g)
}

// 组名、启用状态与引用策略的校验
func (g *Group) validateBasics() error {
	if g.Name == "" {
		return errors.New("用户组名错误")
	}
	if g.Status == 1 && g.PolicyId <= 0 {
		return errors.New("启用的用户组必须指定策略")
	}
	if g.PolicyId > 0 {
		var policy Policy
		if err := GetDB().Get("Id", g.PolicyId, &policy); err != nil {
			return errors.New("引用的策略不存在")
		}
		if policy.Status != 1 {
			return errors.New("引用的策略已停用，请选择启用的策略")
		}
	}
	return nil
}

// 规范化拆分 DNS 列表：trim、去空、校验域名
func (g *Group) normalizeSplitDns() error {
	splitDns := []ValData{}
	for _, v := range g.SplitDns {
		v.Val = strings.TrimSpace(v.Val)
		if v.Val == "" {
			continue
		}
		if !utils.ValidateDomainName(v.Val) {
			return errors.New("域名 错误")
		}
		splitDns = append(splitDns, v)
	}
	g.SplitDns = splitDns
	return nil
}

// 统计独立 v4 网段的四项填写数
func (g *Group) countCidrFields() int {
	n := 0
	for _, f := range []string{g.ClientCidr, g.ClientStart, g.ClientEnd, g.ClientGateway} {
		if f != "" {
			n++
		}
	}
	return n
}

// 校验独立 v4 网段与地址范围
func (g *Group) validateV4Cidr() error {
	_, ipNet, err := net.ParseCIDR(g.ClientCidr)
	if err != nil {
		return fmt.Errorf("组 IP 网段格式无效: %w", err)
	}
	if ipNet.IP.To4() == nil {
		return errors.New("组 IP 网段必须是 IPv4 CIDR（不能填 IPv6）")
	}
	start := net.ParseIP(g.ClientStart)
	end := net.ParseIP(g.ClientEnd)
	gateway := net.ParseIP(g.ClientGateway)
	for _, a := range []struct {
		name string
		ip   net.IP
	}{
		{"起始地址", start},
		{"结束地址", end},
		{"网关地址", gateway},
	} {
		if a.ip == nil {
			return fmt.Errorf("组 IP %s 格式无效", a.name)
		}
		if !ipNet.Contains(a.ip) {
			return fmt.Errorf("组 IP %s 不在网段内", a.name)
		}
	}
	if utils.Ip2long(start) >= utils.Ip2long(end) {
		return errors.New("组 IP 起始地址必须小于结束地址")
	}
	return g.checkCidrOverlap(ipNet, false)
}

// 校验独立 v6 网段；v4 四项未填齐时不允许单独配 v6
func (g *Group) validateV6Cidr(v4FieldCount int) error {
	g.ClientCidr6 = strings.TrimSpace(g.ClientCidr6)
	if g.ClientCidr6 == "" {
		return nil
	}
	if v4FieldCount != 4 {
		return errors.New("组 IPv6 网段需要先启用独立 IP 段（填写 v4 网段等 4 项）")
	}
	_, v6Net, err := net.ParseCIDR(g.ClientCidr6)
	if err != nil {
		return fmt.Errorf("组 IPv6 网段格式无效: %w", err)
	}
	if v6Net.IP.To4() != nil {
		return errors.New("组 IPv6 网段必须是 IPv6 CIDR")
	}
	if ones, bits := v6Net.Mask.Size(); bits != 128 || ones >= 128 {
		return errors.New("组 IPv6 网段前缀须小于 128 才有分配空间")
	}
	return g.checkCidrOverlap(v6Net, true)
}

// 校验出网网卡与独立 v4/v6 网段
func (g *Group) validateNetwork() error {
	if g.OutDev != "" {
		if _, err := net.InterfaceByName(g.OutDev); err != nil {
			return fmt.Errorf("出网网卡 %s 不存在", g.OutDev)
		}
	}
	n := g.countCidrFields()
	switch n {
	case 0:
	case 4:
		if err := g.validateV4Cidr(); err != nil {
			return err
		}
	default:
		return errors.New("组 IP 配置必须全部填写：网段、起始地址、结束地址、网关")
	}
	return g.validateV6Cidr(n)
}

// 校验认证流水线配置；未配置时补默认本地口令认证
func (g *Group) validateAuthProfile() error {
	if len(g.AuthProfile) == 0 {
		g.AuthProfile = json.RawMessage(`{"step":[{"type":"local"}]}`)
	}
	profile, err := auth.ParseAuthProfile(g.AuthProfile)
	if err != nil {
		return fmt.Errorf("认证配置格式无效: %w", err)
	}
	for i, step := range profile.Step {
		if !auth.Registry.IsRegistered(step.Type) {
			return fmt.Errorf("未知的认证方式 %q (步骤 %d)", step.Type, i+1)
		}
		switch step.Type {
		case "ldap", "radius", "wxwork", "feishu", "dingtalk":
			if step.Provider == "" {
				return fmt.Errorf("认证类型 %q 必须设置 Provider (步骤 %d)", step.Type, i+1)
			}
		default:
			if step.Provider != "" {
				return fmt.Errorf("认证类型 %q 不支持 Provider 引用 (步骤 %d)", step.Type, i+1)
			}
		}
	}
	return nil
}

// 持久化用户组
func (st *GroupStore) persistGroup(g *Group) error {
	g.UpdatedAt = time.Now()
	var err error
	if g.Id > 0 {
		err = GetDB().Save(g)
	} else {
		err = GetDB().Insert(g)
	}
	return err
}

// 用指定认证配置校验用户名口令，通过返回 nil（后台"测试认证配置"入口）
func (st *GroupStore) VerifyAuth(name, pwd string, authProfile json.RawMessage) error {
	profile, err := auth.ParseAuthProfile(authProfile)
	if err != nil {
		return fmt.Errorf("认证配置解析失败: %w", err)
	}

	pipeline, err := auth.GetPipeline(*profile, Providers.ResolveConfig)
	if err != nil {
		return err
	}

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  name,
			Password:  pwd,
			GroupName: "",
		},
	}

	result, err := pipeline.Run(ctx)
	if err != nil {
		return err
	}

	switch result {
	case auth.StepPass:
		return nil
	case auth.StepFail:
		return fmt.Errorf("认证失败")
	case auth.StepPending:
		return fmt.Errorf("该认证流程需要额外交互（如 OTP 验证码），无法在此测试")
	default:
		return fmt.Errorf("认证返回未知状态: %v", result)
	}
}

// 是否有启用组开启了客户端证书（cert）认证；TLS 层据此决定是否请求客户端证书
// 30 秒 TTL 缓存避免每次握手全表扫描组配置
func (st *GroupStore) AnyGroupHasCertAuth() bool {
	certAuthCacheMux.RLock()
	if time.Since(certAuthCacheAt) < certAuthCacheTTL {
		v := certAuthCached
		certAuthCacheMux.RUnlock()
		return v
	}
	certAuthCacheMux.RUnlock()

	groups, err := st.GetAllGroups()
	has := false
	if err == nil {
		for _, g := range groups {
			if st.HasAuthType(g.AuthProfile, "cert") {
				has = true
				break
			}
		}
	}

	certAuthCacheMux.Lock()
	certAuthCached = has
	certAuthCacheAt = time.Now()
	certAuthCacheMux.Unlock()
	return has
}

// 组认证配置变更后调用，使证书认证缓存立即失效，保证 TLS 层及时响应
func (st *GroupStore) InvalidateCertAuthCache() {
	certAuthCacheMux.Lock()
	certAuthCacheAt = time.Time{}
	certAuthCacheMux.Unlock()
}

// 检查 AuthProfile 中是否包含指定认证类型
func (st *GroupStore) HasAuthType(authProfile json.RawMessage, authType string) bool {
	profile, err := auth.ParseAuthProfile(authProfile)
	if err != nil {
		return false
	}
	for _, step := range profile.Step {
		if step.Type == authType {
			return true
		}
	}
	return false
}

// 判断组的认证配置是否为组合认证（不少于 2 个步骤）
func (st *GroupStore) HasMultiAuth(authProfile json.RawMessage) bool {
	profile, err := auth.ParseAuthProfile(authProfile)
	if err != nil {
		return false
	}
	return profile.HasMultiStep()
}

// 检查组是否引用了指定 Provider
func (st *GroupStore) UsesProvider(g *Group, providerName string) bool {
	profile, err := auth.ParseAuthProfile(g.AuthProfile)
	if err != nil {
		return false
	}
	for _, step := range profile.Step {
		if step.Provider == providerName {
			return true
		}
	}
	return false
}

// 组配置了外部认证 + OTP 时自动同步用户到本地
func (st *GroupStore) SyncExternalUsersForOTP(g *Group) {
	if !st.HasAuthType(g.AuthProfile, "otp") {
		return
	}

	if st.HasAuthType(g.AuthProfile, "ldap") {
		authLdap, err := st.ResolveLdapConfig(g)
		if err != nil {
			base.Error("解析LDAP配置失败:", err)
		} else {
			authLdap.EnableOtp = true
			go func() {
				if err := authLdap.SaveUsers(g); err != nil {
					base.Error("LDAP用户同步失败:", g.Name, err)
				} else {
					base.Info("LDAP用户同步成功:", g.Name)
				}
			}()
		}
	}

	if st.HasAuthType(g.AuthProfile, "wxwork") {
		authWx, err := st.ResolveWxworkConfig(g)
		if err != nil {
			base.Error("解析企微配置失败:", err)
		} else {
			go func() {
				if err := authWx.SaveUsers(g); err != nil {
					base.Error("企微用户同步失败:", g.Name, err)
				} else {
					base.Info("企微用户同步成功:", g.Name)
				}
			}()
		}
	}

	if st.HasAuthType(g.AuthProfile, "feishu") {
		authFs, err := st.ResolveFeishuConfig(g)
		if err != nil {
			base.Error("解析飞书配置失败:", err)
		} else {
			go func() {
				if err := authFs.SaveUsers(g); err != nil {
					base.Error("飞书用户同步失败:", g.Name, err)
				} else {
					base.Info("飞书用户同步成功:", g.Name)
				}
			}()
		}
	}

	if st.HasAuthType(g.AuthProfile, "dingtalk") {
		authDt, err := st.ResolveDingtalkConfig(g)
		if err != nil {
			base.Error("解析钉钉配置失败:", err)
		} else {
			go func() {
				if err := authDt.SaveUsers(g); err != nil {
					base.Error("钉钉用户同步失败:", g.Name, err)
				} else {
					base.Info("钉钉用户同步成功:", g.Name)
				}
			}()
		}
	}
}

// 返回引用了指定策略的用户组数量
func (st *GroupStore) CountByPolicy(policyId int) int {
	return GetDB().CountWhere(&Group{}, "policy_id=?", policyId)
}

// 按 ID 加载用户组
func (st *GroupStore) Get(id int, dest *Group) error {
	return GetDB().Get("Id", id, dest)
}

// 按 ID 加载用户组，返回指针
func (st *GroupStore) GetByID(id int) (*Group, error) {
	var g Group
	if err := st.Get(id, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func (st *GroupStore) ByName(name string) (*Group, error) {
	g := &Group{}
	if err := GetDB().Get("name", name, g); err != nil {
		return nil, err
	}
	return g, nil
}

// 按名称加载用户组
func (st *GroupStore) GetByName(name string, dest *Group) error {
	return GetDB().Get("Name", name, dest)
}

// 返回全量用户组列表
func (st *GroupStore) FindAll(dest *[]Group) error {
	return GetDB().Find(dest, 0, 0)
}

// 返回启用状态的用户组数量
func (st *GroupStore) CountActive() int {
	return GetDB().CountWhere(&Group{}, "status=1")
}

// 返回已分配策略的用户组数量
func (st *GroupStore) CountWithPolicy() int {
	return GetDB().CountWhere(&Group{}, "policy_id>0")
}

// 返回用户组总行数
func (st *GroupStore) Count() int {
	return GetDB().Count(&Group{})
}

// 批量把组改挂到指定策略
func (st *GroupStore) SetPolicy(ids []int, policyID int) (int, error) {
	count := 0
	for _, id := range ids {
		g, err := st.GetByID(id)
		if err != nil {
			continue
		}
		g.PolicyId = policyID
		if err := st.Save(g); err != nil {
			return count, fmt.Errorf("更新组 %s 失败: %w", g.Name, err)
		}
		count++
	}
	return count, nil
}

// 批量把当前引用指定策略的组移出该策略
func (st *GroupStore) ClearPolicy(ids []int, policyID int) (int, error) {
	count := 0
	for _, id := range ids {
		g, err := st.GetByID(id)
		if err != nil || g.PolicyId != policyID {
			continue
		}
		g.PolicyId = 0
		if err := st.Save(g); err != nil {
			return count, fmt.Errorf("移出用户组 %s 失败: %w", g.Name, err)
		}
		count++
	}
	return count, nil
}
