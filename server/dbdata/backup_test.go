package dbdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/base"
)

// 清理测试产生的备份文件和 db.json。
func cleanupBackupFiles(t *testing.T) {
	t.Helper()
	_ = os.RemoveAll(filepath.Join("conf", "backup"))
	_ = os.Remove(filepath.Join("conf", "db.json"))
}

func TestAllTableNames(t *testing.T) {
	ast := assert.New(t)

	names := GetBackupManager().AllTableNames()
	ast.Len(names, len(GetBackupManager().Tables))

	for i := 1; i < len(names); i++ {
		ast.True(names[i-1] < names[i], "names should be sorted: %v", names)
	}

	for _, want := range []string{"user", "group", "setting", "policy", "provider"} {
		ast.Contains(names, want)
	}
}

func TestGetTableSizes(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	base.Test()
	defer closeIpdata()
	defer cleanupBackupFiles(t)

	sizes := GetBackupManager().GetTableSizes()
	ast.Len(sizes, len(GetBackupManager().Tables))

	groups := map[string]int{}
	for _, s := range sizes {
		ast.NotEmpty(s.Table)
		ast.NotEmpty(s.Name)
		ast.NotEmpty(s.Group)
		groups[s.Group]++
	}
	ast.Equal(10, groups["business"])
	ast.Equal(4, groups["log"])
	ast.Equal(4, groups["stats"])
}

func TestAllBusinessTableNames(t *testing.T) {
	ast := assert.New(t)

	names := GetBackupManager().BusinessTableNames()
	for _, t := range GetBackupManager().Tables {
		if t.Group == "business" {
			ast.Contains(names, t.Name)
		} else {
			ast.NotContains(names, t.Name)
		}
	}
}

func TestTableModels_MatchesBackupTables(t *testing.T) {
	ast := assert.New(t)

	// 旧校验仅比对数量，无法发现"新增模型漏注册备份表"或"备份表指向错误模型类型"。
	// 改为按具体模型类型集合比对，确保 TableModels 与 backupTables 完全一一对应。
	models := TableModels()
	modelTypes := make(map[reflect.Type]bool, len(models))
	for _, m := range models {
		modelTypes[reflect.TypeOf(m).Elem()] = true
	}
	backupTypes := make(map[reflect.Type]bool, len(GetBackupManager().Tables))
	for _, bt := range GetBackupManager().Tables {
		backupTypes[reflect.TypeOf(bt.Model)] = true
	}

	ast.Equal(len(modelTypes), len(backupTypes), "TableModels 与 backupTables 的模型类型数量应一致")
	for typ := range modelTypes {
		ast.True(backupTypes[typ], "TableModels 中的模型 %s 未在 backupTables 注册（该表将不会被备份/还原）", typ.Name())
	}
	for typ := range backupTypes {
		ast.True(modelTypes[typ], "backupTables 中的模型 %s 未在 TableModels 注册（Sync2 不会建该表）", typ.Name())
	}
}

func TestBackupTable_NewSlicePtr_NewPtr(t *testing.T) {
	ast := assert.New(t)

	for _, bt := range GetBackupManager().Tables {
		sp := bt.newSlicePtr()
		ast.NotNil(sp, "table %s newSlicePtr returned nil", bt.Name)

		np := bt.newPtr()
		ast.NotNil(np, "table %s newPtr returned nil", bt.Name)
	}
}

// ========== 备份文件校验 ==========

func TestBackupFileRead_InvalidFilename(t *testing.T) {
	ast := assert.New(t)

	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{"路径穿越", "../etc/passwd", true},
		{"绝对路径", "/tmp/remlink_backup_config_test.json", true},
		{"无前缀", "somefile.json", true},
		{"点号", ".", true},
		{"双点号", "..", true},
		{"空格", " ", true},
		{"空字符串", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetBackupManager().NewImporter(xdb).ReadFile(tt.filename)
			if tt.wantErr {
				ast.Error(err, "expected error for %q", tt.filename)
			} else {
				ast.NoError(err)
			}
		})
	}
}

func TestDeleteBackup_InvalidFilename(t *testing.T) {
	ast := assert.New(t)
	defer cleanupBackupFiles(t)

	err := GetBackupManager().DeleteBackup("../etc/passwd")
	ast.Error(err)
	ast.Contains(err.Error(), "invalid")

	err = GetBackupManager().DeleteBackup("bad.txt")
	ast.Error(err)
	ast.Contains(err.Error(), "invalid")
}

// ========== 备份创建 ==========

func TestCreateBackup_Config(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	base.Test()
	defer closeIpdata()
	defer cleanupBackupFiles(t)

	filename, err := GetBackupManager().Exporter().Create("config", nil)
	require.NoError(t, err)
	ast.Contains(filename, "remlink_backup_config_")
	ast.True(strings.HasSuffix(filename, ".json"))

	filePath := filepath.Join(backupFileDir, filename)
	raw, err := os.ReadFile(filePath)
	require.NoError(t, err)

	var data BackupData
	ast.NoError(json.Unmarshal(raw, &data))
	ast.Equal(currentBackupVersion, data.Version)
	ast.Equal("config", data.Type)
	ast.NotNil(data.Config)
	ast.Empty(data.Config.JwtSecret) // 敏感字段已清空
	ast.Empty(data.Config.AdminPass)
	ast.Empty(data.Config.AdminOtp)
	ast.Nil(data.Tables) // config 备份无表数据
}

