package dbdata

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wsczx/remlink/base"
	"xorm.io/xorm"
)

var (
	errWebVpnEmptyName    = errors.New("应用名称不能为空")
	errWebVpnInvalidName  = errors.New("应用名称仅允许小写字母、数字与中划线")
	errWebVpnEmptyBackend = errors.New("后端地址不能为空")
	errWebVpnInvalidId    = errors.New("无效的 ID")
)

// 校验应用名是否仅含小写字母、数字与中划线
// 用于作为 WebVPN 子域前缀（a.Name + "." + 域名）时的安全过滤
func webVpnAppNameValid(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return false
	}
	return true
}

// 应用配置的进程内缓存，避免每个请求都查库
// Set/Del 后通过 InvalidateWebVpnAppCache 主动失效；TTL 兜底防止遗漏
var webVpnAppCache = struct {
	mu     sync.RWMutex
	m      map[string]*WebVpnApp
	expire time.Time
}{m: make(map[string]*WebVpnApp)}

const webVpnAppCacheTTL = 60 * time.Second

// 主动失效缓存（配置变更后调用）
func InvalidateWebVpnAppCache() {
	webVpnAppCache.mu.Lock()
	webVpnAppCache.m = make(map[string]*WebVpnApp)
	webVpnAppCache.mu.Unlock()
}

// WebVPN 反向代理应用配置
// 通过子域名 *.WebVpnDomain 访问：子域名 = Name，反代到 Backend。
type WebVpnApp struct {
	Id          int       `json:"id" xorm:"pk autoincr not null"`
	Name        string    `json:"name" xorm:"varchar(60) not null unique"` // 子域名前缀，也是应用唯一标识
	Note        string    `json:"note" xorm:"varchar(255)"`
	Backend     string    `json:"backend" xorm:"varchar(255)"`      // 后端地址，如 https://10.0.0.5:8080
	Users       []string  `json:"users" xorm:"Text"`                // 授权用户名白名单；空=全部用户（受 Groups 约束）
	Groups      []string  `json:"groups" xorm:"Text"`               // 授权用户组白名单；空=不限组
	AllowPath   []string  `json:"allow_path" xorm:"Text"`           // 路径前缀白名单；空=全部路径
	IpAllowList []string  `json:"ip_allow_list" xorm:"Text"`        // 客户端来源 IP 白名单（CIDR 或单 IP）；空=不限制
	HostRewrite string    `json:"host_rewrite" xorm:"varchar(255)"` // 反代时改写到后端的 Host（覆盖默认的后端地址）；空=用后端地址
	SkipVerify  bool      `json:"skip_verify" xorm:"Bool"`          // 后端为自签/内网证书时跳过 TLS 校验
	Status      int8      `json:"status" xorm:"Int"`                // 1=启用
	CreatedAt   time.Time `json:"created_at" xorm:"DateTime created"`
	UpdatedAt   time.Time `json:"updated_at" xorm:"DateTime updated"`
}

// 分页查询所有 WebVPN 应用；name 非空时按子域名/名称前缀模糊过滤
func WebVpnAppList(pageSize, page int, name string) ([]WebVpnApp, int, error) {
	var datas []WebVpnApp
	name = strings.TrimSpace(name)
	if name == "" {
		count := CountAll(&WebVpnApp{})
		err := Find(&datas, pageSize, page)
		return datas, count, err
	}
	like := "%" + EscapeLike(name) + "%"
	count := FindWhereCount(&WebVpnApp{}, "name LIKE ? OR note LIKE ?", like, like)
	err := FindWhere(&datas, pageSize, page, "name LIKE ? OR note LIKE ?", like, like)
	return datas, count, err
}

// 新增或更新 WebVPN 应用
func SetWebVpnApp(a *WebVpnApp) error {
	if a.Name == "" {
		return errWebVpnEmptyName
	}
	// 应用名作为 WebVPN 子域名前缀使用（a.Name + "." + 域名），
	// 仅允许小写字母、数字与中划线，避免拼接出非预期主机或注入。
	if !webVpnAppNameValid(a.Name) {
		return errWebVpnInvalidName
	}
	if a.Backend == "" {
		return errWebVpnEmptyBackend
	}
	// 记录变更前的授权范围，用于权限收窄/调整时吊销相关用户会话
	var oldUsers, oldGroups []string
	if a.Id > 0 {
		var old WebVpnApp
		if err := One("Id", a.Id, &old); err == nil {
			oldUsers, oldGroups = old.Users, old.Groups
		}
	}
	a.UpdatedAt = time.Now()
	var err error
	if a.Id > 0 {
		// 更新
		err = Set(a)
	} else {
		// 新增：默认启用
		if a.Status == 0 {
			a.Status = 1
		}
		err = Add(a)
	}
	if err == nil {
		InvalidateWebVpnAppCache()
		// 权限白名单变更后，令新旧授权范围内的用户重新签发会话，使权限立即生效
		revokeAffectedWebVpnUsers(oldUsers, oldGroups, a.Users, a.Groups)
	}
	return err
}

