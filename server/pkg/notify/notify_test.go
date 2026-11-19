package notify

import (
	"errors"
	"testing"
)

type fakeLoader struct {
	smtp *SmtpConfig
	sms  *SmsConfig
	err  error
}

func (f *fakeLoader) Smtp() (*SmtpConfig, error) { return f.smtp, f.err }
func (f *fakeLoader) Sms() (*SmsConfig, error)   { return f.sms, f.err }

func TestSendEmailWithoutConfig(t *testing.T) {
	orig := loader
	defer func() { loader = orig }()

	SetLoader(&fakeLoader{smtp: &SmtpConfig{}})
	err := SendEmail(Message{To: "a@example.com", Body: "x"})
	if err == nil {
		t.Fatal("SMTP 未配置时应返回错误")
	}
}

func TestNilLoaderConfigsNoPanic(t *testing.T) {
	orig := loader
	defer func() { loader = orig }()

	SetLoader(&fakeLoader{})
	if IsEmailConfigured() || IsSmsConfigured() {
		t.Fatal("nil 配置不应视为已配置")
	}
	if err := SendEmail(Message{To: "a@example.com", Body: "x"}); err == nil {
		t.Fatal("nil SMTP 配置应返回错误")
	}
	if err := SendSms(Message{To: "13800000000", Params: map[string]string{"code": "1"}}); err == nil {
		t.Fatal("nil 短信配置应返回错误")
	}
}

func TestIsEmailConfigured(t *testing.T) {
	orig := loader
	defer func() { loader = orig }()

	SetLoader(&fakeLoader{smtp: &SmtpConfig{Host: "smtp.example.com"}})
	if !IsEmailConfigured() {
		t.Error("已配置 Host 时 IsEmailConfigured 应为 true")
	}

	SetLoader(&fakeLoader{smtp: &SmtpConfig{}})
	if IsEmailConfigured() {
		t.Error("未配置 Host 时 IsEmailConfigured 应为 false")
	}
}

func TestIsSmsConfigured(t *testing.T) {
	orig := loader
	defer func() { loader = orig }()

	SetLoader(&fakeLoader{sms: &SmsConfig{Provider: "aliyun"}})
	if !IsSmsConfigured() {
		t.Error("已配置 Provider 时 IsSmsConfigured 应为 true")
	}

	SetLoader(&fakeLoader{sms: &SmsConfig{}})
	if IsSmsConfigured() {
		t.Error("未配置 Provider 时 IsSmsConfigured 应为 false")
	}
}

func TestSendSmsWithoutProvider(t *testing.T) {
	orig := loader
	defer func() { loader = orig }()

	SetLoader(&fakeLoader{sms: &SmsConfig{}})
	if err := SendSms(Message{To: "13800000000"}); err == nil {
		t.Fatal("未配置短信通道时应返回错误")
	}
}

func TestLoaderErrorPropagates(t *testing.T) {
	orig := loader
	defer func() { loader = orig }()

	SetLoader(&fakeLoader{err: errors.New("存储不可用")})
	if IsEmailConfigured() {
		t.Error("加载出错时不应判定为已配置")
	}
}
