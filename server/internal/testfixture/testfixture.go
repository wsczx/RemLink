package testfixture

import (
	"path"
	"testing"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

// 初始化一个独立的临时 sqlite 测试库并启动数据库，注册 t.Cleanup 在用例
// 结束时停止。每个用例使用独立 TempDir，互不污染
// 配置快照恢复、异步日志 worker 排空、吊销阈值复位
//
// 注意： 改写全局配置与全局 DB（base cfg / dbdata.xdb / 各包级
// 单例），禁止在调用方中使用 t.Parallel()；同包内测试应串行执行
func SetupDB(t *testing.T) {
	t.Helper()
	// 配置快照：用例内的 UpdateCfg 改动只在本用例生效，避免向后泄漏
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
		// 先排空异步日志 worker 再关库，避免残留 worker 读已关闭的库
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