func TestCreateBackup_Full(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	base.Test()
	defer closeIpdata()
	defer cleanupBackupFiles(t)

	// 先写入测试数据
	u := User{Username: "backup_test_user", Nickname: "测试用户"}
	_ = Add(&u)

	filename, err := GetBackupManager().Exporter().Create("full", nil)
	require.NoError(t, err)
	ast.Contains(filename, "remlink_backup_full_")

	filePath := filepath.Join(backupFileDir, filename)
	raw, err := os.ReadFile(filePath)
	require.NoError(t, err)

	var data BackupData
	ast.NoError(json.Unmarshal(raw, &data))
	ast.Equal("full", data.Type)
	ast.NotNil(data.Config)
	ast.NotNil(data.Tables)

	// 默认包含所有 business 表
	businessNames := GetBackupManager().BusinessTableNames()
	for _, n := range businessNames {
		ast.Contains(data.Tables, n, "business table %s should be in backup", n)
	}
	// user 表应有数据
	ast.NotEmpty(data.Tables["user"])
}

func TestCreateBackup_FullWithIncludeTables(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	base.Test()
	defer closeIpdata()
	defer cleanupBackupFiles(t)

	// 只备份 user 和 group 两张表
	filename, err := GetBackupManager().Exporter().Create("full", []string{"user", "group"})
	require.NoError(t, err)

	filePath := filepath.Join(backupFileDir, filename)
	raw, err := os.ReadFile(filePath)
	require.NoError(t, err)

	var data BackupData
	ast.NoError(json.Unmarshal(raw, &data))
	ast.Len(data.Tables, 2)
	ast.Contains(data.Tables, "user")
	ast.Contains(data.Tables, "group")
	ast.NotContains(data.Tables, "setting")
}

func TestCreateBackup_InvalidType(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	base.Test()
	defer closeIpdata()
	defer cleanupBackupFiles(t)

	_, err := GetBackupManager().Exporter().Create("unknown", nil)
	// 不会报错，但 tag 会设为 "config"
	ast.NoError(err)
}

// ========== 备份列表 / 删除 ==========

func TestListBackups_Empty(t *testing.T) {
	ast := assert.New(t)
	defer cleanupBackupFiles(t)

	list, err := GetBackupManager().ListBackups()
	ast.NoError(err)
	// 目录刚清理，可能为空或只含残留
	for _, f := range list {
		ast.True(strings.HasPrefix(f.Name, "remlink_backup_"))
	}
}

func TestListAndDeleteBackup(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	base.Test()
	defer closeIpdata()
	defer cleanupBackupFiles(t)

	filename, err := GetBackupManager().Exporter().Create("config", nil)
	require.NoError(t, err)

	// 列表
	list, err := GetBackupManager().ListBackups()
	require.NoError(t, err)
	ast.GreaterOrEqual(len(list), 1)

	found := false
	for _, f := range list {
		if f.Name == filename {
			found = true
			ast.Equal("config", f.Type)
			ast.Greater(f.Size, int64(0))
		}
	}
	ast.True(found, "backup file should appear in list")

	// 删除
	ast.NoError(GetBackupManager().DeleteBackup(filename))

	// 确认已删除
	list, err = GetBackupManager().ListBackups()
	require.NoError(t, err)
	for _, f := range list {
		ast.NotEqual(f.Name, filename)
	}
}

// ========== 备份还原完整链路 ==========

