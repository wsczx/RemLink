package base

import "encoding/json"

const (
	LinkModeTUN     = "tun"
	LinkModeTAP     = "tap"
	LinkModeMacvtap = "macvtap"
	LinkModeIpvtap  = "ipvtap"
)

const InitialSetupWarning = "检测到默认初始化配置，请修改系统名称，完成后此提示将自动消失"

type SystemWarning struct {
	Code       string `json:"code"`
	Level      string `json:"level"`
	Message    string `json:"message"`
	ActionPath string `json:"action_path,omitempty"`
}

type ServerConfig struct {
	// 基础信息
	ProfileName   string `json:"profile_name"`
	Issuer        string `json:"issuer"`
	AdminUser     string `json:"admin_user"`
	AdminPass     string `json:"admin_pass"`
	AdminOtp      string `json:"admin_otp"`
	JwtSecret     string `json:"jwt_secret"`
	AdminTemp     bool   `json:"admin_temp"`
	UpgradeSource string `json:"upgrade_source"`

	// 服务监听
	ServerAddr        string `json:"server_addr"`
	ServerDTLS        bool   `json:"server_dtls"`
	ServerDTLSAddr    string `json:"server_dtls_addr"`
	AdvertiseDTLSAddr string `json:"advertise_dtls_addr"`
	AdminAddr         string `json:"admin_addr"`
	ProxyProtocol     bool   `json:"proxy_protocol"`
	FilesPath         string `json:"files_path"`

	// 数据库
	DbType   string `json:"db_type"`
	DbSource string `json:"db_source"`
	ShowSQL  bool   `json:"show_sql"`

	// 虚拟网络
	LinkMode       string `json:"link_mode"`
	MasterDev      string `json:"master_dev"`
	Ipv4CIDR       string `json:"ipv4_cidr"`
	Ipv4Gateway    string `json:"ipv4_gateway"`
	Ipv4Start      string `json:"ipv4_start"`
	Ipv4End        string `json:"ipv4_end"`
	Ipv6CIDR       string `json:"ipv6_cidr"`
	IpLease        int    `json:"ip_lease"`
	FirewallDriver string `json:"firewall_driver"`
	GlobalNat      bool   `json:"global_nat"`
	GlobalNat6     bool   `json:"global_nat6"`

	// 连接控制
	MaxClient       int    `json:"max_client"`
	MaxUserClient   int    `json:"max_user_client"`
	DefaultGroup    string `json:"default_group"`
	CstpKeepalive   int    `json:"cstp_keepalive"`
	CstpDpd         int    `json:"cstp_dpd"`
	MobileKeepalive int    `json:"mobile_keepalive"`
	MobileDpd       int    `json:"mobile_dpd"`
	Mtu             int    `json:"mtu"`
	DefaultDomain   string `json:"default_domain"`
	IdleTimeout     int    `json:"idle_timeout"`
	SessionTimeout  int    `json:"session_timeout"`
	Compression     bool   `json:"compression"`
	NoCompressLimit int    `json:"no_compress_limit"`

	// 日志/调试
	LogPath       string `json:"log_path"`
	LogLevel      string `json:"log_level"`
	HttpServerLog bool   `json:"http_server_log"`
	Pprof         bool   `json:"pprof"`
	AuditInterval int    `json:"audit_interval"`

	// 安全/认证
	DisplayError       bool   `json:"display_error"`
	ExcludeExportIp    bool   `json:"exclude_export_ip"`
	SendOtp            bool   `json:"send_otp"`
	SendOtpType        string `json:"send_otp_type"`
	EncryptionPassword bool   `json:"encryption_password"`

	// 防暴破
	AntiBruteForce bool   `json:"anti_brute_force"`
	IPWhiteList    string `json:"ip_whitelist"`
	IPBlackList    string `json:"ip_blacklist"`

	// 锁定策略
	MaxBanCount                   int `json:"max_ban_score"`
	BanResetTime                  int `json:"ban_reset_time"`
	LockTime                      int `json:"lock_time"`
	MaxGlobalUserBanCount         int `json:"max_global_user_ban_count"`
	GlobalUserBanResetTime        int `json:"global_user_ban_reset_time"`
	GlobalUserLockTime            int `json:"global_user_lock_time"`
	MaxGlobalIPBanCount           int `json:"max_global_ip_ban_count"`
	GlobalIPBanResetTime          int `json:"global_ip_ban_reset_time"`
	GlobalIPLockTime              int `json:"global_ip_lock_time"`
	GlobalLockStateExpirationTime int `json:"global_lock_state_expiration_time"`

	// 门户设置
	EnableUserPortal         bool   `json:"enable_user_portal"`
	EnableWebAuth            bool   `json:"enable_web_auth"`
	WebAuthBrowserMode       string `json:"web_auth_browser_mode"`
	EnableWebAuthGroupFilter bool   `json:"enable_web_auth_group_filter"` // 开启后 Web 认证先输入用户名、按所属用户组过滤可选组（仅支持本地用户认证）；关闭则直接展示全部启用组
	AllowMobileSSO           bool   `json:"allow_mobile_sso"`             // 允许手机端使用 SSO 单点登录（默认关闭；开启后手机端浏览器模式仍强制内置）

	WebVpnDomain    string `json:"webvpn_domain"`     // Web VPN域名
	WebVpnSsoDomain string `json:"webvpn_sso_domain"` // Web 认证单点登录域名

	// WebVPN 会话时效
	WebVpnSessionTTL         int `json:"webvpn_session_ttl"`          // 滑动续期周期(分钟)，距签发超过该值-1h 时续期；0 取默认 60
	WebVpnSessionMaxLifetime int `json:"webvpn_session_max_lifetime"` // 绝对寿命上限(分钟)，首次登录起算，超过强制重新登录；0 取默认 480

	// 高级功能可见性
	ShowFakeDNS bool `json:"show_fakedns"`
}

