// notify 统一通知包，支持邮件、短信等通道
package notify

import (
	"fmt"

	"github.com/wsczx/remlink/base"
	mail "github.com/xhit/go-simple-mail/v2"
)

// 统一消息结构
type Message struct {
	Subject    string            // 邮件主题（短信通道忽略）
	To         string            // 接收人：邮箱 或 手机号
	Body       string            // 正文（邮件支持 HTML，短信为纯文本）
	Params     map[string]string // 短信模板变量
	Attachment *mail.File        // 邮件附件
}

// 发送邮件
func SendEmail(msg Message) error {
	smtpCfg, err := loadSmtp()
	if err != nil || smtpCfg == nil || smtpCfg.Host == "" {
		return fmt.Errorf("SMTP未配置")
	}
	return sendEmail(smtpCfg, msg)
}

// 发送短信
func SendSms(msg Message) error {
	smsCfg, err := loadSms()
	if err != nil {
		return fmt.Errorf("短信配置未找到: %w", err)
	}
	if smsCfg == nil {
		return fmt.Errorf("短信配置未找到")
	}
	sender, err := newSmsSender(smsCfg)
	if err != nil {
		return err
	}
	return sender.Send(msg.To, msg.Params)
}

// 检查邮件是否已配置
func IsEmailConfigured() bool {
	cfg, err := loadSmtp()
	return err == nil && cfg != nil && cfg.Host != ""
}

// 检查短信是否已配置
func IsSmsConfigured() bool {
	cfg, err := loadSms()
	return err == nil && cfg != nil && cfg.Provider != ""
}

// 测试邮件发送
func SendEmailTest(smtpCfg *SmtpConfig, to string) error {
	msg := Message{
		Subject: fmt.Sprintf("[%s] 邮件发送测试", base.GetCfg().Issuer),
		To:      to,
		Body:    "<p>这是一封测试邮件，如果您收到此邮件，说明 SMTP 配置正确。</p>",
	}
	return sendEmail(smtpCfg, msg)
}

// 测试短信发送
func SendSmsTest(smsCfg *SmsConfig, phone string) error {
	sender, err := newSmsSender(smsCfg)
	if err != nil {
		return err
	}
	return sender.Send(phone, map[string]string{"1": "123456", "2": "5"})
}
