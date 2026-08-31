// 新增配置项只需改 config.go：在 ServerConfig 结构体加字段，并在 configMetas 加对应元数据。
// 本文件的加载/校验/读写逻辑通过反射 + init() 断言自动适配，无需改动。
// ServerConfig 仅允许 string/int/bool 等值类型（见 init 断言），否则启动即 panic。
package base

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/wsczx/remlink/pkg/mask"
	"github.com/wsczx/remlink/pkg/utils"
)

type ConfigManager struct {
	cfgPtr      atomic.Pointer[ServerConfig]
	explicitSet map[string]bool
}

var (
	configFields         = buildConfigFields()
	configFieldByName    = buildConfigFieldByName(configFields)
	defaultConfigManager = NewConfigManager()
)

func NewConfigManager() *ConfigManager {
	m := &ConfigManager{}
	m.cfgPtr.Store(&ServerConfig{})
	return m
}

func (m *ConfigManager) Get() *ServerConfig { return m.cfgPtr.Load() }

// fn 返回错误时放弃本次提交直接返回
// fn 内只能修改传入的 *ServerConfig
func (m *ConfigManager) mutate(fn func(c *ServerConfig) error) error {
	for {
		old := m.cfgPtr.Load()
		newCfg := *old
		if err := fn(&newCfg); err != nil {
			return err
		}
		if m.cfgPtr.CompareAndSwap(old, &newCfg) {
			return nil
		}
	}
}

// 仅用于绝不会失败的纯字段赋值；若 fn 可能失败，必须改用 mutate（func(cfg) error），
func (m *ConfigManager) Update(fn func(cfg *ServerConfig)) {
	m.mutate(func(c *ServerConfig) error { fn(c); return nil })
}

func (m *ConfigManager) InitDirs() {
	cfg := m.Get()
	dirs := []string{
		filepath.Join(cfg.FilesPath, ".keep"),
	}
	if cfg.DbType == "sqlite3" {
		dirs = append(dirs, cfg.DbSource)
	}
	if cfg.LogPath != "" {
		dirs = append(dirs, cfg.LogPath)
	}
	for _, p := range dirs {
		CreateDir(filepath.Dir(p))
	}
}

func (m *ConfigManager) Complete(cfg *ServerConfig) {
	if cfg.JwtSecret == "" {
		cfg.JwtSecret = NewJwtSecret()
	}

	newAdminPass := ""
	if cfg.AdminPass == "" {
		plainPass, err := GenerateRandomPassword(16)
		if err != nil {
			Error("生成管理员随机密码失败:", err)
		} else {
			hash, hashErr := utils.PasswordHash(plainPass)
			if hashErr != nil {
				Error("生成管理员密码哈希失败:", hashErr)
			} else {
				cfg.AdminPass = hash
				cfg.AdminTemp = true
				newAdminPass = plainPass
			}
		}
	}
	if cfg.AdvertiseDTLSAddr == "" {
		cfg.AdvertiseDTLSAddr = cfg.ServerDTLSAddr
	}
	printAdminBanner(cfg, newAdminPass)
}

// 在首次生成管理员密码时打印账号信息到终端
func printAdminBanner(cfg *ServerConfig, plainPass string) {
	if plainPass == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "========================================\n")
	fmt.Fprintf(os.Stderr, "  RemLink 首次启动 — 管理员账号信息\n")
	fmt.Fprintf(os.Stderr, "========================================\n")
	fmt.Fprintf(os.Stderr, "  用户名:      %s\n", cfg.AdminUser)
	fmt.Fprintf(os.Stderr, "  初始密码:    %s\n", plainPass)
	fmt.Fprintf(os.Stderr, "  管理后台:    https://<服务器IP>%s\n", cfg.AdminAddr)
	fmt.Fprintf(os.Stderr, "========================================\n")
	fmt.Fprintf(os.Stderr, "  ⚠ 请立即登录修改密码！\n")
	fmt.Fprintf(os.Stderr, "  忘记密码时请停止服务后执行: remlink --reset-admin-password\n")
	fmt.Fprintf(os.Stderr, "========================================\n\n")
}

