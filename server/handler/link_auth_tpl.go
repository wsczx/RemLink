// 认证模板：XML 模板、渲染器、RequestData、工具函数

package handler

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"text/template"

	"github.com/wsczx/remlink/auth/authsrv"
)

const (
	tpl_request = iota
	tpl_complete
	tpl_otp
	tpl_request_saml
	tpl_accept_challenge
	tpl_request_force_pwd
)

// RequestData 模板渲染数据
type RequestData struct {
	// 登录表单
	Groups []string
	Group  string
	Error  string

	// auth-complete
	SessionId    string
	SessionToken string
	Banner       string
	ProfileName  string
	ProfileHash  string
	CertHash     string

	// SAML SSO
	ServerAddr  string
	BrowserMode string
	SsoType     string // "wxwork" 或 "feishu"

	// 强制改密
	State string // 强制改密会话 state

	// SMS 短信验证码挑战：脱敏手机号（三端与 WebAuth/门户对齐）
	PhoneMasked string
}

// 根据模板类型渲染认证页面模板到 ResponseWriter
func tplRequest(typ int, w io.Writer, data RequestData) {
	switch typ {
	case tpl_request:
		t, _ := template.New("auth_request").Parse(auth_request)
		_ = t.Execute(w, data)
	case tpl_complete:
		if data.Banner != "" {
			buf := new(bytes.Buffer)
			_ = xml.EscapeText(buf, []byte(data.Banner))
			data.Banner = buf.String()
		}
		t, _ := template.New("auth_complete").Parse(auth_complete)
		_ = t.Execute(w, data)
	case tpl_otp:
		t, _ := template.New("auth_otp").Parse(auth_otp)
		_ = t.Execute(w, data)
	case tpl_request_saml:
		t, _ := template.New("auth_request_saml").Parse(auth_request_saml)
		_ = t.Execute(w, data)
	case tpl_accept_challenge:
		t, _ := template.New("accept_challenge").Parse(accept_challenge)
		_ = t.Execute(w, data)
	case tpl_request_force_pwd:
		t, _ := template.New("auth_request_force_pwd").Parse(auth_request_force_pwd)
		_ = t.Execute(w, data)
	}
}

var auth_request = `<?xml version="1.0" encoding="UTF-8"?>
<config-auth client="vpn" type="auth-request" aggregate-auth-version="2">
    <opaque is-for="sg">
        <tunnel-group>{{.Group}}</tunnel-group>
        <group-alias>{{.Group}}</group-alias>
        <aggauth-handle>168179266</aggauth-handle>
        <config-hash>1595829378234</config-hash>
        <auth-method>multiple-cert</auth-method>
        <auth-method>single-sign-on-v2</auth-method>
    </opaque>
    <auth id="main">
        <title>Login</title>
        <message>请输入你的用户名和密码</message>
        <banner></banner>
        {{if .Error}}
        <error id="88" param1="{{.Error}}" param2="">登陆失败: {{.Error}}</error>
        {{end}}
        <form>
            <input type="text" name="username" label="Username:"></input>
            <input type="password" name="password" label="Password:"></input>
            <select name="group_list" label="GROUP:">
                {{range $v := .Groups}}
                <option {{if eq $v $.Group}} selected="true"{{end}}>{{$v}}</option>
                {{end}}
            </select>
        </form>
    </auth>
</config-auth>
`

var auth_complete = `<?xml version="1.0" encoding="UTF-8"?>
<config-auth client="vpn" type="complete" aggregate-auth-version="2">
    <session-id>{{.SessionId}}</session-id>
    <session-token>{{.SessionToken}}</session-token>
    <auth id="success">
        <banner>{{.Banner}}</banner>
        <message id="0" param1="" param2=""></message>
    </auth>
    <capabilities>
        <crypto-supported>ssl-dhe</crypto-supported>
    </capabilities>
    <config client="vpn" type="private">
        <vpn-base-config>
            <server-cert-hash>{{.CertHash}}</server-cert-hash>
        </vpn-base-config>
        <opaque is-for="vpn-client"></opaque>
        <vpn-profile-manifest>
            <vpn rev="1.0">
                <file type="profile" service-type="user">
                    <uri>/{{.ProfileName}}.xml</uri>
                    <hash type="sha1">{{.ProfileHash}}</hash>
                </file>
            </vpn>
        </vpn-profile-manifest>
    </config>
</config-auth>
`

var auth_otp = `<?xml version="1.0" encoding="UTF-8"?>
<config-auth client="vpn" type="auth-request" aggregate-auth-version="2">
    <auth id="otp-verification">
        <title>OTP 动态码验证</title>
        <message>请输入您的 OTP 动态码</message>
        {{if .Error}}
        <error id="otp-verification" param1="{{.Error}}" param2="">验证失败:  %s</error>
        {{end}}		
        <form method="post" action="/otp-verification">
            <input type="password" name="secondary_password" label="OTPCode:"/>
        </form>
    </auth>
</config-auth>`

