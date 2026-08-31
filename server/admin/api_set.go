package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	"github.com/vishvananda/netlink"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/mask"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
	"xorm.io/xorm"
)

// 进程启动时间，用于计算运行时长
var serverStartTime = time.Now()

func SetHome(w http.ResponseWriter, r *http.Request) {
	data := make(map[string]any)

	sess := sessdata.GetOnlineSess("", "", false)

	data["counts"] = map[string]int{
		"online": len(sess),
		"user":   dbdata.CountAll(&dbdata.User{}),
		"group":  dbdata.CountAll(&dbdata.Group{}),
		"ip_map": dbdata.CountAll(&dbdata.IpMap{}),
	}

	data["show_fakedns"] = base.GetCfg().ShowFakeDNS

	RespSucess(w, data)
}

func SetSystem(w http.ResponseWriter, r *http.Request) {
	data := make(map[string]any)

	m, _ := mem.VirtualMemory()
	data["mem"] = map[string]any{
		"total":   utils.HumanByte(m.Total),
		"free":    utils.HumanByte(m.Free),
		"percent": decimal(m.UsedPercent),
	}

	d, _ := disk.Usage("/")
	data["disk"] = map[string]any{
		"total":   utils.HumanByte(d.Total),
		"free":    utils.HumanByte(d.Free),
		"percent": decimal(d.UsedPercent),
	}

	cc, _ := cpu.Counts(true)
	c, _ := cpu.Info()
	var ci cpu.InfoStat
	if len(c) > 0 {
		ci = c[0]
	}
	cpuUsedPercent, _ := cpu.Percent(0, false)
	var cup float64
	if len(cpuUsedPercent) > 0 {
		cup = cpuUsedPercent[0]
	}
	if cup == 0 {
		cup = 1
	}
	data["cpu"] = map[string]any{
		"core":      cc,
		"modelName": ci.ModelName,
		"ghz":       fmt.Sprintf("%.2f GHz", ci.Mhz/1000),
		"percent":   decimal(cup),
	}

	hi, _ := host.Info()
	l, _ := load.Avg()
	data["sys"] = map[string]any{
		"goOs":         runtime.GOOS,
		"goArch":       runtime.GOARCH,
		"goVersion":    runtime.Version(),
		"goroutine":    runtime.NumGoroutine(),
		"appVersion":   "v" + base.APP_VER,
		"appCommitId":  base.CommitId,
		"appBuildDate": base.BuildDate,

		"hostname": hi.Hostname,
		"platform": fmt.Sprintf("%v %v %v", hi.Platform, hi.PlatformFamily, hi.PlatformVersion),
		"kernel":   hi.KernelVersion,

		"load": fmt.Sprint(l.Load1, l.Load5, l.Load15),

		"uptime": int64(time.Since(serverStartTime).Seconds()),
	}

	RespSucess(w, data)
}

func SetSoft(w http.ResponseWriter, r *http.Request) {
	data := base.GetConfigMeta()
	// 为 ipv4_master 注入动态物理网卡选项
	for i, item := range data {
		if item["name"] == "master_dev" {
			ifaces := utils.GetPhysicalInterfaces()
			options := make(map[string]string, len(ifaces))
			for _, iface := range ifaces {
				options[iface] = iface
			}
			if len(options) > 0 {
				item["options"] = options
			}
			data[i] = item
			break
		}
	}
	RespSucess(w, data)
}

type setSoftEditReq struct {
	Name string `json:"name"`
	Data any    `json:"data"`
}

type setProfileReq struct {
	Content string `json:"content"`
}

func SetSoftStatus(w http.ResponseWriter, r *http.Request) {
	RespSucess(w, map[string]any{
		"warnings": base.GetSystemWarnings(),
	})
}