// 删除 WebVPN 应用
func DelWebVpnApp(id int) error {
	if id < 1 {
		return errWebVpnInvalidId
	}
	// 删除前取出授权范围，删除后吊销相关用户会话
	var delUsers, delGroups []string
	var old WebVpnApp
	if err := One("Id", id, &old); err == nil {
		delUsers, delGroups = old.Users, old.Groups
	}
	err := Del(&WebVpnApp{Id: id})
	if err == nil {
		InvalidateWebVpnAppCache()
		revokeAffectedWebVpnUsers(delUsers, delGroups, nil, nil)
	}
	return err
}

// 权限白名单（用户/组）变更后，仅吊销「被移除授权」的用户 WebVPN 会话，
// 使已签发的会话（token 内固化的 webvpn_groups）立即失效，下次访问须重新授权。
func revokeAffectedWebVpnUsers(oldUsers, oldGroups, newUsers, newGroups []string) {
	keepUsers := make(map[string]bool, len(newUsers))
	for _, u := range newUsers {
		if u != "" {
			keepUsers[u] = true
		}
	}
	keepGroups := make(map[string]bool, len(newGroups))
	for _, g := range newGroups {
		if g != "" {
			keepGroups[g] = true
		}
	}

	seen := make(map[string]bool)
	var toKick []string
	add := func(u string) {
		if u != "" && !seen[u] {
			seen[u] = true
			toKick = append(toKick, u)
		}
	}
	// 仅吊销：旧用户白名单中、已不在新白名单里的
	for _, u := range oldUsers {
		if !keepUsers[u] {
			add(u)
		}
	}
	// 仅吊销：旧组白名单中、且不在新组/新用户白名单里的成员
	for _, g := range oldGroups {
		if g == "" || keepGroups[g] {
			continue
		}
		for _, u := range UsernamesOfGroup(g) {
			if !keepUsers[u] {
				add(u)
			}
		}
	}
	if len(toKick) > 0 {
		WebVpnRevokeUsers(toKick)
	}
}

// 按子域名前缀（Name）查找应用，带进程内缓存
func GetWebVpnAppByName(name string) (*WebVpnApp, error) {
	webVpnAppCache.mu.RLock()
	hit := webVpnAppCache.m[name]
	fresh := time.Now().Before(webVpnAppCache.expire)
	webVpnAppCache.mu.RUnlock()
	if hit != nil && fresh {
		return hit, nil
	}

	a := &WebVpnApp{}
	if err := One("Name", name, a); err != nil {
		if CheckErrNotFound(err) {
			// 未命中也缓存空标记，避免穿透；用独立负缓存 key
			webVpnAppCache.mu.Lock()
			webVpnAppCache.m[name] = nil
			webVpnAppCache.expire = time.Now().Add(webVpnAppCacheTTL)
			webVpnAppCache.mu.Unlock()
		}
		return nil, err
	}

	webVpnAppCache.mu.Lock()
	webVpnAppCache.m[name] = a
	webVpnAppCache.expire = time.Now().Add(webVpnAppCacheTTL)
	webVpnAppCache.mu.Unlock()
	return a, nil
}

