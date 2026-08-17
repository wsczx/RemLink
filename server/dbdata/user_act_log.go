package dbdata

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"xorm.io/xorm"
)

const (
	UserAuthFail       = 0  // 认证失败
	UserAuthSuccess    = 1  // 认证成功
	UserConnected      = 2  // 连线成功
	UserLogout         = 3  // 用户登出
	UserLogoutLose     = 0  // 用户掉线
	UserLogoutBanner   = 1  // 用户banner弹窗取消
	UserLogoutClient   = 2  // 用户主动登出
	UserLogoutTimeout  = 3  // 用户超时登出
	UserLogoutAdmin    = 4  // 账号被管理员踢下线
	UserLogoutExpire   = 5  // 账号过期被踢下线
	UserIdleTimeout    = 6  // 用户空闲链接超时
	UserLogoutOneAdmin = 7  // 账号被管理员一键下线
	UserLogoutQuota    = 8  // 流量配额超限被踢下线
	UserLogoutReconn   = 9  // 客户端自动重连，旧连接已关闭
	UserLogoutLink     = 10 // 链路读写异常/超时断开
	UserLogoutTunErr   = 11 // 虚拟网卡建立失败

	UserPortalClient = 4 // 门户/浏览器登录
	UserPortalOs     = 6 // 门户登录操作系统
	UserAdminClient  = 5 // 管理后台登录
)

type UserActLogProcess struct {
	Pool        *utils.WorkerPool
	StatusOps   []string
	OsOps       []string
	ClientOps   []string
	InfoOps     []string
	failLimiter *base.WarnLimiter
}

var (
	UserActLogIns = &UserActLogProcess{
		Pool: utils.NewWorkerPool(1, 100),
		StatusOps: []string{ // 操作类型
			UserAuthFail:    "认证失败",
			UserAuthSuccess: "认证成功",
			UserConnected:   "连接成功",
			UserLogout:      "用户登出",
		},
		OsOps: []string{ // 操作系统
			0: "Unknown",
			1: "Windows",
			2: "macOS",
			3: "Linux",
			4: "Android",
			5: "iOS",
			6: "浏览器",
		},
		ClientOps: []string{ // 客户端
			0: "Unknown",
			1: "AnyConnect",
			2: "OpenConnect",
			3: "RemLink",
			4: "门户",
			5: "后台",
		},
		InfoOps: []string{ // 信息
			UserLogoutLose:     "用户掉线",
			UserLogoutBanner:   "用户取消弹窗/客户端发起的logout",
			UserLogoutClient:   "用户/客户端主动断开",
			UserLogoutTimeout:  "Session过期被踢下线",
			UserLogoutAdmin:    "账号被管理员踢下线",
			UserLogoutExpire:   "账号过期被踢下线",
			UserIdleTimeout:    "用户空闲链接超时",
			UserLogoutOneAdmin: "账号被管理员一键下线",
			UserLogoutQuota:    "流量配额超限被踢下线",
			UserLogoutReconn:   "客户端自动重连，旧连接已关闭",
			UserLogoutLink:     "链路异常断开",
			UserLogoutTunErr:   "虚拟网卡建立失败",
		},
		failLimiter: base.NewWarnLimiter(time.Minute),
	}
)

