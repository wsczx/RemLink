package handler

import (
	"os"
	"strings"
)

// 读取 Linux sysctl 参数
func sysctlGet(key string) (string, error) {
	b, err := os.ReadFile("/proc/sys/" + strings.ReplaceAll(key, ".", "/"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// 写入 Linux sysctl 参数
func sysctlSet(key, value string) error {
	return os.WriteFile("/proc/sys/"+strings.ReplaceAll(key, ".", "/"), []byte(value), 0644)
}
