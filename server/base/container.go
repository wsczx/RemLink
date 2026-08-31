package base

import (
	"os"
)

const (
	inContainerKey = "REMLINK_IN_CONTAINER"
)

var InContainer = false

// 读取容器环境标记（防火墙 NAT 规则下发时据此判断是否需要额外 MASQUERADE/链策略）
func initContainer() {
	if os.Getenv(inContainerKey) == "on" {
		InContainer = true
	}
	Debug("InContainer", InContainer)
}