// 异步写入用户操作日志。isPortal 为 true 时按门户（浏览器）解析客户端与系统。
func (ua *UserActLogProcess) Add(u UserActLog, userAgent string, isPortal ...bool) {
	// 账号/IP 已被锁定后的重复登录失败没有新的审计价值，按 username|ip 每分钟限频一条，
	// 避免前端自动重试刷屏撑爆数据库。
	if u.Status == UserAuthFail && u.IsLockedFail {
		host, _, _ := net.SplitHostPort(u.RemoteAddr)
		if host == "" {
			host = u.RemoteAddr
		}
		key := u.Username + "|" + host
		if !ua.failLimiter.Allow(key, time.Now()) {
			return
		}
	}
	os_idx, client_idx, ver := ua.ParseUserAgent(userAgent)
	if len(isPortal) > 0 && isPortal[0] {
		u.Os = UserPortalOs
		u.Client = UserPortalClient
		u.Version = ""
		browserName, browserVer := parseBrowserUA(userAgent)
		if browserName != "" {
			u.DeviceType = browserName
			u.PlatformVersion = browserVer
		}
	} else {
		// 调用方已指定客户端类型（如后台管理员）时，仍需按 User-Agent 识别操作系统。
		if u.Os == 0 {
			u.Os = os_idx
		}
		if u.Client == 0 {
			u.Client = client_idx
			u.Version = ver
		}
		// 后台/门户等浏览器场景，补充浏览器信息
		if (u.Client == UserPortalClient || u.Client == UserAdminClient) && u.DeviceType == "" {
			browserName, browserVer := parseBrowserUA(userAgent)
			if browserName != "" {
				u.DeviceType = browserName
				u.PlatformVersion = browserVer
			}
		}
		// 调用方已提供设备信息时直接保留（如 WebAuth 失败路径传入了 DeviceType/PlatformVer），无需覆盖
	}
	u.RemoteAddr, _, _ = net.SplitHostPort(u.RemoteAddr)
	// 去除 Info 中的用户名前缀和分隔符
	infoSlice := strings.Split(u.Info, " ")
	infoLen := len(infoSlice)
	if infoLen > 1 {
		if u.Username == infoSlice[0] {
			u.Info = strings.Join(infoSlice[1:], " ")
		}
		if infoLen > 2 && infoSlice[1] == "-" {
			u.Info = u.Info[2:]
		}
	}
	// 截断超长字段
	u.Version = substr(u.Version, 0, 15)
	u.DeviceType = substr(u.DeviceType, 0, 128)
	u.PlatformVersion = substr(u.PlatformVersion, 0, 128)
	u.Info = substr(u.Info, 0, 255)

	UserActLogIns.Pool.JobQueue <- func() {
		err := Add(u)
		if err != nil {
			base.Error("Add UserActLog error: ", err)
		}
	}
}

// 返回带 tag 颜色的操作类型列表（供前端下拉选择）
func (ua *UserActLogProcess) GetStatusOpsWithTag() any {
	type StatusTag struct {
		Key   int    `json:"key"`
		Value string `json:"value"`
		Tag   string `json:"tag"`
	}
	var res []StatusTag
	for k, v := range ua.StatusOps {
		tag := "info"
		switch k {
		case UserAuthFail:
			tag = "danger"
		case UserAuthSuccess:
			tag = "success"
		case UserConnected:
			tag = ""
		}
		res = append(res, StatusTag{k, v, tag})
	}
	return res
}

func (ua *UserActLogProcess) GetInfoOpsById(id uint8) string {
	if int(id) >= len(ua.InfoOps) {
		return "未知的信息类型"
	}
	return ua.InfoOps[id]
}

var ieVerRe = regexp.MustCompile(`trident/.*rv:([0-9.]+)`)

// 从常见浏览器 UA 中提取浏览器名称和版本
func parseBrowserUA(userAgent string) (name, version string) {
	ua := strings.ToLower(userAgent)
	if ua == "" {
		return "", ""
	}

	if m := regexp.MustCompile(`edg/([0-9.]+)`).FindStringSubmatch(ua); len(m) > 1 {
		return "Edge", m[1]
	}
	// Opera
	if m := regexp.MustCompile(`opr/([0-9.]+)`).FindStringSubmatch(ua); len(m) > 1 {
		return "Opera", m[1]
	}
	// Samsung Browser
	if m := regexp.MustCompile(`samsungbrowser/([0-9.]+)`).FindStringSubmatch(ua); len(m) > 1 {
		return "Samsung Browser", m[1]
	}
	// Chrome
	if m := regexp.MustCompile(`chrome/([0-9.]+)`).FindStringSubmatch(ua); len(m) > 1 {
		return "Chrome", m[1]
	}
	// Firefox
	if m := regexp.MustCompile(`firefox/([0-9.]+)`).FindStringSubmatch(ua); len(m) > 1 {
		return "Firefox", m[1]
	}
	// Safari（Chrome/Firefox 已排除，只剩 Safari）
	if m := regexp.MustCompile(`version/([0-9.]+).+safari/`).FindStringSubmatch(ua); len(m) > 1 {
		return "Safari", m[1]
	}
	// IE
	if m := regexp.MustCompile(`msie ([0-9.]+)`).FindStringSubmatch(ua); len(m) > 1 {
		return "IE", m[1]
	}
	if m := ieVerRe.FindStringSubmatch(ua); len(m) > 1 {
		return "IE", m[1]
	}

	return "", ""
}