var auth_request_saml = `<?xml version="1.0" encoding="UTF-8"?>
<config-auth client="vpn" type="auth-request" aggregate-auth-version="2">
    <opaque is-for="sg">
        <tunnel-group>{{.Group}}</tunnel-group>
        <group-alias>{{.Group}}</group-alias>
        <aggauth-handle>168179266</aggauth-handle>
        <config-hash>1595829378234</config-hash>
        <auth-method>single-sign-on-v2</auth-method>
    </opaque>
    <auth id="main">
        <title>SAML SSO Login</title>
        <message>请完成SAML单点登录认证</message>
        <banner></banner>
        {{if .Error}}
        <error id="88" param1="{{.Error}}" param2="">SAML认证失败: {{.Error}}</error>
        {{end}}
        <sso-v2-login>{{.ServerAddr}}/+CSCOE+/saml/sp/login?tgname={{.Group | urlquery}}&#x26;ssotype={{.SsoType}}&#x26;acsamlcap=v2</sso-v2-login>
        <sso-v2-login-final>{{.ServerAddr}}/+CSCOE+/saml_ac_login.html</sso-v2-login-final>
        <sso-v2-token-cookie-name>acSamlv2Token</sso-v2-token-cookie-name>
        {{if .BrowserMode}}<sso-v2-browser-mode>{{.BrowserMode}}</sso-v2-browser-mode>{{end}}
        <form>
            <input type="sso" name="sso-token"></input>
            <select name="group_list" label="GROUP:">
            {{range $v := .Groups}}
            <option {{if eq $v $.Group}} selected="true"{{end}}>{{$v}}</option>
            {{end}}
            </select>
        </form>
    </auth>
</config-auth>`

var accept_challenge = `<?xml version="1.0" encoding="UTF-8"?>
<config-auth client="vpn" type="auth-request" aggregate-auth-version="2">
    <auth id="radius-challenge">
        <banner>RADIUS 二次验证</banner>
        <message>{{if .Error}}{{.Error}}{{else}}请输入二次验证码{{end}}{{if .PhoneMasked}}（已发送至 {{.PhoneMasked}}）{{end}}</message>
        <form method="post" action="/otp-verification">
            <input type="password" name="secondary_password" label="Response:"/>
        </form>
    </auth>
</config-auth>`

// ds_domains_xml 动态分流域名配置模板（由 link_tunnel.go 使用）
var ds_domains_xml = `
<?xml version="1.0" encoding="UTF-8"?>
<config-auth client="vpn" type="complete" aggregate-auth-version="2">
    <config client="vpn" type="private">
        <opaque is-for="vpn-client">
            <custom-attr>
            {{if .DsExcludeDomains}}
               <dynamic-split-exclude-domains><![CDATA[{{.DsExcludeDomains}},]]></dynamic-split-exclude-domains>
            {{else if .DsIncludeDomains}}
               <dynamic-split-include-domains><![CDATA[{{.DsIncludeDomains}}]]></dynamic-split-include-domains>
            {{end}}
            </custom-attr>
        </opaque>
    </config>
</config-auth>
`

// samlSuccessHTML SAML 认证成功页面
var samlSuccessHTML = `
<!DOCTYPE html>
<html>
<head>
    <title>认证成功 - 请关闭</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            background: #f5f5f5;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 15px rgba(0,0,0,0.2);
            padding: 40px;
            max-width: 450px;
            width: 100%;
            text-align: center;
        }
        .icon {
            font-size: 56px;
            color: #4CAF50; 
            margin-bottom: 24px;
        }
        h1 {
            color: #303133;
            font-size: 28px;
            margin-bottom: 15px;
        }
        .detail {
            color: #606266;
            font-size: 15px;
            margin-bottom: 25px;
        }
        .note {
            color: #909399;
            font-size: 13px;
            margin-top: 30px;
            border-top: 1px solid #e4e7ed;
            padding-top: 20px;
        }
    </style>
    <script>
        function tryCloseWindow() {
            window.close(); 
            setTimeout(function() {
                alert("如果窗口未关闭，请手动关闭此页面并返回您的客户端。");
            }, 50); 
        }
    </script>
</head>
<body>
    <div class="container">
        <div class="icon">✅</div>
        <h1>认证成功！</h1>
        <p class="detail">已成功完成认证，VPN客户端将自动完成连接。</p>
        <p class="detail">
            请返回 AnyConnect 客户端查看连接状态。
        </p>
        <div class="note">
            提示：由于浏览器安全限制，网页无法自动关闭非脚本打开的窗口。
            请手动关闭此页面以完成认证流程。
        </div>
    </div>
</body>
</html>`

// 获取当前服务地址
func getServerAddr(r *http.Request) string {
	return "https://" + r.Host
}

// 判断是否为移动设备
func isMobileDevice(r *http.Request) bool {
	userAgent := r.Header.Get("User-Agent")
	if userAgent == "" {
		return false
	}
	mobileKeywords := []string{"Mobile", "Android", "iPhone", "iPad", "iPod"}
	for _, keyword := range mobileKeywords {
		if strings.Contains(userAgent, keyword) {
			return true
		}
	}
	return false
}

// 手机端强制使用内置浏览器（外部浏览器的 localhost:29786 回调在手机上不可用）
func samlBrowserMode(r *http.Request, ssoType, groupName string) string {
	if isMobileDevice(r) {
		return "internal"
	}
	return authsrv.GetSSOBrowserMode(ssoType, groupName)
}

// 判断是否是 AnyConnect 内置浏览器
func isAnyConnectInternalBrowser(r *http.Request) bool {
	userAgent := r.Header.Get("User-Agent")
	if userAgent == "" {
		return false
	}
	return strings.Contains(userAgent, "AnyConnect")
}
