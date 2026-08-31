package security

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

const keyFileName = ".encryption_key"

var (
	globalKey  atomic.Pointer[[]byte]
	keyMu      sync.Mutex
	defaultDir string
)

// 设置密钥文件目录（仅启动时调用，非并发安全）。
func SetDir(dir string) { defaultDir = dir }

// 环境变量优先于默认路径：
// REMLINK_ENCRYPTION_KEY 指定完整路径，REMLINK_ENCRYPTION_KEY_DIR 指定目录。
func keyPath() string {
	if p := os.Getenv("REMLINK_ENCRYPTION_KEY"); p != "" {
		return p
	}
	if d := os.Getenv("REMLINK_ENCRYPTION_KEY_DIR"); d != "" {
		return filepath.Join(d, keyFileName)
	}
	return filepath.Join(defaultDir, keyFileName)
}

// 加密是否已启用（密钥已加载）。
func IsEnabled() bool { return globalKey.Load() != nil }

func getKeyLocked() []byte {
	p := globalKey.Load()
	if p == nil {
		return nil
	}
	return *p
}

// 生成 32 字节 AES-256 密钥并写入文件。
func GenerateKey() error {
	keyMu.Lock()
	defer keyMu.Unlock()

	if IsEnabled() {
		return errors.New("加密密钥已存在，请先关闭加密")
	}
	kp := keyPath()
	if _, err := os.Stat(kp); err == nil {
		return errors.New("密钥文件已存在: " + kp)
	}

	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return err
	}
	hexKey := hex.EncodeToString(key)
	if err := os.MkdirAll(filepath.Dir(kp), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(kp, []byte(hexKey), 0600); err != nil {
		return err
	}
	k := make([]byte, 32)
	copy(k, key)
	globalKey.Store(&k)
	return nil
}

// 从密钥文件加载密钥。文件不存在时返回 nil（加密未启用）。
func LoadKey() error {
	keyMu.Lock()
	defer keyMu.Unlock()

	kp := keyPath()
	info, err := os.Stat(kp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode().Perm()&0077 != 0 {
		return errors.New("密钥文件权限过高（需 0600）: " + kp)
	}
	data, err := os.ReadFile(kp)
	if err != nil {
		return err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return err
	}
	if len(key) != 32 {
		return errors.New("密钥长度不足 256 位")
	}
	k := make([]byte, 32)
	copy(k, key)
	globalKey.Store(&k)
	return nil
}

// 返回密钥文件路径。
func KeyFilePath() string { return keyPath() }

// 删除密钥文件并清空内存密钥。
func DeleteKey() error {
	keyMu.Lock()
	defer keyMu.Unlock()

	kp := keyPath()
	if err := os.Remove(kp); err != nil && !os.IsNotExist(err) {
		return err
	}
	globalKey.Store(nil)
	return nil
}
