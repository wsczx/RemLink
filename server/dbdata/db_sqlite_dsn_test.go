package dbdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSqliteDSN(t *testing.T) {
	ast := assert.New(t)
	ast.Equal("file:./conf/remlink.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate",
		sqliteDSN("file:./conf/remlink.db"))
	// 旧版 mattn 参数转换
	ast.Equal("file:a.db?cache=shared&_pragma=busy_timeout(8000)&_pragma=journal_mode(WAL)&_txlock=immediate",
		sqliteDSN("file:a.db?_busy_timeout=8000&cache=shared"))
	// 显式配置优先
	ast.Equal("file:a.db?_pragma=journal_mode(DELETE)&_txlock=deferred&_pragma=busy_timeout(5000)",
		sqliteDSN("file:a.db?_pragma=journal_mode(DELETE)&_txlock=deferred"))
	ast.Equal(":memory:", sqliteDSN(":memory:"))
}