// 从 User-Agent 解析操作系统、客户端类型和版本
func (ua *UserActLogProcess) ParseUserAgent(userAgent string) (os_idx, client_idx uint8, ver string) {
	userAgent = strings.ToLower(userAgent)
	if len(userAgent) == 0 {
		return 0, 0, ""
	}
	os_idx = 0
	if strings.Contains(userAgent, "windows") {
		os_idx = 1
	} else if strings.Contains(userAgent, "mac os") || strings.Contains(userAgent, "darwin_i386") || strings.Contains(userAgent, "darwin_amd64") || strings.Contains(userAgent, "darwin_arm64") {
		os_idx = 2
	} else if strings.Contains(userAgent, "darwin_arm") || strings.Contains(userAgent, "apple") {
		os_idx = 5
	} else if strings.Contains(userAgent, "android") {
		os_idx = 4
	} else if strings.Contains(userAgent, "linux") {
		os_idx = 3
	}
	client_idx = 0
	if strings.Contains(userAgent, "anyconnect") {
		client_idx = 1
	} else if strings.Contains(userAgent, "openconnect") {
		client_idx = 2
	} else if strings.Contains(userAgent, "anylink") {
		client_idx = 3
	}
	uaSlice := strings.Split(userAgent, " ")
	ver = uaSlice[len(uaSlice)-1]
	if ver[0] == 'v' {
		ver = ver[1:]
	}
	if !regexp.MustCompile(`^(\d+\.?)+$`).MatchString(ver) {
		ver = ""
	}
	return
}

// 清除用户操作日志
func (ua *UserActLogProcess) ClearUserActLog(ts string) (int64, error) {
	affected, err := xdb.Where("created_at < ?", ts).Delete(&UserActLog{})
	return affected, err
}

// 后台筛选用户操作日志
func (ua *UserActLogProcess) GetSession(values url.Values) *xorm.Session {
	session := xdb.Where("1=1")
	if values.Get("username") != "" {
		session.And("username = ?", values.Get("username"))
	}
	if values.Get("sdate") != "" {
		session.And("created_at >= ?", values.Get("sdate")+" 00:00:00")
	}
	if values.Get("edate") != "" {
		session.And("created_at <= ?", values.Get("edate")+" 23:59:59")
	}
	if v := values.Get("status"); v != "" {
		n, _ := strconv.ParseUint(v, 10, 8)
		session.And("status = ?", uint8(n)-1)
	}
	if v := values.Get("os"); v != "" {
		n, _ := strconv.ParseUint(v, 10, 8)
		session.And("os = ?", uint8(n)-1)
	}
	if v := values.Get("client"); v != "" {
		n, _ := strconv.ParseUint(v, 10, 8)
		session.And("client = ?", uint8(n)-1)
	}
	if values.Get("sort") == "1" {
		session.OrderBy("id desc")
	} else {
		session.OrderBy("id asc")
	}
	return session
}

// 截取字符串
func substr(s string, pos, length int) string {
	runes := []rune(s)
	l := min(pos+length, len(runes))
	return string(runes[pos:l])
}
