package dbdata

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/base"
)

func runInitDataTx(t *testing.T) base.ServerConfig {
	t.Helper()

	sess := GetDB().Engine().NewSession()
	defer sess.Close()
	require.NoError(t, sess.Begin())
	cfg, err := addInitDataTx(sess)
	require.NoError(t, err)
	require.NoError(t, sess.Commit())
	return cfg
}

func TestInitDefaults_Idempotent(t *testing.T) {
	preIpData(t)
	defer closeIpdata()

	// 第一次：正常首装
	runInitDataTx(t)

	settingN := GetDB().Count(new(Setting))
	policyN := GetDB().Count(new(Policy))
	groupN := GetDB().Count(new(Group))
	require.Positive(t, settingN)
	require.Equal(t, 2, policyN)
	require.Equal(t, 2, groupN)

	// 第二次：模拟崩溃后重跑初始化
	runInitDataTx(t)

	// 重跑不应新增重复行
	require.Equal(t, settingN, GetDB().Count(new(Setting)))
	require.Equal(t, 2, GetDB().Count(new(Policy)))
	require.Equal(t, 2, GetDB().Count(new(Group)))

	// 重跑后内置组仍应正确引用到内置策略（PolicyId 未被写成 0）
	all := &Group{}
	require.NoError(t, Groups.GetByName("all", all))
	require.NotZero(t, all.PolicyId)
	ops := &Group{}
	require.NoError(t, Groups.GetByName("ops", ops))
	require.NotZero(t, ops.PolicyId)
}
func TestInitDefaultsKeepsExistingSettings(t *testing.T) {
	preIpData(t)
	defer closeIpdata()

	runInitDataTx(t)
	other := &SettingOther{Banner: "管理员自定义横幅"}
	require.NoError(t, Settings.Save(other))

	runInitDataTx(t)

	got := &SettingOther{}
	require.NoError(t, Settings.Get(got))
	require.Equal(t, "管理员自定义横幅", got.Banner)
}

func TestSeedGroupPreservesAdminFields(t *testing.T) {
	preIpData(t)
	defer closeIpdata()

	runInitDataTx(t)

	all := &Group{}
	require.NoError(t, Groups.GetByName("all", all))
	all.Note = "管理员备注"
	all.SplitDns = []ValData{{Val: "corp.internal", Note: "内网域名"}}
	all.Hidden = 1
	all.Status = 0
	require.NoError(t, Groups.SetGroup(all))

	runInitDataTx(t)

	got := &Group{}
	require.NoError(t, Groups.GetByName("all", got))
	require.Equal(t, "管理员备注", got.Note)
	require.Equal(t, []ValData{{Val: "corp.internal", Note: "内网域名"}}, got.SplitDns)
	require.Equal(t, int8(0), got.Hidden, "内置隐藏位应被校正为默认值")
	require.Equal(t, int8(1), got.Status, "内置组状态应被重置为启用")
	require.NotZero(t, got.PolicyId)
}

// 校验 initData 的决策：已完成初始化（标记 Installed=true）时直接跳过，
func TestInitData_SkipsWhenInstalled(t *testing.T) {
	preIpData(t)
	defer closeIpdata()

	require.NoError(t, Settings.Save(&SettingInstall{Installed: true}))
	require.NoError(t, initData())

	// 跳过初始化：未播种任何内置策略/组
	require.Zero(t, GetDB().Count(new(Policy)))
	require.Zero(t, GetDB().Count(new(Group)))
}

// 校验 Setting.SessUpsert
func TestSettingSessUpsert_Idempotent(t *testing.T) {
	preIpData(t)
	defer closeIpdata()

	sess := GetDB().Engine().NewSession()
	defer sess.Close()
	require.NoError(t, sess.Begin())
	require.NoError(t, Settings.SessUpsert(sess, &SettingAuditLog{LifeDay: 1, ClearTime: "01:00"}))
	require.NoError(t, Settings.SessUpsert(sess, &SettingAuditLog{LifeDay: 7, ClearTime: "05:00"}))
	require.NoError(t, sess.Commit())

	got := &SettingAuditLog{}
	require.NoError(t, Settings.Get(got))
	require.Equal(t, 7, got.LifeDay)
	require.Equal(t, "05:00", got.ClearTime)
	// 不应出现重复行
	require.Equal(t, 1, GetDB().CountWhere(new(Setting), "name = ?", "SettingAuditLog"))
}