func (m *ConfigManager) ResetAdminPassword() string {
	plainPass, err := GenerateRandomPassword(16)
	if err != nil {
		Error("重置管理员密码失败（随机数不可用）:", err)
		return ""
	}
	hash, err := utils.PasswordHash(plainPass)
	if err != nil {
		Error("重置管理员密码失败（哈希）:", err)
		return ""
	}
	m.Update(func(c *ServerConfig) {
		c.AdminPass = hash
		c.AdminTemp = true
	})
	return plainPass
}

func (m *ConfigManager) DisableAdminOtp() {
	m.Update(func(c *ServerConfig) {
		c.AdminOtp = ""
	})
}

func (m *ConfigManager) EnableFakeDNS() {
	m.Update(func(c *ServerConfig) {
		c.ShowFakeDNS = true
	})
}

func (m *ConfigManager) applyDefaults(cfg *ServerConfig, skipSet map[string]bool) {
	s := reflect.ValueOf(cfg).Elem()
	typ := s.Type()
	for i := 0; i < s.NumField(); i++ {
		field := s.Field(i)
		name := typ.Field(i).Tag.Get("json")
		dv := configMetas[name].defaultVal
		if dv == "" || skipSet[name] {
			continue
		}
		if dv == "defaultPwd" {
			continue
		}
		if !field.IsZero() {
			continue
		}
		if err := setFieldValue(field, dv); err != nil {
			Warn("设置默认值失败", name, err)
		}
	}
}

func (m *ConfigManager) loadDbConfig(cfg *ServerConfig) {
	dbPath := filepath.Join("conf", "db.json")
	b, err := os.ReadFile(dbPath)
	if err != nil {
		// 文件不存在：写入默认数据库配置
		m.ensureDbConfig(cfg)
		return
	}
	if !m.applyDbConfig(cfg, b) {
		// 解析失败不覆盖原文件
		Warn("db.json 解析失败, 将使用默认配置")
	}
}