// 空白名单=全部放行；用户维度判断与 handler 层 webVpnAuthorized 保持一致。
func WebVpnUserAllowed(a *WebVpnApp, user *User) bool {
	if len(a.Users) > 0 {
		if !contains(a.Users, user.Username) {
			return false
		}
	}
	if len(a.Groups) > 0 {
		hit := false
		for _, g := range user.Groups {
			if contains(a.Groups, g) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

func contains(slice []string, target string) bool {
	return slices.Contains(slice, target)
}

// WebVPN 访问审计记录（每次代理请求落一条，异步批量写入）。
type WebVpnAudit struct {
	Id         int64     `json:"id" xorm:"pk autoincr not null"`
	Username   string    `json:"username" xorm:"varchar(60) not null"`
	GroupName  string    `json:"group_name" xorm:"varchar(60) not null default ''"`
	AppName    string    `json:"app_name" xorm:"varchar(60) not null"` // 子域名前缀（应用标识）
	Host       string    `json:"host" xorm:"varchar(255) not null"`    // 原始 Host 请求头
	Method     string    `json:"method" xorm:"varchar(16) not null"`   // GET/POST...
	Path       string    `json:"path" xorm:"varchar(1024) not null"`   // 请求路径（不含 query，避免敏感信息入库）
	StatusCode int       `json:"status_code" xorm:"Int not null default 0"`
	BytesSent  int64     `json:"bytes_sent" xorm:"BigInt not null default 0"`
	BytesRecv  int64     `json:"bytes_recv" xorm:"BigInt not null default 0"`
	ClientIP   string    `json:"client_ip" xorm:"varchar(60) not null"`
	DurationMs int64     `json:"duration_ms" xorm:"BigInt not null default 0"`
	RiskLevel  int8      `json:"risk_level" xorm:"Int not null default 0"` // 0=正常 1=可疑 2=高危
	CreatedAt  time.Time `json:"created_at" xorm:"DateTime created"`
}

// 显式指定审计表名
func (WebVpnAudit) TableName() string  { return "webvpn_audit" }
func (WebVpnApp) TableName() string    { return "webvpn_app" }
func (WebVpnRevoke) TableName() string { return "webvpn_revoke" }

// 分页查询（支持按用户名/应用名/时间范围过滤）
func WebVpnAuditList(pageSize, page int, search WebVpnAuditSearch) ([]WebVpnAudit, int, error) {
	var datas []WebVpnAudit
	buildCond := func(s *xorm.Session) {
		if search.Username != "" {
			s.And("username = ?", search.Username)
		}
		if search.AppName != "" {
			s.And("app_name = ?", search.AppName)
		}
		if search.Method != "" {
			s.And("method = ?", search.Method)
		}
		if len(search.Date) == 2 && search.Date[0] != "" {
			s.And("created_at BETWEEN ? AND ?", search.Date[0], search.Date[1])
		}
	}
	countSession := xdb.Where("1=1")
	buildCond(countSession)
	count, err := countSession.Count(&WebVpnAudit{})
	if err != nil {
		return nil, 0, err
	}
	listSession := xdb.Where("1=1")
	buildCond(listSession)
	if err := listSession.OrderBy("id desc").Limit(pageSize, (page-1)*pageSize).Find(&datas); err != nil {
		return nil, 0, err
	}
	return datas, int(count), nil
}

// 审计查询条件
type WebVpnAuditSearch struct {
	Username string   `json:"username"`
	AppName  string   `json:"app_name"`
	Method   string   `json:"method"`
	Date     []string `json:"date"`
}

// 导出查询：按条件返回审计记录。
// 审计表可能极大，导出设硬上限防止一次性拉爆内存（命中条数超过上限时截断）
const webVpnAuditExportLimit = 100000

func WebVpnAuditExportList(search WebVpnAuditSearch) ([]WebVpnAudit, error) {
	var datas []WebVpnAudit
	session := xdb.Where("1=1")
	if search.Username != "" {
		session.And("username = ?", search.Username)
	}
	if search.AppName != "" {
		session.And("app_name = ?", search.AppName)
	}
	if search.Method != "" {
		session.And("method = ?", search.Method)
	}
	if len(search.Date) == 2 && search.Date[0] != "" {
		session.And("created_at BETWEEN ? AND ?", search.Date[0], search.Date[1])
	}
	if err := session.OrderBy("id desc").Limit(webVpnAuditExportLimit).Find(&datas); err != nil {
		return nil, err
	}
	if len(datas) >= webVpnAuditExportLimit {
		base.Warn("WebVPN 审计导出达到上限，仅返回前 ", webVpnAuditExportLimit, " 条，请缩小时间/筛选范围")
	}
	return datas, nil
}

// 返回导出查询命中总数（用于上限校验）
func WebVpnAuditExportCount(search WebVpnAuditSearch) (int64, error) {
	session := xdb.Where("1=1")
	if search.Username != "" {
		session.And("username = ?", search.Username)
	}
	if search.AppName != "" {
		session.And("app_name = ?", search.AppName)
	}
	if search.Method != "" {
		session.And("method = ?", search.Method)
	}
	if len(search.Date) == 2 && search.Date[0] != "" {
		session.And("created_at BETWEEN ? AND ?", search.Date[0], search.Date[1])
	}
	return session.Count(&WebVpnAudit{})
}

// 批量写入审计记录
func AddBatchWebVpnAudit(datas []WebVpnAudit) error {
	if len(datas) == 0 {
		return nil
	}
	_, err := xdb.Insert(&datas)
	return err
}

// WebVPN 会话整用户踢出阈值：username -> 吊销时间戳（unix 秒）。
// 该时间戳之前签发的 WebVPN 会话一律视为已吊销，实现 O(1) 整用户下线。
var (
	webVpnRevokeBeforeMu sync.Mutex
	webVpnRevokeBefore   = map[string]int64{}
)

// 在服务启动时将 DB 中的吊销记录加载到内存缓存，保证重启后旧 token 仍按上次吊销阈值失效
func LoadWebVpnRevoke() {
	var rows []WebVpnRevoke
	if err := xdb.Find(&rows); err != nil {
		base.Error("加载 WebVPN 吊销记录失败:", err)
		return
	}
	webVpnRevokeBeforeMu.Lock()
	for _, r := range rows {
		webVpnRevokeBefore[r.Username] = r.RevokedAt
	}
	webVpnRevokeBeforeMu.Unlock()
}

// 吊销指定用户的全部 WebVPN 会话（整用户下线）。
// 通过抬高吊销阈值实现：此后该用户签名时间早于阈值的会话都将被拒绝。
func WebVpnRevokeUser(username string) {
	if username == "" {
		return
	}
	ts := time.Now().Unix()

	// 写内存缓存
	webVpnRevokeBeforeMu.Lock()
	webVpnRevokeBefore[username] = ts
	webVpnRevokeBeforeMu.Unlock()

	// 持久化到 DB
	rec := &WebVpnRevoke{Username: username, RevokedAt: ts}
	var probe WebVpnRevoke
	var err error
	if e := One("Username", username, &probe); e == nil {
		err = Update("Username", username, rec)
	} else {
		err = Add(rec)
	}
	if err != nil {
		base.Error("持久化 WebVPN 吊销记录失败:", err)
	}
}

// 返回指定用户的吊销阈值（0 表示未吊销）。
// 优先读内存缓存；未命中时回查 DB 并回填缓存。
func WebVpnRevokeBeforeOf(username string) int64 {
	webVpnRevokeBeforeMu.Lock()
	ts, ok := webVpnRevokeBefore[username]
	webVpnRevokeBeforeMu.Unlock()
	if ok {
		return ts
	}
	rec := &WebVpnRevoke{}
	if err := One("Username", username, rec); err != nil {
		return 0
	}
	// 回查命中，回填内存缓存
	webVpnRevokeBeforeMu.Lock()
	webVpnRevokeBefore[username] = rec.RevokedAt
	webVpnRevokeBeforeMu.Unlock()
	return rec.RevokedAt
}

// 清空全部吊销阈值（取消整用户下线状态）。
// 主要用于测试隔离与运维排障，生产环境应谨慎使用。
func WebVpnRevokeReset() {
	webVpnRevokeBeforeMu.Lock()
	webVpnRevokeBefore = map[string]int64{}
	webVpnRevokeBeforeMu.Unlock()
}

// 批量吊销一批用户的 WebVPN 会话（权限变更后让已签发会话立即失效）。
// WebVPN 未启用（WebVpnDomain 为空）时直接跳过，避免无意义地逐用户写库。
func WebVpnRevokeUsers(usernames []string) {
	if len(usernames) == 0 {
		return
	}
	if base.GetCfg().WebVpnDomain == "" {
		return
	}
	for _, u := range usernames {
		WebVpnRevokeUser(u)
	}
}

// 吊销指定用户组全部成员的 WebVPN 会话。
// 改/删用户组后，成员 token 内固化的 webvpn_groups 已过期，须令其重新签发。
func WebVpnRevokeGroupMembers(groupNames []string) {
	if len(groupNames) == 0 {
		return
	}
	// WebVPN 未启用时无需吊销，直接返回。
	if base.GetCfg().WebVpnDomain == "" {
		return
	}
	want := make(map[string]bool, len(groupNames))
	for _, g := range groupNames {
		want[g] = true
	}
	// 精确组名匹配，避免误吊销
	var users []User
	for _, g := range groupNames {
		like := `%"` + EscapeLike(g) + `"%`
		var part []User
		if err := FindWhere(&part, 0, 0, "groups LIKE ?", like); err != nil {
			base.Error("WebVPN 查询组成员失败:", err)
			continue
		}
		users = append(users, part...)
	}
	kicked := make(map[string]bool)
	for _, u := range users {
		for _, g := range u.Groups {
			if want[g] {
				if !kicked[u.Username] {
					WebVpnRevokeUser(u.Username)
					kicked[u.Username] = true
				}
				break
			}
		}
	}
}

// 返回某用户组下的全部用户名（内存过滤，因 Groups 以 Text 序列化存储）。
func UsernamesOfGroup(groupName string) []string {
	if groupName == "" {
		return nil
	}
	var users []User
	if err := Find(&users, -1, 0); err != nil {
		base.Error("查询用户组成员失败:", err)
		return nil
	}
	names := make([]string, 0)
	for _, u := range users {
		if slices.Contains(u.Groups, groupName) {
			names = append(names, u.Username)
		}
	}
	return names
}
