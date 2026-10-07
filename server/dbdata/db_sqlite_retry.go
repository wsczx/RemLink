package dbdata

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"time"

	sqlite "modernc.org/sqlite"
)

// SQLite 瞬时写锁竞争的重试次数与退避步长。busy_timeout 已让驱动在拿不到写锁时
// 等待最多 5 秒，这里只在仍超时（极端并发）时做应用层退避兜底
const (
	sqliteRetryAttempts = 4
	sqliteRetryBaseWait = 20 * time.Millisecond
)

// 判断错误是否为 SQLite 并发写锁竞争（瞬时错误，可重试）
// 这两类文案是 SQLite 驱动特有，其它数据库的错误不会被误判
func isSqliteLocked(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database is busy")
}

// 包装 SQLite 驱动，在连接层对 SQLite 锁错误做退避重试
type retrySqliteDriver struct {
	d *sqlite.Driver
}

// modernc.org/sqlite 默认把驱动注册为 "sqlite"，重新注册为 "sqlite3" 以保持兼容
func init() {
	if slices.Contains(sql.Drivers(), "sqlite3") {
		return
	}
	sql.Register("sqlite3", &sqlite.Driver{})
}

func (r *retrySqliteDriver) Open(dsn string) (driver.Conn, error) {
	conn, err := r.d.Open(dsn)
	if err != nil {
		return nil, err
	}
	return &retrySqliteConn{conn: conn}, nil
}

// 实现 driver.Connector，供 sql.OpenDB 使用
type retryConnector struct {
	d   *retrySqliteDriver
	dsn string
}

func (c *retryConnector) Driver() driver.Driver { return c.d }

func (c *retryConnector) Connect(ctx context.Context) (driver.Conn, error) {
	return c.d.Open(c.dsn)
}

// 对单次 exec 做 SQLite 锁退避重试，非锁错误立即返回
func execWithRetry(fn func() (driver.Result, error)) (driver.Result, error) {
	var lastErr error
	for attempt := range sqliteRetryAttempts {
		res, err := fn()
		if err == nil || !isSqliteLocked(err) {
			return res, err
		}
		lastErr = err
		time.Sleep(sqliteRetryBaseWait * time.Duration(attempt+1))
	}
	return nil, lastErr
}

// 对单次 query 做 SQLite 锁退避重试（读也可能撞锁）
func queryWithRetry(fn func() (driver.Rows, error)) (driver.Rows, error) {
	var lastErr error
	for attempt := range sqliteRetryAttempts {
		rows, err := fn()
		if err == nil || !isSqliteLocked(err) {
			return rows, err
		}
		lastErr = err
		time.Sleep(sqliteRetryBaseWait * time.Duration(attempt+1))
	}
	return nil, lastErr
}

// 对 Exec/Query 加锁重试；其余方法（含底层实现的可选接口）如有则透传，避免功能降级
type retrySqliteConn struct {
	conn driver.Conn
}

func (c *retrySqliteConn) Prepare(query string) (driver.Stmt, error) {
	return c.conn.Prepare(query)
}

func (c *retrySqliteConn) Close() error { return c.conn.Close() }
func (c *retrySqliteConn) Begin() (driver.Tx, error) {
	if v, ok := c.conn.(driver.ConnBeginTx); ok {
		return v.BeginTx(context.Background(), driver.TxOptions{})
	}
	return nil, errors.New("underlying conn does not support ConnBeginTx")
}

func (c *retrySqliteConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if v, ok := c.conn.(driver.ConnPrepareContext); ok {
		return v.PrepareContext(ctx, query)
	}
	return c.conn.Prepare(query)
}

func (c *retrySqliteConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if v, ok := c.conn.(driver.ConnBeginTx); ok {
		return v.BeginTx(ctx, opts)
	}
	return nil, errors.New("underlying conn does not support ConnBeginTx")
}

func (c *retrySqliteConn) CheckNamedValue(v *driver.NamedValue) error {
	if chk, ok := c.conn.(driver.NamedValueChecker); ok {
		return chk.CheckNamedValue(v)
	}
	return driver.ErrSkip
}

func (c *retrySqliteConn) IsValid() bool {
	if v, ok := c.conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

func (c *retrySqliteConn) ResetSession(ctx context.Context) error {
	if v, ok := c.conn.(driver.SessionResetter); ok {
		return v.ResetSession(ctx)
	}
	return nil
}

// 实现 driver.ExecerContext，带锁重试
func (c *retrySqliteConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if v, ok := c.conn.(driver.ExecerContext); ok {
		return execWithRetry(func() (driver.Result, error) {
			return v.ExecContext(ctx, query, args)
		})
	}
	return nil, driver.ErrSkip
}

// 实现 driver.QueryerContext，带锁重试
func (c *retrySqliteConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if v, ok := c.conn.(driver.QueryerContext); ok {
		return queryWithRetry(func() (driver.Rows, error) {
			return v.QueryContext(ctx, query, args)
		})
	}
	return nil, driver.ErrSkip
}

// 返回注入锁重试的 *sql.DB（仅 sqlite3 使用）
func newRetrySqliteDB(dsn string) (*sql.DB, error) {
	connector := &retryConnector{
		d:   &retrySqliteDriver{d: &sqlite.Driver{}},
		dsn: dsn,
	}
	return sql.OpenDB(connector), nil
}

// 规范化 SQLite DSN（modernc.org/sqlite 语法）：
//   - 旧版 mattn 驱动的 _busy_timeout=N 在 modernc 下不生效，转换为 _pragma=busy_timeout(N)，
//     未配置时默认 5000ms。否则驱动拿不到写锁会立即返回 "database is locked"
//   - 默认 journal_mode(WAL)：读写互不阻塞，并发读不再阻塞流量统计、审计批量写
//   - 默认 _txlock=immediate：事务开始即取写锁，避免「先读后写」升级锁时 busy_timeout 无法化解的冲突
//
// 用户在 db_source 中显式配置的同名参数优先
func sqliteDSN(dsn string) string {
	if dsn == ":memory:" || strings.Contains(dsn, "mode=memory") {
		return dsn
	}
	base, query, _ := strings.Cut(dsn, "?")
	var params []string
	busy := "5000"
	for _, kv := range strings.Split(query, "&") {
		if kv == "" {
			continue
		}
		if v, ok := strings.CutPrefix(kv, "_busy_timeout="); ok {
			if v != "" {
				busy = v
			}
			continue
		}
		params = append(params, kv)
	}
	has := func(s string) bool {
		for _, p := range params {
			if strings.Contains(p, s) {
				return true
			}
		}
		return false
	}
	if !has("busy_timeout") {
		params = append(params, "_pragma=busy_timeout("+busy+")")
	}
	if !has("journal_mode") {
		params = append(params, "_pragma=journal_mode(WAL)")
	}
	if !has("_txlock=") {
		params = append(params, "_txlock=immediate")
	}
	return base + "?" + strings.Join(params, "&")
}
