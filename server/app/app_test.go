package app

import (
	"embed"
	"encoding/json"
	"os"
	"testing"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

func TestMain(m *testing.M) {
	code := m.Run()
	_ = os.RemoveAll("conf")
	os.Exit(code)
}

func TestNewHoldsUI(t *testing.T) {
	a := New(embed.FS{})
	if a == nil {
		t.Fatal("New 应返回非 nil")
	}
	if a.DB != nil {
		t.Error("未 OpenStore 时 Store 应为 nil")
	}
}

func TestInitSetsVersion(t *testing.T) {
	base.Test()
	origVer, origCommit, origDate := base.APP_VER, base.CommitId, base.BuildDate
	defer base.SetVersion(origVer, origCommit, origDate)

	a := New(embed.FS{})
	a.Init("9.9.9", "testcommit", "2026-11-06")

	if base.APP_VER != "9.9.9" {
		t.Errorf("APP_VER = %q, want 9.9.9", base.APP_VER)
	}
	if base.CommitId != "testcommit" {
		t.Errorf("CommitId = %q, want testcommit", base.CommitId)
	}
	if base.BuildDate != "2026-11-06" {
		t.Errorf("BuildDate = %q, want 2026-11-06", base.BuildDate)
	}
}

// OpenStore 早于 Start 且会解析组配置；注册表须在 Init 阶段完成注册，
// 否则 SSO 与凭据组合的冲突校验会因空表而静默失效
func TestInitRegistersProvidersBeforeConfigParse(t *testing.T) {
	base.Test()
	a := New(embed.FS{})
	a.Init("9.9.9", "testcommit", "2026-11-06")

	if !auth.Registry.IsSSOType("wxwork") {
		t.Fatal("Init 后 SSO 认证器未注册，配置解析期的类型判定会失效")
	}
	_, err := auth.ParseAuthProfile(json.RawMessage(`{"step":[{"type":"wxwork"},{"type":"local"}]}`))
	if err == nil {
		t.Fatal("SSO 与凭据组合应被拒绝")
	}
}

func TestSetDBInjectsStore(t *testing.T) {
	base.Test()
	// SetDB 会改写 dbdata 的全局 store 与聚合绑定，用例结束后恢复，避免污染同进程后续用例
	orig := dbdata.GetDB()
	t.Cleanup(func() { dbdata.SetDB(orig) })

	a := New(embed.FS{})
	s := &dbdata.DB{}
	a.SetDB(s)
	if a.DB != s {
		t.Error("SetDB 未写入 App.Store")
	}
}
