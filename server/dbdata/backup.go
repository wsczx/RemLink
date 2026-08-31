package dbdata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wsczx/remlink/base"
	"xorm.io/xorm"
)

const backupFileDir = "./conf/backup"
const currentBackupVersion = 1
const restoreBatchSize = 500

type backupTable struct {
	Name  string // 表名
	Label string // 中文名称
	Group string // "business" / "log" / "stats"
	Model any    // 结构体实例，如 User{}
}

func (t backupTable) newSlicePtr() any {
	return reflect.New(reflect.SliceOf(reflect.TypeOf(t.Model))).Interface()
}

func (t backupTable) newPtr() any {
	return reflect.New(reflect.TypeOf(t.Model)).Interface()
}

// 注册所有需要备份/还原的业务表（元数据驱动）
// 新增数据表时务必在此登记，否则不会被备份也不会被建表。
var backupTables = []backupTable{
	{"user", "用户", "business", User{}},
	{"group", "用户组", "business", Group{}},
	{"setting", "系统设置", "business", Setting{}},
	{"policy", "用户策略", "business", Policy{}},
	{"client_cert_data", "客户端证书", "business", ClientCertData{}},
	{"ip_map", "IP-MAC绑定", "business", IpMap{}},
	{"password_reset", "密码重置令牌", "business", PasswordReset{}},
	{"provider", "认证源", "business", Provider{}},
	{"user_act_log", "用户活动日志", "log", UserActLog{}},
	{"access_audit", "访问审计", "log", AccessAudit{}},
	{"admin_op_log", "管理员操作日志", "log", AdminOpLog{}},
	{"stats_online", "在线人数统计", "stats", StatsOnline{}},
	{"stats_network", "网络流量统计", "stats", StatsNetwork{}},
	{"stats_cpu", "CPU使用率统计", "stats", StatsCpu{}},
	{"stats_mem", "内存使用率统计", "stats", StatsMem{}},
	{"webvpn_app", "WebVPN应用", "business", WebVpnApp{}},
	{"webvpn_audit", "WebVPN审计", "log", WebVpnAudit{}},
	{"webvpn_revoke", "WebVPN会话吊销", "business", WebVpnRevoke{}},
}

func backupTableByNameMap() map[string]backupTable {
	m := make(map[string]backupTable, len(backupTables))
	for _, t := range backupTables {
		m[t.Name] = t
	}
	return m
}

// 备份/还原子系统的中心对象（单例）。
// 聚合 Exporter（导出）与 Importer（导入）两个子组件
// 各子组件通过构造函数持有 *xorm.Engine，避免 *xorm.Session 在调用链中层层透传
type BackupManager struct {
	Tables      []backupTable
	TableByName map[string]backupTable
}

var (
	backupManagerOnce sync.Once
	backupManagerInst *BackupManager
)

// 返回全局唯一的 BackupManager 单例
func GetBackupManager() *BackupManager {
	backupManagerOnce.Do(func() {
		backupManagerInst = &BackupManager{
			Tables:      backupTables,
			TableByName: backupTableByNameMap(),
		}
	})
	return backupManagerInst
}

// 返回默认引擎（xdb）的备份导出器
func (m *BackupManager) Exporter() *Exporter {
	return NewExporter(xdb)
}

// 返回绑定指定引擎的备份导入器
func (m *BackupManager) NewImporter(engine *xorm.Engine) *Importer {
	return NewImporter(engine)
}

// 还原到默认引擎（xdb），成功后热加载配置
func (m *BackupManager) Restore(filename string) error {
	return NewImporter(xdb).Restore(filename)
}

// 返回所有已注册备份表的名称（已排序）
func (m *BackupManager) AllTableNames() []string {
	names := make([]string, len(m.Tables))
	for i, t := range m.Tables {
		names[i] = t.Name
	}
	sort.Strings(names)
	return names
}

// 返回所有 business 组的表名
func (m *BackupManager) BusinessTableNames() []string {
	var names []string
	for _, t := range m.Tables {
		if t.Group == "business" {
			names = append(names, t.Name)
		}
	}
	return names
}

// 返回各备份表的行数统计
func (m *BackupManager) GetTableSizes() []TableSizeInfo {
	result := make([]TableSizeInfo, 0, len(m.Tables))
	for _, t := range m.Tables {
		result = append(result, TableSizeInfo{
			Table: t.Name,
			Name:  t.Label,
			Rows:  m.TableCount(t.Name),
			Group: t.Group,
		})
	}
	return result
}

