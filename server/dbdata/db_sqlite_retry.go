package dbdata

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
)

// SQLite 瞬时写锁竞争的重试次数与退避步长。busy_timeout 已让驱动在拿不到写锁时
// 等待最多 5 秒，这里只在仍超时（极端并发）时做应用层退避兜底
const (
	sqliteRetryAttempts = 4
	sqliteRetryBaseWait = 20 * time.Millisecond
)

// 判断错误是否为 SQLite 并发写锁竞争（瞬时错误，可重试）
// 这两类文案是 go-sqlite3 驱动特有，其它数据库的错误不会被误判
func isSqliteLocked(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database is busy")
}

// 包装 go-sqlite3 驱动，在连接层对 SQLite 锁错误做退避重试
type retrySqliteDriver struct {
	d *sqlite3.SQLiteDriver
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

// 对 Exec/Query 加锁重试；其余方法（含底层实现的可选接口）如有则透传，避免功能降级。
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
		d:   &retrySqliteDriver{d: &sqlite3.SQLiteDriver{}},
		dsn: dsn,
	}
	return sql.OpenDB(connector), nil
}