func SetSoftEdit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	req := &setSoftEditReq{}
	err = json.Unmarshal(body, req)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if req.Name == "" {
		RespError(w, RespParamErr, "配置名不能为空")
		return
	}

	// 数据库类型和连接字符串不允许在此接口修改，请使用 /set/db/switch
	if req.Name == "db_type" || req.Name == "db_source" {
		RespError(w, RespParamErr, "数据库配置请通过数据库切换向导修改")
		return
	}

	if req.Name == "admin_pass" {
		RespError(w, RespParamErr, "管理员密码请通过安全设置页面修改")
		return
	}
	// 敏感字段：前端未修改时传占位符，拒绝并提示
	if base.IsFieldSensitive(req.Name) && fmt.Sprint(req.Data) == mask.Placeholder {
		RespError(w, RespParamErr, "敏感配置未修改")
		return
	}

	// FakeDNS 只能通过命令行开启，前端仅允许关闭
	if req.Name == "show_fakedns" {
		if val, ok := req.Data.(bool); ok && val {
			RespError(w, RespParamErr, "FakeDNS 功能只能通过命令行开启（remlink --enable-fakedns）")
			return
		}
	}

	// 校验监听地址格式，防止错误配置导致服务重启后不可达
	if err := validateAddressField(req.Name, fmt.Sprint(req.Data)); err != nil {
		RespError(w, RespParamErr, err)
		return
	}

	restart, err := base.SetConfigField(req.Name, req.Data)
	if err != nil {
		RespError(w, RespParamErr, err)
		return
	}
	// 热加载：IP 白名单/黑名单变更时重新解析
	switch req.Name {
	case "ip_whitelist":
		auth.GetLockManager().LoadIPList(auth.IPWhiteList, base.GetCfg().IPWhiteList)
	case "ip_blacklist":
		auth.GetLockManager().LoadIPList(auth.IPBlackList, base.GetCfg().IPBlackList)
	case "show_sql":
		dbdata.GetXdb().ShowSQL(base.GetCfg().ShowSQL)
	}

	err = dbdata.SettingSaveServerConfig()
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("系统设置", req.Name, "修改了系统配置: "+req.Name+"="+formatConfigValue(req.Name, req.Data), r.RemoteAddr)
	RespSucess(w, map[string]any{
		"restart": restart,
	})
}

