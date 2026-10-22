package dbtest

import (
	"path"
	"testing"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

// 为每个用例初始化独立临时 sqlite 库，t.Cleanup 自动收尾
// 注意：改写全局配置与 DB，禁止调用方使用 t.Parallel()
func SetupDB(t *testing.T) {
	t.Helper()
	snapshot := *base.GetCfg()
	t.Cleanup(func() { base.SetCfgForTest(&snapshot) })

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.DbType = "sqlite3"
		c.DbSource = path.Join(t.TempDir(), "remlink_test.db")
		c.WebVpnDomain = "wv.example.com"
		c.JwtSecret = "unit-test-secret"
	})
	base.ReinitLog() // 初始化日志器，避免 base.Info/Error 在测试中 nil panic
	ResetRevocations()
	dbdata.Start()
	t.Cleanup(func() {
		// 先排空 worker 再关库，避免残留 worker 读已关闭的库
		dbdata.UserActLogIns.Pool.Release()
		dbdata.UserActLogIns.Pool = utils.NewWorkerPool(1, 100)
		// 用例可能抬高整用户吊销阈值，复位避免影响后续用例
		dbdata.WebVpnRevokeReset()
		_ = dbdata.Stop()
	})
}

// 重置包级全局的 WebVPN 整用户吊销阈值，避免跨用例副作用
func ResetRevocations() {
	dbdata.WebVpnRevokeReset()
}
