// notify 统一通知包，支持邮件、短信、企业微信、钉钉、飞书、Webhook 等通道。
package notify

import (
	"fmt"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	mail "github.com/xhit/go-simple-mail/v2"
)

// Message 统一消息结构
type Message struct {
	Subject    string            // 邮件主题（短信/其他通道忽略）
	To         string            // 接收人：邮箱 或 手机号
	Body       string            // 正文（邮件支持 HTML，短信为纯文本）
	Params     map[string]string // 短信模板变量
	Attachment *mail.File        // 邮件附件（仅邮件通道使用）
}

// Dispatcher 统一通知调度器
type Dispatcher struct{}

var defaultDispatcher = &Dispatcher{}

// 获取全局通知调度器
func GetNotify() *Dispatcher {
	return defaultDispatcher
}

// 发送邮件
func (d *Dispatcher) SendEmail(msg Message) error {
	smtpCfg := &dbdata.SettingSmtp{}
	if err := dbdata.SettingGet(smtpCfg); err != nil || smtpCfg.Host == "" {
		return fmt.Errorf("SMTP未配置")
	}
	return sendEmail(smtpCfg, msg)
}

// 发送短信
func (d *Dispatcher) SendSms(msg Message) error {
	smsCfg := &dbdata.SettingSms{}
	if err := dbdata.SettingGet(smsCfg); err != nil {
		return fmt.Errorf("短信配置未找到: %w", err)
	}
	sender, err := newSmsSender(smsCfg)
	if err != nil {
		return err
	}
	return sender.Send(msg.To, msg.Params)
}

// 检查邮件是否已配置
func IsEmailConfigured() bool {
	smtpCfg := &dbdata.SettingSmtp{}
	if err := dbdata.SettingGet(smtpCfg); err != nil {
		return false
	}
	return smtpCfg.Host != ""
}

// 检查短信是否已配置
func IsSmsConfigured() bool {
	smsCfg := &dbdata.SettingSms{}
	if err := dbdata.SettingGet(smsCfg); err != nil {
		return false
	}
	return smsCfg.Provider != ""
}

// 测试邮件发送
func SendEmailTest(smtpCfg *dbdata.SettingSmtp, to string) error {
	msg := Message{
		Subject: fmt.Sprintf("[%s] 邮件发送测试", base.GetCfg().Issuer),
		To:      to,
		Body:    "<p>这是一封测试邮件，如果您收到此邮件，说明 SMTP 配置正确。</p>",
	}
	return sendEmail(smtpCfg, msg)
}

// 测试短信发送（不依赖全局 Dispatcher，可指定配置）
func SendSmsTest(smsCfg *dbdata.SettingSms, phone string) error {
	sender, err := newSmsSender(smsCfg)
	if err != nil {
		return err
	}
	return sender.Send(phone, map[string]string{"1": "123456", "2": "5"})
}
