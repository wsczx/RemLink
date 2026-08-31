package dbdata

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/wsczx/remlink/pkg/security"
)

// 写入密钥文件并加载到内存。
func ImportEncryptionKey(hexKey string) error {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return fmt.Errorf("密钥格式无效（需16进制）: %w", err)
	}
	if len(key) != 32 {
		return fmt.Errorf("密钥长度不足 256 位")
	}
	if err := os.WriteFile(security.KeyFilePath(), []byte(hexKey), 0600); err != nil {
		return err
	}
	return security.LoadKey()
}

// 含 EncryptedString 字段的 Setting 类型
var settingFactories = []func() any{
	func() any { return &SettingSmtp{} },
	func() any { return &SettingSms{} },
	func() any { return &SettingTLSCert{} },
	func() any { return &SettingClientCA{} },
	func() any { return &SettingLetsEncrypt{} },
	func() any { return &LegoUserData{} },
}

// 检查数据库中是否有已加密的数据。
func HasEncryptedData() bool {
	certs, _ := certsEncrypted()
	if certs > 0 {
		return true
	}
	if slices.ContainsFunc(settingFactories, settingEncrypted) {
		return true
	}
	if raw, _, err := loadSettingRaw("SettingServerConfig"); err == nil && len(raw) > 0 &&
		strings.Contains(string(raw), security.Prefix()) {
		return true
	}
	return providersEncrypted()
}

// 批量加密库中所有敏感字段。
func EnableEncryption() (map[string]int, error) {
	if !security.IsEnabled() {
		return nil, fmt.Errorf("加密未启用")
	}
	stats := map[string]int{}

	for _, factory := range settingFactories {
		target := factory()
		if err := SettingGet(target); err != nil {
			if CheckErrNotFound(err) {
				continue
			}
			return stats, err
		}
		if err := SettingSave(target); err != nil {
			return stats, err
		}
		stats[StructName(target)] = 1
	}

	// 证书私钥 + Provider 配置用原始 SQL 绕过 Scan/Value
	n, _, err := migrateRawColumn("client_cert_data", "private_key", "ClientCertData", true)
	if err != nil {
		return stats, err
	}
	stats["ClientCertData"] = n

	n, _, err = migrateRawColumn("provider", "config", "Provider", true)
	if err != nil {
		return stats, err
	}
	stats["Provider"] = n

	// JwtSecret/AdminOtp 由 MarshalJSON 加密
	{
		sc := &SettingServerConfig{}
		if err := SettingGet(sc); err != nil {
			if !CheckErrNotFound(err) {
				return stats, fmt.Errorf("读取 SettingServerConfig 失败: %w", err)
			}
			// 记录不存在，跳过（全新安装尚未持久化配置）
		} else if err := saveServerConfig(sc); err != nil {
			return stats, fmt.Errorf("加密 SettingServerConfig 失败: %w", err)
		} else {
			stats["SettingServerConfig"] = 1 // 仅成功时计数
		}
	}

	return stats, nil
}

// 批量解密所有敏感字段并删除密钥。用原始 SQL 避开 Scan/Value 的解密回路。
func DisableEncryption() (map[string]int, []string, error) {
	if !security.IsEnabled() {
		return nil, nil, fmt.Errorf("加密未启用")
	}
	stats := map[string]int{}
	var warnings []string

	for _, factory := range settingFactories {
		target := factory()
		name := StructName(target)
		raw, rowId, err := loadSettingRaw(name)
		if err != nil {
			return stats, warnings, err
		}
		if len(raw) == 0 {
			continue
		}
		decrypted, err := security.DecryptJSON(raw)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s 解密失败: %v", name, err))
			continue
		}
		if err := saveSettingRow(rowId, decrypted); err != nil {
			return stats, warnings, err
		}
		stats[name] = 1
	}

	// 证书私钥 + Provider 配置
	n, w, err := migrateRawColumn("client_cert_data", "private_key", "ClientCertData", false)
	if err != nil {
		return stats, warnings, err
	}
	stats["ClientCertData"] = n
	warnings = append(warnings, w...)
	n, w, err = migrateRawColumn("provider", "config", "Provider", false)
	if err != nil {
		return stats, warnings, err
	}
	stats["Provider"] = n
	warnings = append(warnings, w...)
	if raw, srvId, srvErr := loadSettingRaw("SettingServerConfig"); srvErr == nil && len(raw) > 0 {
		dec, decErr := security.DecryptJSON(raw)
		if decErr != nil {
			warnings = append(warnings, fmt.Sprintf("SettingServerConfig 解密失败: %v", decErr))
		} else if err := saveSettingRow(srvId, dec); err != nil {
			return stats, warnings, err
		} else {
			stats["SettingServerConfig"] = 1
		}
	}

	if err := security.DeleteKey(); err != nil {
		return stats, warnings, fmt.Errorf("删除密钥文件失败: %w", err)
	}

	return stats, warnings, nil
}

func loadSettingRaw(name string) (json.RawMessage, int, error) {
	s := &Setting{}
	err := One("name", name, s)
	if err != nil {
		if CheckErrNotFound(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	return s.Data, s.Id, nil
}

func saveSettingRow(rowId int, data json.RawMessage) error {
	_, err := xdb.ID(rowId).Cols("Data").Update(&Setting{Id: rowId, Data: data})
	return err
}

func settingEncrypted(factory func() any) bool {
	raw, _, err := loadSettingRaw(StructName(factory()))
	if err != nil || len(raw) == 0 {
		return false
	}
	return strings.Contains(string(raw), security.Prefix())
}

func certsEncrypted() (int, error) {
	count, err := xdb.SQL("SELECT COUNT(*) FROM client_cert_data WHERE private_key LIKE ?", "%$AES256$%").Count()
	return int(count), err
}

func providersEncrypted() bool {
	total, err := xdb.SQL("SELECT COUNT(*) FROM provider WHERE config LIKE ?", "%$AES256$%").Count()
	return err == nil && total > 0
}

// 用原始 SQL 批量加/解密某表的某列，绕过 EncryptedString 的 Scan/Value 和 EncryptedJSON 的 FromDB/ToDB。
func migrateRawColumn(table, col, label string, encrypt bool) (int, []string, error) {
	rows, err := xdb.SQL(fmt.Sprintf("SELECT id, %s FROM %s", col, table)).QueryString()
	if err != nil {
		return 0, nil, fmt.Errorf("读取 %s 失败: %w", label, err)
	}
	var warnings []string
	count := 0
	for _, row := range rows {
		id := row["id"]
		val := row[col]
		if encrypt {
			if val == "" || val == "null" || val == "{}" || security.IsEncrypted(val) {
				continue
			}
			out, err := security.Encrypt(val)
			if err != nil {
				return count, warnings, fmt.Errorf("加密 %s[%s] 失败: %w", label, id, err)
			}
			if _, err := xdb.Exec(fmt.Sprintf("UPDATE %s SET %s = ? WHERE id = ?", table, col), out, id); err != nil {
				return count, warnings, fmt.Errorf("更新 %s %s 失败: %w", label, id, err)
			}
		} else {
			if val == "" || !security.IsEncrypted(val) {
				continue
			}
			out, err := security.Decrypt(val)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s[%s] 解密失败: %v", label, id, err))
				continue
			}
			if _, err := xdb.Exec(fmt.Sprintf("UPDATE %s SET %s = ? WHERE id = ?", table, col), out, id); err != nil {
				return count, warnings, fmt.Errorf("更新 %s %s 失败: %w", label, id, err)
			}
		}
		count++
	}
	return count, warnings, nil
}