type configMeta struct {
	defaultVal string
	usage      string
	group      string
	restart    bool
	sensitive  bool
	hidden     bool
	readonly   bool
	multiline  bool
	options    map[string]string // 可选项: 显示名 -> 值
}

var configMetas = map[string]configMeta{
	"profile_name":   {usage: "profile name(用于区分不同服务端的配置)", group: "基础信息", defaultVal: "profile"},
	"issuer":         {usage: "系统名称", group: "基础信息", defaultVal: "XX公司VPN"},
	"admin_user":     {usage: "管理用户名", group: "基础信息", defaultVal: "admin", restart: true},
	"admin_pass":     {usage: "管理用户密码", group: "基础信息", defaultVal: "defaultPwd", sensitive: true, hidden: true},
	"admin_otp":      {usage: "管理用户OTP两步验证密钥,可在安全设置页面扫码绑定", group: "基础信息", sensitive: true, hidden: true},
	"jwt_secret":     {usage: "JWT密钥", group: "基础信息", sensitive: true},
	"admin_temp":     {usage: "管理员仍在使用首次生成或重置后的临时密码", group: "基础信息", hidden: true},
	"upgrade_source": {usage: "在线升级更新源：gitee / github；部署在国内的服务端建议选 gitee", group: "基础信息", defaultVal: "gitee", options: map[string]string{"Gitee": "gitee", "GitHub": "github"}},

	"server_addr":         {usage: "TCP服务监听地址，可只填端口(如 8443)监听所有网卡，或 IP:端口", group: "服务监听", defaultVal: ":443", restart: true},
	"server_dtls":         {usage: "开启DTLS", group: "服务监听", restart: true},
	"server_dtls_addr":    {usage: "DTLS监听地址，可只填端口(如 8443)监听所有网卡，或 IP:端口", group: "服务监听", defaultVal: ":443", restart: true},
	"advertise_dtls_addr": {usage: "DTLS对外映射端口(为空则与server_dtls_addr相同)，可只填端口", group: "服务监听"},
	"admin_addr":          {usage: "后台服务监听地址，可只填端口(如 8800)监听所有网卡，或 IP:端口", group: "服务监听", defaultVal: ":8800", restart: true},
	"proxy_protocol":      {usage: "TCP代理协议", group: "服务监听", restart: true},
	"files_path":          {usage: "外部下载文件路径", group: "服务监听", defaultVal: "./conf/files"},

	"db_type":   {usage: "数据库类型 [sqlite3 mysql postgres mssql]", group: "数据库", defaultVal: "sqlite3", restart: true, readonly: true},
	"db_source": {usage: "数据库source", group: "数据库", defaultVal: "./conf/remlink.db", restart: true, readonly: true},
	"show_sql":  {usage: "显示sql语句，用于调试", group: "数据库"},

	"link_mode":       {usage: "虚拟网络类型", group: "虚拟网络", defaultVal: "tun", restart: true, options: map[string]string{"TUN": "tun", "TAP": "tap", "MACVTAP": "macvtap", "IPVTAP": "ipvtap"}},
	"master_dev":      {usage: "NAT出网主网卡名称", group: "虚拟网络", defaultVal: "eth0", restart: true},
	"ipv4_cidr":       {usage: "ip地址网段", group: "虚拟网络", defaultVal: "192.168.90.0/24", restart: true},
	"ipv4_gateway":    {usage: "ipv4_gateway", group: "虚拟网络", defaultVal: "192.168.90.1", restart: true},
	"ipv4_start":      {usage: "IPV4开始地址", group: "虚拟网络", defaultVal: "192.168.90.100", restart: true},
	"ipv4_end":        {usage: "IPV4结束", group: "虚拟网络", defaultVal: "192.168.90.200", restart: true},
	"ipv6_cidr":       {usage: "IPv6地址池CIDR(如 2001:db8:1::/64，前缀须<128)；为空则纯v4", group: "虚拟网络", restart: true},
	"ip_lease":        {usage: "IP租期(秒)", group: "虚拟网络", defaultVal: "86400"},
	"global_nat":      {usage: "是否自动添加全局NAT(IPv4)", group: "虚拟网络", defaultVal: "true", restart: true},
	"global_nat6":     {usage: "是否自动添加全局NAT66(IPv6)；关闭即纯路由模式，需上游回指v6池", group: "虚拟网络", defaultVal: "true", restart: true},
	"firewall_driver": {usage: "防火墙后端", group: "虚拟网络", defaultVal: "auto", restart: true, options: map[string]string{"自动选择": "auto", "nftables": "nftables", "iptables": "iptables"}},

	"max_client":        {usage: "最大用户连接", group: "连接控制", defaultVal: "200"},
	"max_user_client":   {usage: "最大单用户连接", group: "连接控制", defaultVal: "3"},
	"default_group":     {usage: "默认用户组", group: "连接控制", defaultVal: "one"},
	"cstp_keepalive":    {usage: "keepalive时间(秒)", group: "连接控制", defaultVal: "3"},
	"cstp_dpd":          {usage: "死链接检测时间(秒)", group: "连接控制", defaultVal: "20"},
	"mobile_keepalive":  {usage: "移动端keepalive接检测时间(秒)", group: "连接控制", defaultVal: "4"},
	"mobile_dpd":        {usage: "移动端死链接检测时间(秒)", group: "连接控制", defaultVal: "60"},
	"mtu":               {usage: "最大传输单元MTU", group: "连接控制", defaultVal: "1460"},
	"default_domain":    {usage: "客户端dns的默认搜索域", group: "连接控制", defaultVal: "example.com"},
	"idle_timeout":      {usage: "空闲链接超时时间(秒)-超时后断开链接，0关闭此功能", group: "连接控制"},
	"session_timeout":   {usage: "session过期时间(秒)-用于断线重连，0永不过期", group: "连接控制", defaultVal: "3600"},
	"compression":       {usage: "启用压缩", group: "连接控制"},
	"no_compress_limit": {usage: "低于及等于多少字节不压缩", group: "连接控制", defaultVal: "256"},

	"log_path":        {usage: "日志文件路径,默认标准输出", group: "日志/调试"},
	"log_level":       {usage: "日志等级", group: "日志/调试", defaultVal: "info", options: map[string]string{"Trace": "trace", "Debug": "debug", "Info": "info", "Warn": "warn", "Error": "error"}},
	"http_server_log": {usage: "开启go标准库http.Server的日志", group: "日志/调试"},
	"pprof":           {usage: "开启pprof", group: "日志/调试", defaultVal: "false"},
	"audit_interval":  {usage: "审计去重间隔(秒),-1关闭", group: "日志/调试", defaultVal: "600"},

	"display_error":       {usage: "客户端显示详细错误信息(线上环境慎开启)", group: "安全/认证"},
	"exclude_export_ip":   {usage: "排除出口ip路由(出口ip不加密传输)", group: "安全/认证", defaultVal: "true"},
	"send_otp":            {usage: "是否发送OTP", group: "安全/认证"},
	"send_otp_type":       {usage: "发送OTP方式", group: "安全/认证", options: map[string]string{"邮件": "mail", "短信": "phone"}},
	"encryption_password": {usage: "用户密码是否加密保存", group: "安全/认证", defaultVal: "true"},

	"anti_brute_force": {usage: "是否开启防爆功能", group: "防暴破", defaultVal: "true"},
	"ip_whitelist":     {usage: "IP白名单", group: "防暴破", defaultVal: "192.168.90.1,172.16.0.0/24", multiline: true},
	"ip_blacklist":     {usage: "IP黑名单", group: "防暴破", multiline: true},

	"max_ban_score":                     {usage: "单位时间内最大尝试次数，0为关闭该功能", group: "锁定策略", defaultVal: "5"},
	"ban_reset_time":                    {usage: "设置单位时间(秒)，超过则重置计数", group: "锁定策略", defaultVal: "600"},
	"lock_time":                         {usage: "超过最大尝试次数后的锁定时长(秒)", group: "锁定策略", defaultVal: "300"},
	"max_global_user_ban_count":         {usage: "全局用户单位时间内最大尝试次数，0为关闭该功能", group: "锁定策略", defaultVal: "20"},
	"global_user_ban_reset_time":        {usage: "全局用户设置单位时间(秒)", group: "锁定策略", defaultVal: "600"},
	"global_user_lock_time":             {usage: "全局用户锁定时间(秒)", group: "锁定策略", defaultVal: "300"},
	"max_global_ip_ban_count":           {usage: "全局IP单位时间内最大尝试次数，0为关闭该功能", group: "锁定策略", defaultVal: "40"},
	"global_ip_ban_reset_time":          {usage: "全局IP设置单位时间(秒)", group: "锁定策略", defaultVal: "1200"},
	"global_ip_lock_time":               {usage: "全局IP锁定时间(秒)", group: "锁定策略", defaultVal: "300"},
	"global_lock_state_expiration_time": {usage: "全局锁定状态的保存生命周期(秒),超过则删除记录", group: "锁定策略", defaultVal: "3600"},

	"enable_user_portal":           {usage: "开启用户门户，浏览器访问 VPN 服务地址时进入用户自助页面", group: "门户设置"},
	"enable_web_auth":              {usage: "开启 Web 认证模式，客户端登录改为浏览器认证流程", group: "门户设置"},
	"web_auth_browser_mode":        {usage: "Web 认证浏览器模式", group: "门户设置", defaultVal: "external", options: map[string]string{"内置": "internal", "系统": "external"}},
	"enable_web_auth_group_filter": {usage: "Web 认证先输入用户名，按所属用户组过滤组列表（仅支持本地用户）;关闭则展示全部启用组", group: "门户设置"},
	"webvpn_domain":                {usage: "WebVPN 子域名反代根域名（如 wv.example.com）。解析 *.wv.example.com 到本机，访问 <应用名>.wv.example.com 即反代到对应内网 Web 应用", group: "门户设置"},
	"webvpn_sso_domain":            {usage: "WebVPN 子域名三方登录（企微/飞书/钉钉）跳转认证用的门户域名。留空则子域名登录页不显示三方登录入口", group: "门户设置"},
	"webvpn_session_ttl":           {usage: "WebVPN 会话滑动续期周期（分钟）。用户持续活跃时按此周期刷新登录态，到期前 1 小时触发续期；建议 30~120", group: "门户设置", defaultVal: "60"},
	"webvpn_session_max_lifetime":  {usage: "WebVPN 会话绝对寿命上限（分钟）。自首次登录起算，超过后无论是否活跃都强制重新登录；建议 240~1440", group: "门户设置", defaultVal: "480"},
	"allow_mobile_sso":             {usage: "允许手机端使用 SSO 单点登录（企微/飞书/钉钉等）。默认关闭，开启后手机端浏览器模式仍强制内置", group: "门户设置"},

	"show_fakedns": {usage: "在管理界面显示 FakeDNS 功能入口", group: "高级功能可见性", defaultVal: "false"},
}

