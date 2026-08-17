package dbdata

import (
	"encoding/json"
	"time"

	"github.com/wsczx/remlink/pkg/security"
)

type Group struct {
	Id          int             `json:"id" xorm:"pk autoincr not null"`
	Name        string          `json:"name" xorm:"varchar(60) not null unique"`
	Note        string          `json:"note" xorm:"varchar(255)"`
	SplitDns    []ValData       `json:"split_dns" xorm:"Text"`
	PolicyId    int             `json:"policy_id" xorm:"Int"`     // 引用策略 ID
	AuthProfile json.RawMessage `json:"auth_profile" xorm:"Text"` // 认证方式（Pipeline 格式 JSON）
	Status      int8            `json:"status" xorm:"Int"`        // 1正常

	ClientCidr    string `json:"client_cidr" xorm:"varchar(32)"`    // IP 网段
	ClientStart   string `json:"client_start" xorm:"varchar(32)"`   // 起始IP
	ClientEnd     string `json:"client_end" xorm:"varchar(32)"`     // 结束IP
	ClientGateway string `json:"client_gateway" xorm:"varchar(32)"` // 网关地址
	ClientCidr6   string `json:"client_cidr6" xorm:"varchar(64)"`   // IPv6 网段（单 CIDR 自动分配，需启用独立IP段；空=使用全局 v6 池）
	OutDev        string `json:"out_dev" xorm:"varchar(64)"`        // 出网网卡；空=沿用全局 master_dev

	CreatedAt time.Time `json:"created_at" xorm:"DateTime created"`
	UpdatedAt time.Time `json:"updated_at" xorm:"DateTime updated"`
}

type User struct {
	Id       int    `json:"id" xorm:"pk autoincr not null"`
	Type     string `json:"type" xorm:"varchar(20) default('local')"`
	Username string `json:"username" xorm:"varchar(60) not null unique"`
	Nickname string `json:"nickname" xorm:"varchar(255)"`
	Email    string `json:"email" xorm:"varchar(255)"`
	Phone    string `json:"phone" xorm:"varchar(32)"`
	// Password  string    `json:"password"`
	PinCode        string     `json:"pin_code" xorm:"varchar(64)"`
	LimitTime      *time.Time `json:"limittime,omitempty" xorm:"Datetime limittime"` // 值为null时，前端不显示
	OtpSecret      string     `json:"otp_secret" xorm:"varchar(255)"`
	DisableOtp     bool       `json:"disable_otp" xorm:"Bool"` // 禁用otp
	Groups         []string   `json:"groups" xorm:"Text"`
	Mtu            int        `json:"mtu"`                                                         // 单独设置 mtu
	PolicyId       int        `json:"policy_id" xorm:"Int"`                                        // 个人策略 ID，0=使用组策略
	TrafficUsed    int64      `json:"traffic_used" xorm:"BigInt default 0"`                        // 本周期已用流量(字节)
	TrafficResetAt *time.Time `json:"traffic_reset_at,omitempty" xorm:"DateTime traffic_reset_at"` // 下次重置时间
	Status         int8       `json:"status" xorm:"Int"`                                           // 1正常
	ForcePwd       bool       `json:"change_pwd" xorm:"'change_pwd' Bool"`                         // 首次登录需改密
	SendEmail      bool       `json:"send_email" xorm:"Bool"`
	CreatedAt      time.Time  `json:"created_at" xorm:"DateTime created"`
	UpdatedAt      time.Time  `json:"updated_at" xorm:"DateTime updated"`
}