func TestBackup_RoundTrip(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	base.Test()
	defer closeIpdata()
	defer cleanupBackupFiles(t)

	// 先创建测试策略（group 依赖 policy）
	p := &Policy{Name: "rt_backup_policy", ClientDns: []ValData{{Val: "114.114.114.114"}}, Status: 1}
	require.NoError(t, SetPolicy(p))

	// 1. 写入测试数据
	users := []User{
		{Username: "rt_user1", Nickname: "还原测试1"},
		{Username: "rt_user2", Nickname: "还原测试2"},
	}
	for i := range users {
		require.NoError(t, Add(&users[i]))
		ast.NotZero(users[i].Id)
	}

	g := Group{Name: "rt_group", PolicyId: p.Id, Status: 1}
	require.NoError(t, SetGroup(&g))
	ast.NotZero(g.Id)

	// 2. 备份
	filename, err := GetBackupManager().Exporter().Create("full", []string{"user", "group"})
	require.NoError(t, err)

	// 记录原始 ID 用于验证
	userIds := []int{users[0].Id, users[1].Id}
	groupId := g.Id

	// 3. 清空表数据
	sess := xdb.NewSession()
	defer sess.Close()
	require.NoError(t, sess.Begin())
	require.NoError(t, GetBackupManager().NewImporter(xdb).ClearTable(sess, "user"))
	require.NoError(t, GetBackupManager().NewImporter(xdb).ClearTable(sess, "group"))
	require.NoError(t, sess.Commit())

	ast.Zero(GetBackupManager().TableCount("user"))
	ast.Zero(GetBackupManager().TableCount("group"))

	// 4. 还原（不使用 RestoreBackup，因为会覆盖配置；直接在 session 中操作）
	data, err := GetBackupManager().NewImporter(xdb).ReadFile(filename)
	require.NoError(t, err)

	sess2 := xdb.NewSession()
	defer sess2.Close()
	require.NoError(t, sess2.Begin())
	// 只还原业务表，不还原 config
	for name, raw := range data.Tables {
		bt, ok := GetBackupManager().TableByName[name]
		if !ok {
			continue
		}
		exist, _ := sess2.IsTableExist(bt.newPtr())
		if exist {
			require.NoError(t, GetBackupManager().NewImporter(xdb).ClearTable(sess2, name))
			require.NoError(t, GetBackupManager().NewImporter(xdb).ImportTable(sess2, name, raw))
		}
	}
	require.NoError(t, sess2.Commit())

	// 5. 验证数据已还原
	ast.Equal(int64(2), GetBackupManager().TableCount("user"))
	ast.Equal(int64(1), GetBackupManager().TableCount("group"))

	var restoredUsers []User
	ast.NoError(xdb.Where("id IN (?, ?)", userIds[0], userIds[1]).Find(&restoredUsers))
	ast.Len(restoredUsers, 2)
	for _, u := range restoredUsers {
		ast.True(u.Username == "rt_user1" || u.Username == "rt_user2")
	}

	var restoredGroup Group
	_, err = xdb.Where("id = ?", groupId).Get(&restoredGroup)
	ast.NoError(err)
	ast.Equal("rt_group", restoredGroup.Name)
}

func TestRestoreBackup_NoTables(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	base.Test()
	defer closeIpdata()
	defer cleanupBackupFiles(t)

	// config 备份复原不应报错
	filename, err := GetBackupManager().Exporter().Create("config", nil)
	require.NoError(t, err)

	// 不调用 RestoreBackup 因为会重置配置，只测试数据路径
	data, err := GetBackupManager().NewImporter(xdb).ReadFile(filename)
	require.NoError(t, err)
	ast.Nil(data.Tables)

	sess := xdb.NewSession()
	defer sess.Close()
	cfg, err := GetBackupManager().NewImporter(xdb).restoreToSession(sess, data)
	ast.NoError(err)
	// config 备份应该返回配置信息
	ast.NotNil(cfg)
}

func TestBackupData_JsonRoundTrip(t *testing.T) {
	ast := assert.New(t)

	original := BackupData{
		Version:   currentBackupVersion,
		Type:      "full",
		CreatedAt: "2026-01-01T00:00:00Z",
		DbType:    "sqlite3",
		DbSource:  ":memory:",
		Tables: map[string]json.RawMessage{
			"user": json.RawMessage(`[{"id":1,"username":"test"}]`),
		},
	}

	// 序列化
	b, err := json.Marshal(original)
	require.NoError(t, err)

	// 反序列化
	var restored BackupData
	require.NoError(t, json.Unmarshal(b, &restored))

	ast.Equal(original.Version, restored.Version)
	ast.Equal(original.Type, restored.Type)
	ast.Equal(original.CreatedAt, restored.CreatedAt)
	ast.Len(restored.Tables, 1)

	// json.RawMessage 内容一致性
	ast.JSONEq(string(original.Tables["user"]), string(restored.Tables["user"]))
}

func TestSaveDbConfig(t *testing.T) {
	ast := assert.New(t)
	defer cleanupBackupFiles(t)

	require.NoError(t, GetBackupManager().SaveDbConfig("sqlite3", "/tmp/test.db"))

	b, err := os.ReadFile(filepath.Join("conf", "db.json"))
	require.NoError(t, err)

	var cfg dbConfig
	require.NoError(t, json.Unmarshal(b, &cfg))
	ast.Equal("sqlite3", cfg.DbType)
	ast.Equal("/tmp/test.db", cfg.DbSource)
}

func TestBackupData_VersionMismatch(t *testing.T) {
	ast := assert.New(t)
	base.Test()
	defer cleanupBackupFiles(t)

	badVersion := BackupData{
		Version:   999,
		Type:      "config",
		CreatedAt: "2026-01-01T00:00:00Z",
	}

	b, err := json.MarshalIndent(badVersion, "", "  ")
	require.NoError(t, err)

	base.CreateDir(backupFileDir)
	filename := "remlink_backup_config_bad_version.json"
	filePath := filepath.Join(backupFileDir, filename)
	require.NoError(t, os.WriteFile(filePath, b, 0600))

	// 读取应该报版本不兼容
	_, err = GetBackupManager().NewImporter(xdb).ReadFile(filename)
	ast.Error(err)
	ast.Contains(err.Error(), "版本不兼容")
}
