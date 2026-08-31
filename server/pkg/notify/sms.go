package notify

import (
	"fmt"

	"github.com/wsczx/remlink/dbdata"
)

// SmsSender 短信发送接口，各厂商实现此接口
type SmsSender interface {
	// Send 发送短信
	// phone: 目标手机号
	// params: 模板变量，key 为模板占位符索引如 "1"/"2"，value 为替换值
	Send(phone string, params map[string]string) error
}

// 根据配置创建短信发送器
func newSmsSender(smsCfg *dbdata.SettingSms) (SmsSender, error) {
	switch smsCfg.Provider {
	case "aliyun":
		return newAliyunSender(smsCfg), nil
	case "tencent":
		return newTencentSender(smsCfg), nil
	case "":
		return nil, fmt.Errorf("短信服务未启用")
	default:
		return nil, fmt.Errorf("不支持的短信服务商: %s", smsCfg.Provider)
	}
}
