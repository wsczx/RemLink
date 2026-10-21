package admin

import (
	"fmt"
	"net/http"

	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/security"
)

// 返回加密状态
func SecretStatus(w http.ResponseWriter, r *http.Request) {
	enabled := security.IsEnabled()
	dbEncrypted := dbdata.HasEncryptedData()
	RespSuccess(w, map[string]any{
		"enabled":      enabled,
		"db_encrypted": dbEncrypted,
		"key_path":     security.KeyFilePath(),
	})
}

// 生成密钥并加密全库敏感数据
// 密钥目录通过 REMLINK_ENCRYPTION_KEY_DIR 环境变量配置，默认保存在工作目录
func SecretEnable(w http.ResponseWriter, r *http.Request) {
	if security.IsEnabled() {
		RespError(w, RespParamErr, "加密已启用")
		return
	}

	restartMux.Lock()
	defer restartMux.Unlock()

	if err := security.GenerateKey(); err != nil {
		RespError(w, RespInternalErr, fmt.Errorf("生成密钥失败: %w", err))
		return
	}
	stats, err := dbdata.EnableEncryption()
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("安全设置", "加密", "启用了敏感字段加密", r.RemoteAddr)
	RespSuccess(w, map[string]any{
		"stats":    stats,
		"key_path": security.KeyFilePath(),
	})
}

// 上传密钥并加密全库敏感数据
func SecretUpload(w http.ResponseWriter, r *http.Request) {
	if security.IsEnabled() {
		RespError(w, RespParamErr, "加密已启用")
		return
	}

	var req struct {
		Key string `json:"key"` // 16进制密钥
	}
	if err := readJSON(w, r, 1<<16, &req); err != nil {
		return
	}
	if req.Key == "" {
		RespError(w, RespParamErr, "密钥不能为空")
		return
	}

	restartMux.Lock()
	defer restartMux.Unlock()

	if err := dbdata.ImportEncryptionKey(req.Key); err != nil {
		RespError(w, RespInternalErr, fmt.Errorf("导入密钥失败: %w", err))
		return
	}
	stats, err := dbdata.EnableEncryption()
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("安全设置", "加密", "上传密钥并启用了敏感字段加密", r.RemoteAddr)
	RespSuccess(w, stats)
}

// 解密全库敏感数据并删除密钥
func SecretDisable(w http.ResponseWriter, r *http.Request) {
	if !security.IsEnabled() {
		RespError(w, RespParamErr, "加密未启用")
		return
	}

	restartMux.Lock()
	defer restartMux.Unlock()

	stats, warnings, err := dbdata.DisableEncryption()
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("安全设置", "加密", "关闭了敏感字段加密", r.RemoteAddr)
	RespSuccess(w, map[string]any{
		"stats":    stats,
		"warnings": warnings,
	})
}