// 返回指定表的行数
func (m *BackupManager) TableCount(name string) int64 {
	bt, ok := m.TableByName[name]
	if !ok {
		return 0
	}
	n, err := xdb.Count(bt.newPtr())
	if err != nil {
		base.Error("TableCount:", name, err)
		return 0
	}
	return n
}

// 列出备份目录下的所有备份文件
func (m *BackupManager) ListBackups() ([]BackupFileInfo, error) {
	base.CreateDir(backupFileDir)
	entries, err := os.ReadDir(backupFileDir)
	if err != nil {
		return nil, err
	}

	var result []BackupFileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "remlink_backup_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		bt := "config"
		if strings.Contains(name, "_full_") {
			bt = "full"
		}
		result = append(result, BackupFileInfo{
			Name:    name,
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			Type:    bt,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ModTime > result[j].ModTime
	})
	return result, nil
}

// 删除指定备份文件
func (m *BackupManager) DeleteBackup(filename string) error {
	cleaned := filepath.Clean(filename)
	if cleaned != filepath.Base(cleaned) || cleaned == "." || cleaned == ".." {
		return fmt.Errorf("invalid backup filename")
	}
	if !strings.HasPrefix(cleaned, "remlink_backup_") {
		return fmt.Errorf("invalid backup filename")
	}
	return os.Remove(filepath.Join(backupFileDir, cleaned))
}

// 持久化数据库连接配置到 conf/db.json
func (m *BackupManager) SaveDbConfig(dbType, dbSource string) error {
	cfg := dbConfig{DbType: dbType, DbSource: dbSource}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	base.CreateDir("conf")
	return os.WriteFile(filepath.Join("conf", "db.json"), b, 0600)
}

// Exporter 负责把数据库内容序列化为备份文件。
// 通过 NewExporter 显式持有 *xorm.Engine
type Exporter struct {
	engine *xorm.Engine
}

// 构造一个绑定指定引擎的 Exporter
func NewExporter(engine *xorm.Engine) *Exporter {
	return &Exporter{engine: engine}
}

// 创建一个备份文件并返回文件名
// backupType 为 "config"（仅配置/证书）或 "full"（含业务表数据）
// includeTables 仅对 "full" 生效，为空时导出全部 business 表
func (e *Exporter) Create(backupType string, includeTables []string) (string, error) {
	cfg := base.GetCfg()
	cfgCopy := *cfg
	cfgCopy.JwtSecret = ""
	cfgCopy.AdminPass = ""
	cfgCopy.AdminOtp = ""

	data := &BackupData{
		Version:   currentBackupVersion,
		Type:      backupType,
		CreatedAt: time.Now().Format(time.RFC3339),
		DbType:    cfg.DbType,
		DbSource:  cfg.DbSource,
		Config:    &cfgCopy,
	}

	var tlsCert SettingTLSCert
	if err := SettingGet(&tlsCert); err == nil && tlsCert.CertContent != "" {
		data.TLSCert = &tlsCert
	}
	var clientCA SettingClientCA
	if err := SettingGet(&clientCA); err == nil && clientCA.CertContent != "" {
		data.ClientCA = &clientCA
	}

	if backupType == "full" {
		tables := includeTables
		if tables == nil {
			tables = GetBackupManager().BusinessTableNames()
		}
		data.Tables = make(map[string]json.RawMessage, len(tables))
		for _, t := range tables {
			raw, err := e.exportTable(t)
			if err != nil {
				return "", fmt.Errorf("备份表 %s 失败: %w", t, err)
			}
			data.Tables[t] = raw
		}
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}

	tag := "config"
	if backupType == "full" {
		tag = "full"
	}
	ts := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("remlink_backup_%s_%s.json", tag, ts)
	filePath := filepath.Join(backupFileDir, filename)

	base.CreateDir(backupFileDir)
	if err := os.WriteFile(filePath, b, 0600); err != nil {
		return "", err
	}

	base.Info("backup created:", filePath)
	return filename, nil
}

func (e *Exporter) exportTable(name string) (json.RawMessage, error) {
	bt, ok := GetBackupManager().TableByName[name]
	if !ok {
		return nil, fmt.Errorf("unknown table: %s", name)
	}
	slicePtr := bt.newSlicePtr()
	if err := e.engine.Find(slicePtr); err != nil {
		return nil, err
	}
	rv := reflect.ValueOf(slicePtr).Elem()
	rows := make([]json.RawMessage, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		b, err := marshalEncryptedRow(rv.Index(i).Addr().Interface())
		if err != nil {
			return nil, err
		}
		rows[i] = b
	}
	return json.Marshal(rows)
}