// UnmarshalJSON 兼容旧配置键 ipv4_master（重命名为 master_dev 之前）。
// 旧部署升级后，持久化配置(DB)与备份中仍可能含 ipv4_master，需回退到 MasterDev，
// 否则该字段为空、回退默认 eth0，会导致 NAT 出网网卡错误、转发失效。
func (c *ServerConfig) UnmarshalJSON(b []byte) error {
	type alias ServerConfig
	if err := json.Unmarshal(b, (*alias)(c)); err != nil {
		return err
	}
	if c.MasterDev == "" {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(b, &raw); err == nil {
			if v, ok := raw["ipv4_master"]; ok {
				var s string
				if json.Unmarshal(v, &s) == nil && s != "" {
					c.MasterDev = s
				}
			}
		}
	}
	return nil
}

const DefaultProfileXML = `<?xml version="1.0" encoding="UTF-8"?>
<AnyConnectProfile xmlns="http://schemas.xmlsoap.org/encoding/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
                   xsi:schemaLocation="http://schemas.xmlsoap.org/encoding/ AnyConnectProfile.xsd">
    <ClientInitialization>
        <UseStartBeforeLogon>false</UseStartBeforeLogon>
        <AutoConnectOnStart>true</AutoConnectOnStart>
        <StrictCertificateTrust>false</StrictCertificateTrust>
        <RestrictPreferenceCaching>false</RestrictPreferenceCaching>
        <RestrictTunnelProtocols>IPSec</RestrictTunnelProtocols>
        <BypassDownloader>true</BypassDownloader>
        <AutoUpdate>false</AutoUpdate>
        <LocalLanAccess>true</LocalLanAccess>
        <WindowsVPNEstablishment>AllowRemoteUsers</WindowsVPNEstablishment>
        <LinuxVPNEstablishment>AllowRemoteUsers</LinuxVPNEstablishment>
        <CertEnrollmentPin>pinAllowed</CertEnrollmentPin>
        <CertificateStore>User</CertificateStore>
        <AutomaticCertSelection>true</AutomaticCertSelection>
    </ClientInitialization>
    <ServerList>
        <HostEntry>
            <HostName>RemLink</HostName>
            <HostAddress>https://remlink.example.com</HostAddress>
        </HostEntry>
    </ServerList>
</AnyConnectProfile>
`

