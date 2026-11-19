package dbdata

import "github.com/wsczx/remlink/pkg/notify"

// 把设置表中的通知配置适配为 notify.Loader
type NotifyLoader struct{}

func (NotifyLoader) Smtp() (*notify.SmtpConfig, error) {
	s := &SettingSmtp{}
	if err := Settings.Get(s); err != nil {
		return nil, err
	}
	return ToSmtpConfig(s), nil
}

func (NotifyLoader) Sms() (*notify.SmsConfig, error) {
	s := &SettingSms{}
	if err := Settings.Get(s); err != nil {
		return nil, err
	}
	return ToSmsConfig(s), nil
}

// 把存储结构转换为通知包配置
func ToSmtpConfig(s *SettingSmtp) *notify.SmtpConfig {
	return &notify.SmtpConfig{
		Host:       s.Host,
		Port:       s.Port,
		Username:   s.Username,
		Password:   string(s.Password),
		From:       s.From,
		Encryption: s.Encryption,
	}
}

// 把存储结构转换为通知包配置
func ToSmsConfig(s *SettingSms) *notify.SmsConfig {
	return &notify.SmsConfig{
		Provider:           s.Provider,
		AliAccessKeyId:     s.AliAccessKeyId,
		AliAccessKeySecret: string(s.AliAccessKeySecret),
		AliSignName:        s.AliSignName,
		AliTemplateCode:    s.AliTemplateCode,
		TencentSecretId:    s.TencentSecretId,
		TencentSecretKey:   string(s.TencentSecretKey),
		TencentSdkAppId:    s.TencentSdkAppId,
		TencentSignName:    s.TencentSignName,
		TencentTemplateId:  s.TencentTemplateId,
		TencentRegion:      s.TencentRegion,
	}
}

var _ notify.Loader = NotifyLoader{}