// Importer 负责把备份文件还原到指定引擎
// 通过 NewImporter 显式持有 *xorm.Engine
type Importer struct {
	engine *xorm.Engine
}

// 构造一个绑定指定引擎的 Importer
func NewImporter(engine *xorm.Engine) *Importer {
	return &Importer{engine: engine}
}

// 还原到默认引擎（xdb），并在成功后热加载配置
func (im *Importer) Restore(filename string) error {
	data, err := im.ReadFile(filename)
	if err != nil {
		return err
	}

	sess := im.engine.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}

	cfgCopy, err := im.restoreToSession(sess, data)
	if err != nil {
		sess.Rollback()
		return err
	}

	if err := sess.Commit(); err != nil {
		return err
	}

	if cfgCopy != nil {
		base.LoadPersistedConfig(*cfgCopy)
		resetClientCA()
	}

	base.Info("backup restored:", filename)
	return nil
}

// 还原到指定引擎（不热加载配置，用于测试/离线校验）
func (im *Importer) RestoreToEngine(filename string) error {
	data, err := im.ReadFile(filename)
	if err != nil {
		return err
	}

	sess := im.engine.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}

	if _, err = im.restoreToSession(sess, data); err != nil {
		sess.Rollback()
		return err
	}

	return sess.Commit()
}

// 在已有事务中还原所有表与配置
func (im *Importer) restoreToSession(sess *xorm.Session, data *BackupData) (*base.ServerConfig, error) {
	if err := im.restoreTables(sess, data); err != nil {
		return nil, err
	}
	return im.restoreConfig(sess, data)
}

func (im *Importer) restoreTables(sess *xorm.Session, data *BackupData) error {
	if data.Tables == nil {
		return nil
	}

	existing := make(map[string]bool, len(data.Tables))
	for name := range data.Tables {
		bt, ok := GetBackupManager().TableByName[name]
		if !ok {
			continue
		}
		exist, err := sess.IsTableExist(bt.newPtr())
		if err != nil {
			return fmt.Errorf("检查表 %s 是否存在失败: %w", name, err)
		}
		existing[name] = exist
	}

	for name := range data.Tables {
		if !existing[name] {
			continue
		}
		if err := im.ClearTable(sess, name); err != nil {
			return fmt.Errorf("清空表 %s 失败: %w", name, err)
		}
	}

	for name, raw := range data.Tables {
		if !existing[name] {
			continue
		}
		if err := im.ImportTable(sess, name, raw); err != nil {
			return fmt.Errorf("还原表 %s 失败: %w", name, err)
		}
	}

	return nil
}

// 清空指定表的所有行
func (im *Importer) ClearTable(sess *xorm.Session, name string) error {
	bt, ok := GetBackupManager().TableByName[name]
	if !ok {
		return fmt.Errorf("unknown table: %s", name)
	}
	_, err := sess.Where("1=1").Delete(bt.newPtr())
	return err
}

// 将单张表的 JSON 行批量写入
func (im *Importer) ImportTable(sess *xorm.Session, name string, raw json.RawMessage) error {
	bt, ok := GetBackupManager().TableByName[name]
	if !ok {
		return fmt.Errorf("unknown table: %s", name)
	}

	slicePtr := bt.newSlicePtr()
	if err := json.Unmarshal(raw, slicePtr); err != nil {
		return fmt.Errorf("解析表 %s 数据失败: %w", name, err)
	}

	v := reflect.ValueOf(slicePtr).Elem()
	n := v.Len()
	if n == 0 {
		return nil
	}

	if n <= restoreBatchSize {
		_, err := sess.Insert(slicePtr)
		return err
	}

	for i := 0; i < n; i += restoreBatchSize {
		end := min(i+restoreBatchSize, n)
		batch := v.Slice(i, end).Interface()
		if _, err := sess.Insert(batch); err != nil {
			return err
		}
	}
	return nil
}

