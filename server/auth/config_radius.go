// RADIUS Provider 配置类型

package auth

import (
	"fmt"
)

type RADIUSConfig struct {
	Addr   string `json:"addr"`
	Secret string `json:"secret"`
	NasIP  string `json:"nasip"`
}

func (c *RADIUSConfig) ValidateConfig() error {
	if !ipPortRe.MatchString(c.Addr) {
		return fmt.Errorf("RADIUS 服务器地址格式有误")
	}
	if len(c.Secret) < 8 || len(c.Secret) > 200 {
		return fmt.Errorf("RADIUS 密钥长度需在 8～200 之间")
	}
	return nil
}
