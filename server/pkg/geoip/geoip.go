// Package geoip 基于 MaxMind DB（.mmdb）的 IP → 国家（ISO 3166-1 alpha-2）查询。
//
// 兼容的数据库格式（只读取国家代码字段）：
//   - MaxMind GeoLite2-Country / GeoIP2-Country（country.iso_code，回退 registered_country.iso_code）
//   - DB-IP "IP to Country Lite"（与 GeoLite2 结构一致）
//   - IPinfo Lite 等扁平结构（顶层 country_code）
//
// 数据库文件不存在或无法解析时功能静默关闭，Lookup 一律返回 ""。
// 文件被替换（mtime/size/路径变化）后会在下次查询时自动热加载（最多每 checkInterval 检查一次）。
package geoip

import (
	"container/list"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
)

const (
	defaultCacheSize = 4096
	checkInterval    = 30 * time.Second
)

// 数据库状态
type Status struct {
	Enabled      bool      `json:"enabled"` // 是否已加载
	Path         string    `json:"path"`
	FileExists   bool      `json:"file_exists"`
	FileSize     int64     `json:"file_size"`
	FileModTime  time.Time `json:"file_mod_time"`
	DatabaseType string    `json:"database_type"`
	Description  string    `json:"description"`
	BuildTime    time.Time `json:"build_time"`
	IPVersion    uint      `json:"ip_version"`
	NodeCount    uint      `json:"node_count"`
	RecordCount  int64     `json:"record_count"` // 网段记录数，加载后后台统计，统计完成前为 -1
	LoadedAt     time.Time `json:"loaded_at"`
	Error        string    `json:"error,omitempty"` // 最近一次加载失败原因（文件不存在不算错误）
}

type dbState struct {
	reader  *maxminddb.Reader
	path    string
	size    int64
	modTime time.Time
	loaded  time.Time
	records atomic.Int64
	cache   *lruCache // 缓存随数据库实例绑定，换库即失效
}

// DB 线程安全的 GeoIP 查询器
type DB struct {
	pathFn func() string

	mu    sync.RWMutex // 保护 state（查询持读锁，替换/关闭持写锁）
	state *dbState

	reloadMu  sync.Mutex
	lastCheck atomic.Int64 // unix nano
	lastErr   atomic.Value // string
}

// New 创建查询器，pathFn 返回当前配置的数据库路径（每次检查时调用，支持运行时修改配置）
func New(pathFn func() string) *DB {
	d := &DB{pathFn: pathFn}
	d.lastErr.Store("")
	return d
}

var defaultDB atomic.Pointer[DB]

// Init 初始化默认实例并立即尝试加载
func Init(pathFn func() string) {
	d := New(pathFn)
	defaultDB.Store(d)
	_ = d.Reload()
}

// Default 返回默认实例（未 Init 时返回 nil，所有包级函数安全降级）
func Default() *DB { return defaultDB.Load() }

// Lookup 默认实例查询，见 (*DB).Lookup
func Lookup(ip string) string {
	if d := Default(); d != nil {
		return d.Lookup(ip)
	}
	return ""
}

// Reload 强制重新加载默认实例
func Reload() error {
	if d := Default(); d != nil {
		return d.Reload()
	}
	return errors.New("geoip 未初始化")
}

// GetStatus 默认实例状态
func GetStatus() Status {
	if d := Default(); d != nil {
		return d.Status()
	}
	return Status{RecordCount: -1}
}

// Path 当前配置的数据库路径
func (d *DB) Path() string {
	if d.pathFn == nil {
		return ""
	}
	return strings.TrimSpace(d.pathFn())
}

// Lookup 返回 IP 的国家代码（大写 ISO alpha-2）。
// 支持 "ip"、"ip:port"、"[v6]:port"；私网/回环/链路本地/无效地址、未加载数据库、查不到时返回 ""
func (d *DB) Lookup(ipStr string) string {
	addr, ok := parseAddr(ipStr)
	if !ok || !isPublic(addr) {
		return ""
	}
	d.maybeReload()

	d.mu.RLock()
	defer d.mu.RUnlock()
	st := d.state
	if st == nil {
		return ""
	}
	if v, ok := st.cache.get(addr); ok {
		return v
	}
	code := lookupCode(st.reader, addr)
	st.cache.put(addr, code)
	return code
}

type countryRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	RegisteredCountry struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"registered_country"`
	CountryCode string `maxminddb:"country_code"`
}

func lookupCode(r *maxminddb.Reader, addr netip.Addr) string {
	res := r.Lookup(addr)
	if !res.Found() {
		return ""
	}
	var rec countryRecord
	if err := res.Decode(&rec); err != nil {
		return ""
	}
	code := rec.Country.ISOCode
	if code == "" {
		code = rec.RegisteredCountry.ISOCode
	}
	if code == "" {
		code = rec.CountryCode
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	// DB-IP 对保留地址可能返回 "ZZ"，统一视为未知
	if len(code) != 2 || code == "ZZ" {
		return ""
	}
	return code
}

// 至多每 checkInterval 检查一次文件是否变化
func (d *DB) maybeReload() {
	now := time.Now().UnixNano()
	last := d.lastCheck.Load()
	if now-last < int64(checkInterval) {
		return
	}
	if !d.lastCheck.CompareAndSwap(last, now) {
		return
	}
	if d.changed() {
		_ = d.Reload()
	}
}

func (d *DB) changed() bool {
	path := d.Path()
	d.mu.RLock()
	st := d.state
	d.mu.RUnlock()
	fi, err := os.Stat(path)
	if path == "" || err != nil || fi.IsDir() {
		return st != nil // 文件消失：卸载
	}
	if st == nil {
		return true
	}
	return st.path != path || st.size != fi.Size() || !st.modTime.Equal(fi.ModTime())
}

// Reload 按当前配置路径重新加载数据库；文件不存在时卸载并返回 nil（静默关闭）
func (d *DB) Reload() error {
	d.reloadMu.Lock()
	defer d.reloadMu.Unlock()
	d.lastCheck.Store(time.Now().UnixNano())

	path := d.Path()
	fi, statErr := os.Stat(path)
	if path == "" || statErr != nil || fi.IsDir() {
		d.swap(nil)
		d.lastErr.Store("")
		if path != "" && statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			d.lastErr.Store(statErr.Error())
			return statErr
		}
		return nil
	}

	reader, err := maxminddb.Open(path)
	if err != nil {
		d.lastErr.Store(err.Error())
		return fmt.Errorf("打开 GeoIP 数据库失败: %w", err)
	}
	st := &dbState{reader: reader, path: path, size: fi.Size(), modTime: fi.ModTime(), loaded: time.Now(), cache: newLRU(defaultCacheSize)}
	st.records.Store(-1)
	d.swap(st)
	d.lastErr.Store("")

	// 后台统计网段记录数，仅用于状态展示；使用独立 Reader，避免长时间持锁阻塞查询/热加载
	go func() {
		r, err := maxminddb.Open(path)
		if err != nil {
			return
		}
		defer r.Close()
		var n int64
		for range r.Networks() {
			n++
		}
		st.records.Store(n)
	}()
	return nil
}

func (d *DB) swap(st *dbState) {
	d.mu.Lock()
	old := d.state
	d.state = st
	d.mu.Unlock()
	if old != nil && old.reader != nil {
		_ = old.reader.Close()
	}
}

// Status 当前状态
func (d *DB) Status() Status {
	path := d.Path()
	s := Status{Path: path, RecordCount: -1}
	if fi, err := os.Stat(path); path != "" && err == nil && !fi.IsDir() {
		s.FileExists = true
		s.FileSize = fi.Size()
		s.FileModTime = fi.ModTime()
	}
	if e, _ := d.lastErr.Load().(string); e != "" {
		s.Error = e
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if st := d.state; st != nil {
		m := st.reader.Metadata
		s.Enabled = true
		s.DatabaseType = m.DatabaseType
		s.Description = m.Description["en"]
		s.BuildTime = m.BuildTime()
		s.IPVersion = m.IPVersion
		s.NodeCount = m.NodeCount
		s.RecordCount = st.records.Load()
		s.LoadedAt = st.loaded
	}
	return s
}

// Validate 校验 mmdb 文件可用且为国家库（含国家代码字段），用于上传前检查
func Validate(path string) (maxminddb.Metadata, error) {
	r, err := maxminddb.Open(path)
	if err != nil {
		return maxminddb.Metadata{}, err
	}
	defer r.Close()
	if err := r.Verify(); err != nil {
		return r.Metadata, fmt.Errorf("数据库校验失败: %w", err)
	}
	// 抽样若干公网地址，至少一个能解出国家代码
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "114.114.114.114", "2001:4860:4860::8888", "81.2.69.142"} {
		if lookupCode(r, netip.MustParseAddr(s)) != "" {
			return r.Metadata, nil
		}
	}
	return r.Metadata, errors.New("数据库中未找到国家代码字段（需 Country 级别的 mmdb）")
}

func parseAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Addr{}, false
	}
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Unmap(), true
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		if a, err := netip.ParseAddr(host); err == nil {
			return a.Unmap(), true
		}
	}
	if a, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil {
		return a.Unmap(), true
	}
	return netip.Addr{}, false
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func isPublic(a netip.Addr) bool {
	if !a.IsValid() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() ||
		a.IsLinkLocalMulticast() || a.IsMulticast() || a.IsUnspecified() || a.IsInterfaceLocalMulticast() {
		return false
	}
	if a.Is4() && cgnat.Contains(a) {
		return false
	}
	return true
}

// ---- 小型 LRU 缓存 ----

type lruEntry struct {
	key netip.Addr
	val string
}

type lruCache struct {
	mu    sync.Mutex
	cap   int
	ll    *list.List
	items map[netip.Addr]*list.Element
}

func newLRU(capacity int) *lruCache {
	return &lruCache{cap: capacity, ll: list.New(), items: make(map[netip.Addr]*list.Element, capacity)}
}

func (c *lruCache) get(k netip.Addr) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		c.ll.MoveToFront(e)
		return e.Value.(*lruEntry).val, true
	}
	return "", false
}

func (c *lruCache) put(k netip.Addr, v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		e.Value.(*lruEntry).val = v
		c.ll.MoveToFront(e)
		return
	}
	c.items[k] = c.ll.PushFront(&lruEntry{key: k, val: v})
	for c.ll.Len() > c.cap {
		last := c.ll.Back()
		c.ll.Remove(last)
		delete(c.items, last.Value.(*lruEntry).key)
	}
}