func (im *Importer) restoreConfig(sess *xorm.Session, data *BackupData) (*base.ServerConfig, error) {
	if data.Config == nil {
		return nil, nil
	}

	cfgCopy := *data.Config
	cfgSnapshot := base.GetCfg()
	cfgCopy.DbType = cfgSnapshot.DbType
	cfgCopy.DbSource = cfgSnapshot.DbSource

	// 恢复密钥
	var currentSC SettingServerConfig
	if err := SettingSessGet(sess, &currentSC); err == nil {
		if cfgCopy.JwtSecret == "" {
			cfgCopy.JwtSecret = currentSC.Config.JwtSecret
		}
		if cfgCopy.AdminPass == "" {
			cfgCopy.AdminPass = currentSC.Config.AdminPass
		}
		if cfgCopy.AdminOtp == "" {
			cfgCopy.AdminOtp = currentSC.Config.AdminOtp
		}
	}

	sc := &SettingServerConfig{Config: cfgCopy}
	if _, err := sess.Where("name = ?", "SettingServerConfig").Delete(&Setting{}); err != nil {
		return nil, err
	}
	if err := SettingSessAdd(sess, sc); err != nil {
		return nil, fmt.Errorf("还原配置失败: %w", err)
	}

	if data.TLSCert != nil && data.TLSCert.CertContent != "" {
		if err := restoreCertSetting(sess, "SettingTLSCert", data.TLSCert); err != nil {
			return nil, fmt.Errorf("还原 TLS 证书失败: %w", err)
		}
	}
	if data.ClientCA != nil && data.ClientCA.CertContent != "" {
		if err := restoreCertSetting(sess, "SettingClientCA", data.ClientCA); err != nil {
			return nil, fmt.Errorf("还原客户端 CA 证书失败: %w", err)
		}
	}

	return &cfgCopy, nil
}

// 读取并校验备份文件，返回解析后的结构
func (im *Importer) ReadFile(filename string) (*BackupData, error) {
	cleaned := filepath.Clean(filename)
	if cleaned != filepath.Base(cleaned) || cleaned == "." || cleaned == ".." {
		return nil, fmt.Errorf("invalid backup filename")
	}
	if !strings.HasPrefix(cleaned, "remlink_backup_") {
		return nil, fmt.Errorf("invalid backup filename")
	}

	b, err := os.ReadFile(filepath.Join(backupFileDir, cleaned))
	if err != nil {
		return nil, fmt.Errorf("读取备份文件失败: %w", err)
	}

	var data BackupData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("解析备份文件失败: %w", err)
	}

	if data.Version != currentBackupVersion {
		return nil, fmt.Errorf("备份文件版本不兼容: 当前支持版本 %d，备份文件版本 %d", currentBackupVersion, data.Version)
	}

	return &data, nil
}

type BackupData struct {
	Version   int                        `json:"version"`
	Type      string                     `json:"type"` // "config" | "full"
	CreatedAt string                     `json:"created_at"`
	DbType    string                     `json:"db_type"`
	DbSource  string                     `json:"db_source"`
	Config    *base.ServerConfig         `json:"config,omitempty"`
	Tables    map[string]json.RawMessage `json:"tables,omitempty"`
	TLSCert   *SettingTLSCert            `json:"tls_cert,omitempty"`
	ClientCA  *SettingClientCA           `json:"client_ca,omitempty"`
}

type TableSizeInfo struct {
	Table string `json:"table"`
	Name  string `json:"name"`
	Rows  int64  `json:"rows"`
	Group string `json:"group"`
}

type BackupFileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
	Type    string `json:"type"`
}

type dbConfig struct {
	DbType   string `json:"db_type"`
	DbSource string `json:"db_source"`
}

// 对 EncryptedJSON 字段输出加密形式（备份不落明文凭证）
func marshalEncryptedRow(v any) (json.RawMessage, error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return json.Marshal(v)
	}
	out := make(map[string]json.RawMessage, rv.NumField())
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := tag
		if before, _, ok := strings.Cut(tag, ","); ok {
			name = before
		}
		fv := rv.Field(i)
		if ej, ok := fv.Addr().Interface().(interface {
			MarshalEncrypted() ([]byte, error)
		}); ok {
			b, err := ej.MarshalEncrypted()
			if err != nil {
				return nil, err
			}
			out[name] = b
			continue
		}
		b, err := json.Marshal(fv.Interface())
		if err != nil {
			return nil, err
		}
		out[name] = b
	}
	return json.Marshal(out)
}

func restoreCertSetting(sess *xorm.Session, name string, certData any) error {
	b, err := json.Marshal(certData)
	if err != nil {
		return fmt.Errorf("序列化 %s 失败: %w", name, err)
	}
	if _, err := sess.Where("name = ?", name).Delete(&Setting{}); err != nil {
		return err
	}
	_, err = sess.InsertOne(&Setting{Name: name, Data: b})
	return err
}