type UserActLog struct {
	Id              int       `json:"id" xorm:"pk autoincr not null"`
	Username        string    `json:"username" xorm:"varchar(60)"`
	Nickname        string    `json:"nickname" xorm:"-"`
	GroupName       string    `json:"group_name" xorm:"varchar(60)"`
	IpAddr          string    `json:"ip_addr" xorm:"varchar(64)"`
	RemoteAddr      string    `json:"remote_addr" xorm:"varchar(64)"`
	Os              uint8     `json:"os" xorm:"not null default 0 Int"`
	Client          uint8     `json:"client" xorm:"not null default 0 Int"`
	Version         string    `json:"version" xorm:"varchar(15)"`
	DeviceType      string    `json:"device_type" xorm:"varchar(128) not null default ''"`
	PlatformVersion string    `json:"platform_version" xorm:"varchar(128) not null default ''"`
	Status          uint8     `json:"status" xorm:"not null default 0 Int"`
	Info            string    `json:"info" xorm:"varchar(255) not null default ''"` // 详情
	CreatedAt       time.Time `json:"created_at" xorm:"DateTime created"`
}

type Setting struct {
	Id        int             `json:"id" xorm:"pk autoincr not null"`
	Name      string          `json:"name" xorm:"varchar(60) not null unique"`
	Data      json.RawMessage `json:"data" xorm:"Text"`
	UpdatedAt time.Time       `json:"updated_at" xorm:"DateTime updated"`
}

// AdminOpLog 管理员操作日志
type AdminOpLog struct {
	Id        int       `json:"id" xorm:"pk autoincr not null"`
	AdminUser string    `json:"admin_user" xorm:"varchar(60) not null"`            // 管理员用户名
	OpType    string    `json:"op_type" xorm:"varchar(60) not null"`               // 操作类型：用户管理/用户组管理/策略管理/证书管理/系统设置等
	OpTarget  string    `json:"op_target" xorm:"varchar(255) not null default ''"` // 操作目标（用户名/组名/策略名等）
	Detail    string    `json:"detail" xorm:"varchar(512) not null default ''"`    // 操作详情
	ClientIp  string    `json:"client_ip" xorm:"varchar(64) not null default ''"`  // 管理员操作时IP
	CreatedAt time.Time `json:"created_at" xorm:"DateTime created"`
}

type AccessAudit struct {
	Id          int       `json:"id" xorm:"pk autoincr not null"`
	Username    string    `json:"username" xorm:"varchar(60) not null"`
	Nickname    string    `json:"nickname" xorm:"-"`
	GroupName   string    `json:"group_name" xorm:"varchar(60) not null default ''"`
	Protocol    uint8     `json:"protocol" xorm:"Int not null"`
	Src         string    `json:"src" xorm:"varchar(60) not null"`
	SrcPort     uint16    `json:"src_port" xorm:"Int not null default 0"`
	Dst         string    `json:"dst" xorm:"varchar(60) not null"`
	DstPort     uint16    `json:"dst_port" xorm:"Int not null"`
	AccessProto uint8     `json:"access_proto" xorm:"Int default 0"`            // 访问协议
	Info        string    `json:"info" xorm:"varchar(255) not null default ''"` // 详情
	CreatedAt   time.Time `json:"created_at" xorm:"DateTime"`
}

// Policy 策略定义 — 可被用户组和用户引用的独立策略实体
type Policy struct {
	Id               int            `json:"id" xorm:"pk autoincr not null"`
	Name             string         `json:"name" xorm:"varchar(60) not null unique"`
	Note             string         `json:"note" xorm:"varchar(255)"`
	AllowLan         bool           `json:"allow_lan" xorm:"Bool"`
	ClientDns        []ValData      `json:"client_dns" xorm:"Text"`
	RouteInclude     []ValData      `json:"route_include" xorm:"Text"`
	RouteExclude     []ValData      `json:"route_exclude" xorm:"Text"`
	DsExcludeDomains string         `json:"ds_exclude_domains" xorm:"Text"`
	DsIncludeDomains string         `json:"ds_include_domains" xorm:"Text"`
	LinkAcl          []GroupLinkAcl `json:"link_acl" xorm:"Text"`
	Bandwidth        int            `json:"bandwidth" xorm:"Int"`             // 下行限速(Byte/s)
	BandwidthUp      int            `json:"bandwidth_up" xorm:"Int"`          // 上行限速(Byte/s)
	TrafficQuota     int64          `json:"traffic_quota" xorm:"BigInt"`      // 流量配额(字节), 0=不限
	TrafficReset     string         `json:"traffic_reset" xorm:"varchar(10)"` // daily/weekly/monthly, 空=不限
	// FakeDNS 配置
	EnableFakeDNS   bool   `json:"enable_fakedns" xorm:"Bool"`
	FakeDNSUpstream string `json:"fake_dns_upstream" xorm:"varchar(20)"`
	FakeDNSInclude  string `json:"fake_dns_include" xorm:"LongText"`
	FakeDNSExclude  string `json:"fake_dns_exclude" xorm:"LongText"`
	PreferIPv6      bool   `json:"prefer_ipv6" xorm:"Bool"` // DNS 层优先 IPv6：FakeDNS 命中域名时引导双栈应用走 v6（仅双栈开启生效）
	// 运行时计算字段（不持久化，加载后由 AddFakeDNSRules 预处理）
	FakeDNSIncludeSet map[string]struct{} `json:"-" xorm:"-"`
	FakeDNSExcludeSet map[string]struct{} `json:"-" xorm:"-"`
	Status            int8                `json:"status" xorm:"Int default 1"` // 1正常 0 禁用
	CreatedAt         time.Time           `json:"created_at" xorm:"DateTime created"`
	UpdatedAt         time.Time           `json:"updated_at" xorm:"DateTime updated"`
}