// 客户端下载页默认 HTML
const DefaultDownloadHtml = `<div style="background:#fff;border:1px solid #edf1f7;border-left:3px solid #0078d4;border-radius:6px;padding:10px 14px;margin-bottom:8px">
<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">
<span style="font-size:13px;font-weight:600;color:#303133">Windows 客户端</span>
<span style="font-size:11px;color:#909399">Win 7 及以上</span>
</div>
<div style="display:flex;gap:8px;flex-wrap:wrap">
<a href="/files/remlink-windows-amd64.exe" style="padding:4px 12px;background:#ecf5ff;color:#409EFF;border:1px solid #d6e4ff;border-radius:4px;font-size:12px;text-decoration:none;font-weight:500">x64 安装包</a>
<a href="/files/remlink-windows-arm64.exe" style="padding:4px 12px;background:#ecf5ff;color:#409EFF;border:1px solid #d6e4ff;border-radius:4px;font-size:12px;text-decoration:none;font-weight:500">ARM64 安装包</a>
</div>
</div>

<div style="background:#fff;border:1px solid #edf1f7;border-left:3px solid #555;border-radius:6px;padding:10px 14px;margin-bottom:8px">
<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">
<span style="font-size:13px;font-weight:600;color:#303133">macOS 客户端</span>
<span style="font-size:11px;color:#909399">macOS 10.13+</span>
</div>
<div style="display:flex;gap:8px;flex-wrap:wrap">
<a href="/files/remlink-macos-amd64.dmg" style="padding:4px 12px;background:#ecf5ff;color:#409EFF;border:1px solid #d6e4ff;border-radius:4px;font-size:12px;text-decoration:none;font-weight:500">Intel 芯片</a>
<a href="/files/remlink-macos-arm64.dmg" style="padding:4px 12px;background:#ecf5ff;color:#409EFF;border:1px solid #d6e4ff;border-radius:4px;font-size:12px;text-decoration:none;font-weight:500">Apple 芯片</a>
</div>
</div>

<div style="background:#fff;border:1px solid #edf1f7;border-left:3px solid #f15a24;border-radius:6px;padding:10px 14px;margin-bottom:8px">
<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">
<span style="font-size:13px;font-weight:600;color:#303133">Linux 客户端</span>
<span style="font-size:11px;color:#909399">主流发行版</span>
</div>
<div style="display:flex;gap:8px;flex-wrap:wrap">
<a href="/files/remlink-linux-amd64.tar.gz" style="padding:4px 12px;background:#ecf5ff;color:#409EFF;border:1px solid #d6e4ff;border-radius:4px;font-size:12px;text-decoration:none;font-weight:500">x64 安装包</a>
<a href="/files/remlink-linux-arm64.tar.gz" style="padding:4px 12px;background:#ecf5ff;color:#409EFF;border:1px solid #d6e4ff;border-radius:4px;font-size:12px;text-decoration:none;font-weight:500">ARM64 安装包</a>
</div>
</div>

<div style="background:#fff;border:1px solid #edf1f7;border-left:3px solid #3ddc84;border-radius:6px;padding:10px 14px;margin-bottom:8px">
<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">
<span style="font-size:13px;font-weight:600;color:#303133">Android 客户端</span>
<span style="font-size:11px;color:#909399">Android 5.0+</span>
</div>
<div style="display:flex;gap:8px;flex-wrap:wrap">
<a href="/files/remlink-android.apk" style="padding:4px 12px;background:#ecf5ff;color:#409EFF;border:1px solid #d6e4ff;border-radius:4px;font-size:12px;text-decoration:none;font-weight:500">APK 安装包</a>
</div>
</div>

<div style="background:#fff;border:1px solid #edf1f7;border-left:3px solid #000;border-radius:6px;padding:10px 14px">
<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">
<span style="font-size:13px;font-weight:600;color:#303133">iOS 客户端</span>
<span style="font-size:11px;color:#909399">iPhone / iPad</span>
</div>
<div style="display:flex;gap:8px;flex-wrap:wrap">
<a href="https://apps.apple.com/app/cisco-anyconnect/id1135064690" target="_blank" style="padding:4px 12px;background:#ecf5ff;color:#409EFF;border:1px solid #d6e4ff;border-radius:4px;font-size:12px;text-decoration:none;font-weight:500">App Store</a>
</div>
</div>`
