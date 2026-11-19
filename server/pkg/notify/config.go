package notify

import "errors"

// 邮件服务器配置
type SmtpConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string
	From       string
	Encryption string
}

// 短信服务配置
type SmsConfig struct {
	Provider string

	AliAccessKeyId     string
	AliAccessKeySecret string
	AliSignName        string
	AliTemplateCode    string

	TencentSecretId   string
	TencentSecretKey  string
	TencentSdkAppId   string
	TencentSignName   string
	TencentTemplateId string
	TencentRegion     string
}

// 提供通知配置，由应用层注入以隔离存储实现
type Loader interface {
	Smtp() (*SmtpConfig, error)
	Sms() (*SmsConfig, error)
}

var loader Loader

var errNoLoader = errors.New("通知配置加载器未注入")

// 注入配置加载器
func SetLoader(l Loader) { loader = l }

func loadSmtp() (*SmtpConfig, error) {
	if loader == nil {
		return nil, errNoLoader
	}
	return loader.Smtp()
}

func loadSms() (*SmsConfig, error) {
	if loader == nil {
		return nil, errNoLoader
	}
	return loader.Sms()
}
