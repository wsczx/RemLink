package authsrv

import "github.com/wsczx/remlink/auth"

// 全部内置认证器工厂的显式列表
// 注册由 RegisterProviders 在 app.Init 阶段执行（须早于配置解析）；测试需自行调用完成装配
var providers = []struct {
	Name string
	New  func() auth.Authenticator
}{
	{"admin", func() auth.Authenticator { return &AdminAuth{} }},
	{"cert", func() auth.Authenticator { return &CertAuth{} }},
	{"dingtalk", func() auth.Authenticator { return &DingTalkAuth{} }},
	{"feishu", func() auth.Authenticator { return &FeishuAuth{} }},
	{"forcepwd", func() auth.Authenticator { return &ForcePwd{} }},
	{"ldap", func() auth.Authenticator { return &LDAPAuth{} }},
	{"local", func() auth.Authenticator { return &LocalAuth{} }},
	{"otp", func() auth.Authenticator { return &OTPAuth{} }},
	{"radius", func() auth.Authenticator { return &RADIUSAuth{} }},
	{"sms", func() auth.Authenticator { return &SmsAuth{} }},
	{"wxwork", func() auth.Authenticator { return &WXWorkAuth{} }},
}

// 将全部内置认证器工厂注册到 r。已注册的会被跳过
func RegisterProviders(r *auth.ProviderRegistry) {
	for _, p := range providers {
		if r.IsRegistered(p.Name) {
			continue
		}
		r.Register(p.Name, p.New)
	}
}