// 解析 db.json 内容并写入 cfg
func (m *ConfigManager) applyDbConfig(cfg *ServerConfig, b []byte) bool {
	var d struct {
		DbType   string `json:"db_type"`
		DbSource string `json:"db_source"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return false
	}
	if d.DbType != "" {
		cfg.DbType = d.DbType
	}
	if d.DbSource != "" {
		cfg.DbSource = d.DbSource
	}
	return true
}

// 当 db.json 不存在时写入默认数据库配置
func (m *ConfigManager) ensureDbConfig(cfg *ServerConfig) {
	if cfg.DbType == "" {
		cfg.DbType = "sqlite3"
	}
	if cfg.DbSource == "" {
		cfg.DbSource = "./conf/remlink.db"
	}
	d := struct {
		DbType   string `json:"db_type"`
		DbSource string `json:"db_source"`
	}{DbType: cfg.DbType, DbSource: cfg.DbSource}
	if data, err := json.MarshalIndent(d, "", "  "); err == nil {
		CreateDir("conf")
		if err := os.WriteFile(filepath.Join("conf", "db.json"), data, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "[Warn] 写入 %s 失败: %v\n", "conf/db.json", err)
		}
	}
}

func (m *ConfigManager) initCfg() {
	cfg := &ServerConfig{}

	ref := reflect.ValueOf(cfg).Elem()
	typ := ref.Type()

	explicitSet := make(map[string]bool)

	for i := 0; i < ref.NumField(); i++ {
		name := typ.Field(i).Tag.Get("json")
		value := ref.Field(i)
		raw, explicit := readConfigRaw(name)
		// 兼容旧配置键 ipv4_master（重命名为 master_dev 前的环境变量/命令行覆盖）
		if raw == "" && name == "master_dev" {
			raw, explicit = readConfigRaw("ipv4_master")
		}
		switch value.Kind() {
		case reflect.String:
			value.SetString(raw)
		case reflect.Int:
			if n, err := strconv.Atoi(raw); err == nil {
				value.SetInt(int64(n))
			}
		case reflect.Bool:
			if b, err := strconv.ParseBool(raw); err == nil {
				value.SetBool(b)
			}
		}
		if explicit {
			explicitSet[name] = true
		}
	}

	m.loadDbConfig(cfg)
	m.applyDefaults(cfg, explicitSet)
	formatConfigAddrs(cfg)
	m.explicitSet = explicitSet
	m.cfgPtr.Store(cfg)
	m.InitDirs()
}

func (m *ConfigManager) fields() []configFieldMeta {
	return configFields
}

func (m *ConfigManager) field(name string) (configFieldMeta, bool) {
	field, ok := configFieldByName[name]
	return field, ok
}

func (m *ConfigManager) Meta() []map[string]any {
	cfg := m.Get()
	ref := reflect.ValueOf(cfg).Elem()

	var items []map[string]any
	for _, field := range m.fields() {
		if field.hidden {
			continue
		}
		val := ref.Field(field.index).Interface()

		// show_fakedns 为 false 时不在设置页显示，用户只能通过命令行重新开启
		if field.name == "show_fakedns" && val == false {
			continue
		}
		if field.sensitive {
			val = mask.Placeholder
		}

		item := map[string]any{
			"name":      field.name,
			"usage":     field.usage,
			"type":      field.typ,
			"group":     field.group,
			"restart":   field.restart,
			"sensitive": field.sensitive,
			"readonly":  field.readonly,
			"multiline": field.multiline,
			"data":      val,
		}
		if field.options != nil {
			item["options"] = field.options
		}
		items = append(items, item)
	}
	return items
}

func (m *ConfigManager) SetField(name string, data any) (restart bool, err error) {
	field, found := m.field(name)
	if !found {
		return false, fmt.Errorf("未知配置项: %s", name)
	}
	if field.hidden {
		return false, fmt.Errorf("隐藏字段 %s 不能通过此 API 修改", name)
	}
	if field.readonly {
		return false, fmt.Errorf("只读字段 %s 不能通过此 API 修改", name)
	}
	if field.sensitive && fmt.Sprint(data) == mask.Placeholder {
		return false, fmt.Errorf("敏感配置 %s 未修改, 不能使用占位符", name)
	}
	restart = field.restart

	if err = m.mutate(func(c *ServerConfig) error {
		value := reflect.ValueOf(c).Elem().Field(field.index)
		if err := setFieldValue(value, data); err != nil {
			return fmt.Errorf("设置配置项 %s 失败: %w", name, err)
		}
		formatConfigAddrs(c)
		if c.AdvertiseDTLSAddr == "" {
			c.AdvertiseDTLSAddr = c.ServerDTLSAddr
		}
		return nil
	}); err != nil {
		return false, err
	}

	cfg := m.Get()
	if name == "log_level" {
		baseLevel.Store(int32(logLevel2Int(cfg.LogLevel)))
	}
	if name == "log_path" {
		ReinitLog()
	}
	if name == "files_path" {
		m.InitDirs()
	}
	return restart, nil
}

func (m *ConfigManager) IsSensitive(name string) bool {
	field, found := m.field(name)
	return found && field.sensitive
}
func FormatListenAddr(addr string) string {
	if addr == "" {
		return addr
	}
	if strings.Contains(addr, ":") {
		return addr
	}
	return ":" + addr
}

// 将绑定地址的 v4 通配写法 0.0.0.0:port 修复成:port，双栈监听（同时接受 v4/v6 客户端）。
func fixListenAddr(addr string) string {
	if addr == "" {
		return addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "0.0.0.0" {
		return ":" + port
	}
	return addr
}

func formatConfigAddrs(cfg *ServerConfig) {
	cfg.ServerAddr = fixListenAddr(FormatListenAddr(cfg.ServerAddr))
	cfg.ServerDTLSAddr = fixListenAddr(FormatListenAddr(cfg.ServerDTLSAddr))
	cfg.AdminAddr = fixListenAddr(FormatListenAddr(cfg.AdminAddr))
	cfg.AdvertiseDTLSAddr = FormatListenAddr(cfg.AdvertiseDTLSAddr)
}

// 将 src 中 json 名为 name 的字段值复制到 dst，返回被复制的值
func copyFieldByName(dst, src *ServerConfig, name string) reflect.Value {
	ref := reflect.ValueOf(dst).Elem()
	typ := ref.Type()
	for i := 0; i < ref.NumField(); i++ {
		if typ.Field(i).Tag.Get("json") == name {
			v := reflect.ValueOf(src).Elem().Field(i)
			ref.Field(i).Set(v)
			return v
		}
	}
	return reflect.Value{}
}

// 用数据库持久化配置覆盖当前配置，并叠加命令行/环境变量优先级。
//
//	db.json > 命令行/环境变量(explicitSet) > DB 持久化(incoming) > 启动期 flag 值(敏感字段, DB 为空时回退)
func (m *ConfigManager) LoadPersisted(incoming ServerConfig) {
	m.Update(func(c *ServerConfig) {
		// 含 db.json 值与命令行/环境变量/默认值
		dbType, dbSource := c.DbType, c.DbSource
		startup := *c

		*c = incoming // 先整体采用数据库持久化配置

		// 命令行/环境变量显式设置的字段优先于数据库持久化配置
		for name := range m.explicitSet {
			v := copyFieldByName(c, &startup, name)
			if !v.IsValid() {
				continue
			}
			if m.IsSensitive(name) {
				Info("命令行参数覆盖数据库配置:", name, "= ******(已隐藏)")
			} else {
				Info("命令行参数覆盖数据库配置:", name, "=", fmt.Sprint(v.Interface()))
			}
		}

		// db.json 来源的数据库配置不被 DB 持久化覆盖
		c.DbType, c.DbSource = dbType, dbSource

		// 敏感字段：数据库有值则用数据库，否则保留启动期 flag 值
		if incoming.JwtSecret == "" {
			c.JwtSecret = startup.JwtSecret
		}
		if incoming.AdminPass == "" {
			c.AdminPass = startup.AdminPass
		}
		if incoming.AdminOtp == "" {
			c.AdminOtp = startup.AdminOtp
		}

		if c.AdminPass == "" || c.JwtSecret == "" {
			m.Complete(c)
		} else if c.AdvertiseDTLSAddr == "" {
			c.AdvertiseDTLSAddr = c.ServerDTLSAddr
		}
		formatConfigAddrs(c)
	})
	m.InitDirs()
	// 重新应用日志路径
	ReinitLog()
}

func (m *ConfigManager) Warnings() []SystemWarning {
	cfg := m.Get()
	var warnings []SystemWarning
	if strings.Contains(cfg.Issuer, "XX公司") {
		warnings = append(warnings, SystemWarning{
			Code:       "initial_setup",
			Level:      "warning",
			Message:    InitialSetupWarning,
			ActionPath: "/admin/set/soft",
		})
	}
	if cfg.AdminTemp {
		warnings = append(warnings, SystemWarning{
			Code:       "admin_temp_password",
			Level:      "error",
			Message:    "管理员仍在使用首次生成或命令重置后的临时密码，请在系统设置 > 安全设置中立即修改管理员密码",
			ActionPath: "/admin/set/security",
		})
	}
	if cfg.GlobalNat || cfg.GlobalNat6 {
		if _, err := net.InterfaceByName(cfg.MasterDev); err != nil {
			ifaces := utils.GetPhysicalInterfaces()
			msg := "NAT配置错误：主网卡未正确配置，NAT转发规则无法生效，请在软件配置中修改 master_dev"
			if len(ifaces) > 0 {
				msg += fmt.Sprintf("（可用物理网卡: %s）", strings.Join(ifaces, ", "))
			}
			warnings = append(warnings, SystemWarning{
				Code:       "nat_interface_missing",
				Level:      "error",
				Message:    msg,
				ActionPath: "/admin/set/soft",
			})
		}
	}

	return warnings
}

func (m *ConfigManager) SetForTest(cfg *ServerConfig) { m.cfgPtr.Store(cfg) }

type configFieldMeta struct {
	index     int
	name      string
	usage     string
	typ       string
	group     string
	restart   bool
	sensitive bool
	hidden    bool
	readonly  bool
	multiline bool
	options   map[string]string
}

func valueType(kind reflect.Kind) string {
	switch kind {
	case reflect.Int:
		return "int"
	case reflect.Bool:
		return "bool"
	default:
		return "string"
	}
}

func buildConfigFields() []configFieldMeta {
	typ := reflect.TypeFor[ServerConfig]()
	fields := make([]configFieldMeta, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		fields = append(fields, buildFieldMeta(typ.Field(i), i))
	}
	return fields
}

func buildConfigFieldByName(fields []configFieldMeta) map[string]configFieldMeta {
	result := make(map[string]configFieldMeta, len(fields))
	for _, field := range fields {
		result[field.name] = field
	}
	return result
}

// 启动期校验 ServerConfig 字段与 configMetas 一一对应，避免元数据遗漏或多余
func init() {
	typ := reflect.TypeFor[ServerConfig]()
	structNames := make(map[string]bool, typ.NumField())
	for field := range typ.Fields() {
		name := field.Tag.Get("json")
		structNames[name] = true
		if _, ok := configMetas[name]; !ok {
			panic("configMetas 缺少字段元数据: " + name)
		}
	}
	for name := range configMetas {
		if !structNames[name] {
			panic("configMetas 存在多余字段(在 ServerConfig 中不存在): " + name)
		}
	}

	// ServerConfig 必须仅含可比较的值类型（string/int/bool 等）。
	// mutate 通过浅值拷贝 newCfg := *old 实现无锁并发安全；
	// 若引入 slice/map/指针等引用类型字段，浅拷贝会让 fn 直接修改共享的 old 对象，破坏并发安全。
	for field := range typ.Fields() {
		switch field.Type.Kind() {
		case reflect.String, reflect.Bool,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
			// 值类型，允许
		default:
			panic("ServerConfig 含非值类型字段 " + field.Name +
				" (" + field.Type.Kind().String() + ")，会破坏 mutate 的无锁并发安全：请改用值类型或实现深拷贝")
		}
	}
}

func buildFieldMeta(field reflect.StructField, index int) configFieldMeta {
	tag := field.Tag
	name := tag.Get("json")
	meta := configMetas[name]
	return configFieldMeta{
		index:     index,
		name:      name,
		usage:     meta.usage,
		typ:       valueType(field.Type.Kind()),
		group:     meta.group,
		restart:   meta.restart,
		sensitive: meta.sensitive,
		hidden:    meta.hidden,
		readonly:  meta.readonly,
		multiline: meta.multiline,
		options:   meta.options,
	}
}

// 将 data 按字段类型转换为对应值并写入 value。
func setFieldValue(value reflect.Value, data any) error {
	switch value.Kind() {
	case reflect.String:
		value.SetString(fmt.Sprint(data))
	case reflect.Int:
		switch v := data.(type) {
		case float64:
			value.SetInt(int64(v))
		case int:
			value.SetInt(int64(v))
		default:
			var iv int64
			if _, err := fmt.Sscan(fmt.Sprint(data), &iv); err != nil {
				return err
			}
			value.SetInt(iv)
		}
	case reflect.Bool:
		value.SetBool(fmt.Sprint(data) == "true")
	default:
		return fmt.Errorf("不支持的配置类型")
	}
	return nil
}

func initCfg() { defaultConfigManager.initCfg() }

func GetCfg() *ServerConfig { return defaultConfigManager.Get() }

func UpdateCfg(fn func(cfg *ServerConfig)) { defaultConfigManager.Update(fn) }

func CompleteConfig(cfg *ServerConfig) { defaultConfigManager.Complete(cfg) }

// 重置管理员密码，返回明文密码。
func ResetAdminPassword() string { return defaultConfigManager.ResetAdminPassword() }

// 清空管理员 OTP 密钥。
func DisableAdminOtp() { defaultConfigManager.DisableAdminOtp() }

// 开启 FakeDNS 功能可见性。
func EnableFakeDNS() { defaultConfigManager.EnableFakeDNS() }

// 返回前端设置页所需的配置元数据（含当前值）。
func GetConfigMeta() []map[string]any { return defaultConfigManager.Meta() }

// 修改单个配置项，返回是否需要重启。
func SetConfigField(name string, data any) (restart bool, err error) {
	return defaultConfigManager.SetField(name, data)
}

// 判断配置项是否敏感。
func IsFieldSensitive(name string) bool { return defaultConfigManager.IsSensitive(name) }

// 用数据库配置覆盖当前配置。
func LoadPersistedConfig(incoming ServerConfig) { defaultConfigManager.LoadPersisted(incoming) }

// 返回当前配置触发的系统警告。
func GetSystemWarnings() []SystemWarning { return defaultConfigManager.Warnings() }

// 替换全局配置，仅用于测试。
func SetCfgForTest(cfg *ServerConfig) { defaultConfigManager.SetForTest(cfg) }

// 使用系统随机源生成密码。
func GenerateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("crypto/rand 读取失败: %w", err)
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}

func CreateDir(dir string) {
	if dir == "" || dir == "." {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Fprintln(os.Stderr, "create dir err:", err)
	}
}

func NewJwtSecret() string {
	jwtSecret, err := utils.RandSecret(40, 60)
	if err != nil {
		Error("生成JWT密钥失败:", err)
	}
	return strings.Trim(jwtSecret, "=")
}
