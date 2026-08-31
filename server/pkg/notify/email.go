package notify

import (
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	mail "github.com/xhit/go-simple-mail/v2"
)

// SMTP 邮件发送
func sendEmail(smtpCfg *dbdata.SettingSmtp, msg Message) error {
	server := mail.NewSMTPClient()

	server.Host = smtpCfg.Host
	server.Port = smtpCfg.Port
	server.Username = smtpCfg.Username
	server.Password = string(smtpCfg.Password)

	switch smtpCfg.Encryption {
	case "SSLTLS":
		server.Encryption = mail.EncryptionSSLTLS
	case "STARTTLS":
		server.Encryption = mail.EncryptionSTARTTLS
	default:
		server.Encryption = mail.EncryptionNone
	}

	server.Authentication = mail.AuthAuto
	server.KeepAlive = false
	server.ConnectTimeout = 10 * time.Second
	server.SendTimeout = 10 * time.Second

	smtpClient, err := server.Connect()
	if err != nil {
		base.Error(err)
		return err
	}

	email := mail.NewMSG()
	email.SetFrom(smtpCfg.From).
		AddTo(msg.To).
		SetSubject(msg.Subject)

	if msg.Attachment != nil {
		email.Attach(msg.Attachment)
	}

	email.SetBody(mail.TextHTML, msg.Body)

	err = email.Send(smtpClient)
	if err != nil {
		base.Error(err)
	}
	return err
}