// 批量保存 IPv4 网络配置（网段/网关/起始/结束一起提交）
func SetIPv4Config(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ipv4CIDR    string `json:"ipv4_cidr"`
		Ipv4Gateway string `json:"ipv4_gateway"`
		Ipv4Start   string `json:"ipv4_start"`
		Ipv4End     string `json:"ipv4_end"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespError(w, RespParamErr, "参数解析失败")
		return
	}

	if err := validateIPv4ConfigAll(req.Ipv4CIDR, req.Ipv4Gateway, req.Ipv4Start, req.Ipv4End); err != nil {
		RespError(w, RespParamErr, err)
		return
	}

	fields := map[string]string{
		"ipv4_cidr":    req.Ipv4CIDR,
		"ipv4_gateway": req.Ipv4Gateway,
		"ipv4_start":   req.Ipv4Start,
		"ipv4_end":     req.Ipv4End,
	}
	var restart bool
	for name, value := range fields {
		r, err := base.SetConfigField(name, value)
		if err != nil {
			RespError(w, RespInternalErr, fmt.Errorf("保存 %s 失败: %w", name, err))
			return
		}
		if r {
			restart = true
		}
	}

	if err := dbdata.SettingSaveServerConfig(); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	dbdata.AdminLog("系统设置", "ipv4_config", "修改了 IPv4 网络配置", r.RemoteAddr)
	RespSucess(w, map[string]any{
		"restart": restart,
	})
}

func SetProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := dbdata.SettingGetProfile()
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	hash, err := dbdata.GetProfileHash()
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	RespSucess(w, map[string]any{
		"content": profile.Content,
		"hash":    hash,
	})
}

func SetProfileEdit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	req := &setProfileReq{}
	err = json.Unmarshal(body, req)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if req.Content == "" {
		RespError(w, RespParamErr, "profile内容不能为空")
		return
	}
	err = dbdata.SettingSetProfile(req.Content)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	hash, err := dbdata.GetProfileHash()
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("系统设置", "Profile", "修改了Profile配置", r.RemoteAddr)
	RespSucess(w, map[string]any{
		"hash": hash,
	})
}

var (
	restarting bool
	restartMux sync.Mutex
)

func SetRestart(w http.ResponseWriter, r *http.Request) {
	restartMux.Lock()
	if restarting {
		restartMux.Unlock()
		RespError(w, RespInternalErr, "重启进行中，请稍后再试")
		return
	}
	restarting = true
	restartMux.Unlock()

	dbdata.AdminLog("系统设置", "系统重启", "触发了系统重启", r.RemoteAddr)
	RespSucess(w, map[string]any{
		"message": "restart scheduled",
	})
	go func() {
		time.Sleep(time.Second)
		// 重启前清理所有后端防火墙规则并关闭数据库
		sessdata.CleanupAllNatRules()
		if sessdata.GlobalFakeDNSManager != nil {
			sessdata.GlobalFakeDNSManager.Stop()
		}
		// 清理残留的 macvtap 网卡，防止重启后撞名
		destroyLvtap()
		if err := dbdata.Stop(); err != nil {
			base.Warn("db stop before restart:", err)
		}
		if err := base.RestartProcess(); err != nil {
			base.Error("restart err:", err)
			restartMux.Lock()
			restarting = false
			restartMux.Unlock()
		}
	}()
}

// 返回各表行数，供前端展示并决定排除哪些大表
func SetDbTableSizes(w http.ResponseWriter, r *http.Request) {
	sizes := dbdata.GetBackupManager().GetTableSizes()
	RespSucess(w, sizes)
}

type setDbBackupReq struct {
	Type          string   `json:"type"`           // "config" | "full"
	IncludeTables []string `json:"include_tables"` // 全量备份时包含的表（已排除用户未选中的）
}

// 创建备份
func SetDbBackup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	req := &setDbBackupReq{}
	if err := json.Unmarshal(body, req); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if req.Type != "config" && req.Type != "full" {
		RespError(w, RespParamErr, "type 必须为 config 或 full")
		return
	}

	if req.Type == "full" {
		validNames := dbdata.GetBackupManager().AllTableNames()
		validSet := make(map[string]bool, len(validNames))
		for _, n := range validNames {
			validSet[n] = true
		}
		for _, t := range req.IncludeTables {
			if !validSet[t] {
				RespError(w, RespParamErr, "未知表名:", t)
				return
			}
		}
	}

	filename, err := dbdata.GetBackupManager().Exporter().Create(req.Type, req.IncludeTables)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("系统设置", filename, "创建了数据库备份", r.RemoteAddr)
	RespSucess(w, map[string]string{"filename": filename})
}

// 列出备份文件
func SetDbBackups(w http.ResponseWriter, r *http.Request) {
	list, err := dbdata.GetBackupManager().ListBackups()
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if list == nil {
		list = []dbdata.BackupFileInfo{}
	}
	RespSucess(w, list)
}

type setDbRestoreReq struct {
	File string `json:"file"`
}

// 从备份文件还原
func SetDbRestore(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	req := &setDbRestoreReq{}
	if err := json.Unmarshal(body, req); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if req.File == "" {
		RespError(w, RespParamErr, "备份文件名不能为空")
		return
	}

	if err := dbdata.GetBackupManager().Restore(req.File); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("系统设置", req.File, "从备份还原了数据库", r.RemoteAddr)
	RespSucess(w, map[string]any{
		"message":       "还原成功",
		"needs_restart": true,
	})
}

type setDbBackupDeleteReq struct {
	File string `json:"file"`
}

// 删除备份文件
func SetDbBackupDelete(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	req := &setDbBackupDeleteReq{}
	if err := json.Unmarshal(body, req); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if req.File == "" {
		RespError(w, RespParamErr, "备份文件名不能为空")
		return
	}

	if err := dbdata.GetBackupManager().DeleteBackup(req.File); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("系统设置", req.File, "删除了备份文件", r.RemoteAddr)
	RespSucess(w, map[string]string{"message": "已删除"})
}

func decimal(f float64) float64 {
	i := int(f * 100)
	return float64(i) / 100
}

type setDbTestConnReq struct {
	DbType   string `json:"db_type"`
	DbSource string `json:"db_source"`
}

// 测试新数据库连接是否可达
func SetDbTestConnection(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	req := &setDbTestConnReq{}
	if err := json.Unmarshal(body, req); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if req.DbType == "" || req.DbSource == "" {
		RespError(w, RespParamErr, "数据库类型和连接字符串不能为空")
		return
	}

	engine, err := xorm.NewEngine(req.DbType, req.DbSource)
	if err != nil {
		base.Error("数据库连接测试失败:", err)
		RespError(w, RespInternalErr, "连接失败")
		return
	}
	defer engine.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := engine.DB().PingContext(ctx); err != nil {
		base.Error("数据库Ping测试失败:", err)
		RespError(w, RespInternalErr, "数据库不可达")
		return
	}

	dbdata.AdminLog("系统设置", "数据库切换", "测试了新数据库连接("+req.DbType+")", r.RemoteAddr)
	RespSucess(w, map[string]string{
		"message": "连接测试成功",
	})
}

type setDbSwitchReq struct {
	DbType    string `json:"db_type"`
	DbSource  string `json:"db_source"`
	Migration string `json:"migration"` // "backup_only" | "auto_migrate" | "none"
}

// 数据库切换向导：测试连接 → 备份/迁移 → 写 db.json → 重启
func SetDbSwitch(w http.ResponseWriter, r *http.Request) {
	restartMux.Lock()
	if restarting {
		restartMux.Unlock()
		RespError(w, RespInternalErr, "正在执行数据库切换或重启，请稍后再试")
		return
	}
	restarting = true
	restartMux.Unlock()

	cleanup := func() {
		restartMux.Lock()
		restarting = false
		restartMux.Unlock()
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		cleanup()
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	req := &setDbSwitchReq{}
	if err := json.Unmarshal(body, req); err != nil {
		cleanup()
		RespError(w, RespInternalErr, err)
		return
	}
	if req.DbType == "" || req.DbSource == "" {
		cleanup()
		RespError(w, RespParamErr, "数据库类型和连接字符串不能为空")
		return
	}
	if req.Migration != "backup_only" && req.Migration != "auto_migrate" && req.Migration != "none" {
		cleanup()
		RespError(w, RespParamErr, "migration 必须为 backup_only、auto_migrate 或 none")
		return
	}

	testEngine, err := xorm.NewEngine(req.DbType, req.DbSource)
	if err != nil {
		cleanup()
		RespError(w, RespInternalErr, "新数据库连接失败: ", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := testEngine.DB().PingContext(ctx); err != nil {
		cleanup()
		RespError(w, RespInternalErr, "新数据库不可达: ", err)
		return
	}

	switch req.Migration {
	case "backup_only":
		filename, err := dbdata.GetBackupManager().Exporter().Create("full", dbdata.GetBackupManager().AllTableNames())
		if err != nil {
			testEngine.Close()
			cleanup()
			RespError(w, RespInternalErr, "备份失败: ", err)
			return
		}
		testEngine.Close()
		base.Info("db switch: backup created", filename)

	case "auto_migrate":
		backupFilename, err := dbdata.GetBackupManager().Exporter().Create("full", dbdata.GetBackupManager().AllTableNames())
		if err != nil {
			testEngine.Close()
			cleanup()
			RespError(w, RespInternalErr, "备份失败: ", err)
			return
		}
		base.Info("db switch: backup created", backupFilename)

		err = testEngine.Sync2(dbdata.TableModels()...)
		if err != nil {
			testEngine.Close()
			cleanup()
			RespError(w, RespInternalErr, "新数据库初始化表结构失败: ", err)
			return
		}

		if err := dbdata.GetBackupManager().NewImporter(testEngine).RestoreToEngine(backupFilename); err != nil {
			testEngine.Close()
			cleanup()
			RespError(w, RespInternalErr, "数据迁移到新库失败: ", err)
			return
		}
		testEngine.Close()
		base.Info("db switch: data migrated to new database")

	case "none":
		testEngine.Close()
		base.Info("db switch: skipping data migration")
	}

	// 写入 db.json（在备份/迁移完成之后）
	if err := dbdata.GetBackupManager().SaveDbConfig(req.DbType, req.DbSource); err != nil {
		cleanup()
		RespError(w, RespInternalErr, "写入 db.json 失败: ", err)
		return
	}

	dbdata.AdminLog("系统设置", "数据库切换", "执行了数据库切换", r.RemoteAddr)
	RespSucess(w, map[string]any{
		"message":       "数据库切换成功，即将重启",
		"needs_restart": true,
	})
	go func() {
		time.Sleep(time.Second)
		sessdata.CleanupAllNatRules()
		if sessdata.GlobalFakeDNSManager != nil {
			sessdata.GlobalFakeDNSManager.Stop()
		}
		destroyLvtap()
		if err := dbdata.Stop(); err != nil {
			base.Warn("db stop before switch restart:", err)
		}
		if err := base.RestartProcess(); err != nil {
			base.Error("db switch restart err:", err)
			cleanup()
		}
	}()
}

// 清理残留的 macvtap 网卡（lvtap 前缀）
func destroyLvtap() {
	its, err := net.Interfaces()
	if err != nil {
		base.Error("destroyLvtap:", err)
		return
	}
	for _, v := range its {
		if strings.HasPrefix(v.Name, "lvtap") {
			link, err := netlink.LinkByName(v.Name)
			if err != nil {
				continue
			}
			netlink.LinkDel(link)
		}
	}
}

// 格式化配置值为审计日志可读字符串，敏感字段脱敏
func formatConfigValue(name string, v any) string {
	if base.IsFieldSensitive(name) {
		return "***"
	}
	s := fmt.Sprint(v)
	if len(s) > 80 {
		s = s[:80] + "..."
	}
	return s
}

// 校验监听地址字段，防止错误配置导致服务重启后不可达
func validateAddressField(name, addr string) error {
	// 仅校验地址类字段
	switch name {
	case "admin_addr", "server_addr", "server_dtls_addr":
	default:
		return nil
	}

	addr = base.FormatListenAddr(addr)
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("「%s」地址格式错误，应为 IP:端口 或 :端口 格式，当前值: %s，错误: %v",
			fieldDisplayName(name), addr, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("「%s」端口号无效，范围 1-65535，当前值: %s", fieldDisplayName(name), portStr)
	}

	// 校验 IP（可选，允许空 host 表示监听所有接口）
	if host != "" {
		if ip := net.ParseIP(host); ip == nil {
			return fmt.Errorf("「%s」IP 地址格式无效: %s", fieldDisplayName(name), host)
		}
	}

	// admin_addr 与 server_addr 端口冲突检查（两者都监听 TCP）
	cfg := base.GetCfg()
	if name == "admin_addr" {
		serverHost, serverPort, err := net.SplitHostPort(base.FormatListenAddr(cfg.ServerAddr))
		if err == nil && serverPort == portStr {
			// 同端口 + 同 IP 或至少一方监听所有接口 → 冲突
			if host == "" || serverHost == "" || host == serverHost {
				return fmt.Errorf("「管理后台地址」端口 %s 与「VPN 服务地址」(%s) 冲突，请使用不同端口",
					portStr, cfg.ServerAddr)
			}
		}
	}
	if name == "server_addr" {
		adminHost, adminPort, err := net.SplitHostPort(base.FormatListenAddr(cfg.AdminAddr))
		if err == nil && adminPort == portStr {
			if host == "" || adminHost == "" || host == adminHost {
				return fmt.Errorf("「VPN 服务地址」端口 %s 与「管理后台地址」(%s) 冲突，请使用不同端口",
					portStr, cfg.AdminAddr)
			}
		}
	}

	return nil
}

// 校验 IPv4 网络配置
func validateIPv4ConfigAll(cidr, gateway, start, end string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("「IPv4 CIDR」格式错误，应为 x.x.x.x/xx 格式，当前值: %s", cidr)
	}

	ipv4Gateway := net.ParseIP(gateway)
	if ipv4Gateway == nil || ipv4Gateway.To4() == nil {
		return fmt.Errorf("「IPv4 网关」地址格式无效: %s", gateway)
	}

	ipStart := net.ParseIP(start)
	if ipStart == nil || ipStart.To4() == nil {
		return fmt.Errorf("「IPv4 起始地址」格式无效: %s", start)
	}

	ipEnd := net.ParseIP(end)
	if ipEnd == nil || ipEnd.To4() == nil {
		return fmt.Errorf("「IPv4 结束地址」格式无效: %s", end)
	}

	// CIDR 的起始 IP（网络地址，如 192.168.90.0）
	netIP := ipNet.IP.To4()

	// 网关/起始地址/结束地址 必须在 CIDR 网段内
	if !ipNet.Contains(ipv4Gateway) {
		return fmt.Errorf("「IPv4 网关」(%s) 不在 CIDR(%s) 网段内", gateway, cidr)
	}
	if !ipNet.Contains(ipStart) {
		return fmt.Errorf("「IPv4 起始地址」(%s) 不在 CIDR(%s) 网段内", start, cidr)
	}
	if !ipNet.Contains(ipEnd) {
		return fmt.Errorf("「IPv4 结束地址」(%s) 不在 CIDR(%s) 网段内", end, cidr)
	}

	// 网关不能是网络地址
	if ipv4Gateway.Equal(netIP) {
		return fmt.Errorf("「IPv4 网关」(%s) 不能与 CIDR 网络地址相同，请更换", gateway)
	}

	// IP 池范围合法性：起始 <= 结束
	if utils.Ip2long(ipStart) > utils.Ip2long(ipEnd) {
		return fmt.Errorf("「IPv4 起始地址」(%s) 不能大于「IPv4 结束地址」(%s)", start, end)
	}

	return nil
}

// 返回字段的中文显示名
func fieldDisplayName(name string) string {
	switch name {
	case "admin_addr":
		return "管理后台地址"
	case "server_addr":
		return "VPN 服务地址"
	case "server_dtls_addr":
		return "DTLS 服务地址"
	default:
		return name
	}
}