type StatsOnline struct {
	Id        int       `json:"id" xorm:"pk autoincr not null"`
	Num       int       `json:"num" xorm:"Int"`
	NumGroups string    `json:"num_groups" xorm:"varchar(500) not null"`
	CreatedAt time.Time `json:"created_at" xorm:"DateTime created index"`
}

type StatsNetwork struct {
	Id         int       `json:"id" xorm:"pk autoincr not null"`
	Up         uint64    `json:"up" xorm:"BigInt"`
	Down       uint64    `json:"down" xorm:"BigInt"`
	UpGroups   string    `json:"up_groups" xorm:"varchar(500) not null"`
	DownGroups string    `json:"down_groups" xorm:"varchar(500) not null"`
	CreatedAt  time.Time `json:"created_at" xorm:"DateTime created index"`
}

type StatsCpu struct {
	Id        int       `json:"id" xorm:"pk autoincr not null"`
	Percent   float64   `json:"percent" xorm:"Float"`
	CreatedAt time.Time `json:"created_at" xorm:"DateTime created index"`
}

type StatsMem struct {
	Id        int       `json:"id" xorm:"pk autoincr not null"`
	Percent   float64   `json:"percent" xorm:"Float"`
	CreatedAt time.Time `json:"created_at" xorm:"DateTime created index"`
}

type PasswordReset struct {
	Token           string `json:"token" xorm:"varchar(60) not null unique"`
	UserId          int    `json:"id" xorm:"not null"`
	ExpiresAt       int    `json:"expires_at" xorm:"not null"`
	LastRequestTime int    `json:"last_request_time" xorm:"int default 0"`
}

// 记录每个用户最近一次「整用户会话吊销」的时间戳（unix 秒）。
// 该时间戳之前签发的 WebVPN 会话一律视为已吊销（O(1) 整用户下线）。
// 持久化到 DB，使吊销在重启、多实例部署下依然有效
type WebVpnRevoke struct {
	Username  string `json:"username" xorm:"varchar(60) not null pk"`
	RevokedAt int64  `json:"revoked_at" xorm:"BigInt not null"`
}

// Provider 第三方认证配置，Pipeline 通过 name 引用。
type Provider struct {
	Id        int                                     `json:"id" xorm:"pk autoincr not null"`
	Name      string                                  `json:"name" xorm:"varchar(60) not null unique"`
	Type      string                                  `json:"type" xorm:"varchar(20) not null"`
	Config    security.EncryptedJSON[json.RawMessage] `json:"config" xorm:"Text not null"`
	Status    int8                                    `json:"status" xorm:"Int default 1"`
	CreatedAt time.Time                               `json:"created_at" xorm:"DateTime created"`
	UpdatedAt time.Time                               `json:"updated_at" xorm:"DateTime updated"`
}
