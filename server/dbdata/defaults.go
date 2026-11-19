package dbdata

import (
	"errors"
	"net/http"
	"time"

	"github.com/wsczx/remlink/base"
	"xorm.io/xorm"
)

const accountMail = `<p>您好:</p>
<p>&nbsp;&nbsp;您的{{.Issuer}}账号已经审核开通。</p>
<p>
    登陆地址: <b>{{.LinkAddr}}</b> <br/>
    用户组: <b>{{.Group}}</b> <br/>
    用户名: <b>{{.Username}}</b> <br/>
    用户PIN码: <b>{{.PinCode}}</b> <br/>
    用户过期时间: <b>{{.LimitTime}}</b> <br/>
    {{if .DisableOtp}}
    <!-- nothing -->
    {{else}}
	
    <!-- 
    用户动态码(3天后失效):<br/>
    <img src="{{.OtpImg}}"/><br/>
    -->
    用户动态码(请妥善保存):<br/>
    <img src="cid:userOtpQr.png" alt="userOtpQr" /><br/>

    {{end}}
</p>
<div>
    使用说明:
    <ul>
        <li>请使用OTP软件扫描动态码二维码</li>
        <li>然后使用anyconnect客户端进行登陆</li>
        <li>登陆密码为 PIN 码</li>
		<li>OTP密码为扫码后生成的动态码</li>
    </ul>
</div>
<p>
    软件下载地址: https://{{.LinkAddr}}/files/info.txt
</p>`

const certMail = `<p>您好:</p>
<p>&nbsp;&nbsp;您的 VPN 客户端证书已生成，请查收附件。</p>
<p>
    用户名: <b>{{.Username}}</b> <br/>
    用户组: <b>{{.Groupname}}</b> <br/>
    序列号: <b>{{.SerialNumber}}</b> <br/>
    过期时间: <b>{{.NotAfter}}</b> <br/>
    <br/>
    P12 证书密码: <b>{{.Password}}</b> <br/>
</p>
<div>
    安装说明:
    <ul>
        <li>下载附件中的 .p12 证书文件</li>
        <li>双击安装，按提示输入上述密码</li>
        <li>在 AnyConnect 客户端选择此证书进行连接</li>
        <li>连接地址: https://{{.LinkAddr}}</li>
    </ul>
</div>
<p>如有疑问请联系管理员。</p>`

// 返回默认证书邮件模板，供管理后台获取初始值
func CertMailTemplate() string {
	return certMail
}

// 返回首次初始化写入的全部默认设置
func defaultSettings(cfg base.ServerConfig) []any {
	return []any{
		&SettingSmtp{
			Host:       "127.0.0.1",
			Port:       25,
			From:       "vpn@xx.com",
			Encryption: "None",
		},
		Settings.AuditLogDefault(),
		&SettingServerConfig{Config: cfg},
		&SettingProfile{Content: base.DefaultProfileXML},
		&SettingLetsEncrypt{
			Domain:      "vpn.xxx.com",
			Legomail:    "legomail",
			Name:        "aliyun",
			Renew:       false,
			DNSProvider: DNSProvider{},
		},
		&LegoUserData{},
		&SettingOther{
			LinkAddr:    "vpn.xx.com",
			Banner:      "您已接入公司网络，请按照公司规定使用。\n请勿进行非工作下载及视频行为！",
			Homecode:    http.StatusOK,
			Homeindex:   "RemLink 是一个企业级远程办公 sslvpn 的软件，可以支持多人同时在线使用。",
			AccountMail: accountMail,
			CertMail:    certMail,
		},
	}
}

// 在事务内写入内置的默认策略与用户组
// 同名记录已存在则跳过或校正关键字段，保证初始化可重跑
func seedPoliciesGroupsTx(sess *xorm.Session) error {
	pAll := &Policy{
		Name:         "全局代理",
		Note:         "系统初始化内置策略",
		AllowLan:     true,
		ClientDns:    []ValData{{Val: "114.114.114.114", Note: "默认dns"}},
		RouteInclude: []ValData{{Val: ALL}},
		Status:       1,
	}
	if err := seedPolicyTx(sess, pAll); err != nil {
		return err
	}

	pOps := &Policy{
		Name:         "仅内网分流",
		Note:         "系统初始化内置策略",
		AllowLan:     true,
		ClientDns:    []ValData{{Val: "114.114.114.114", Note: "默认dns"}},
		RouteInclude: []ValData{{Val: "10.0.0.0/8"}},
		Status:       1,
	}
	if err := seedPolicyTx(sess, pOps); err != nil {
		return err
	}

	for _, g := range []*Group{
		{Name: "all", PolicyId: pAll.Id, Status: 1},
		{Name: "ops", PolicyId: pOps.Id, Status: 1},
	} {
		if err := seedGroupTx(sess, g); err != nil {
			return err
		}
	}
	return nil
}

func seedPolicyTx(sess *xorm.Session, p *Policy) error {
	var ex Policy
	has, err := sess.Where("name = ?", p.Name).Get(&ex)
	if err != nil {
		return err
	}
	if has {
		p.Id = ex.Id
		return nil
	}
	if err := p.validateBasics(); err != nil {
		return err
	}
	isDefRoute, err := p.normalizeRoutes()
	if err != nil {
		return err
	}
	if err := p.normalizeAcl(); err != nil {
		return err
	}
	if err := p.normalizeDns(isDefRoute); err != nil {
		return err
	}
	if err := p.validateDomains(isDefRoute); err != nil {
		return err
	}
	p.UpdatedAt = time.Now()
	_, err = sess.Insert(p)
	return err
}

func seedGroupTx(sess *xorm.Session, g *Group) error {
	var ex Group
	has, err := sess.Where("name = ?", g.Name).Get(&ex)
	if err != nil {
		return err
	}
	target := g
	if has {
		if ex.PolicyId == g.PolicyId && ex.Status == g.Status && ex.Hidden == g.Hidden {
			return nil
		}
		ex.PolicyId = g.PolicyId
		ex.Status = g.Status
		ex.Hidden = g.Hidden
		target = &ex
	}

	if target.Name == "" {
		return errors.New("用户组名错误")
	}
	if target.Status == 1 && target.PolicyId <= 0 {
		return errors.New("启用的用户组必须指定策略")
	}
	if target.PolicyId > 0 {
		var policy Policy
		policyHas, err := sess.ID(target.PolicyId).Get(&policy)
		if err != nil {
			return err
		}
		if !policyHas || policy.Status != 1 {
			return errors.New("引用的策略不存在或已停用")
		}
	}
	if err := target.normalizeSplitDns(); err != nil {
		return err
	}
	if err := target.validateNetwork(); err != nil {
		return err
	}
	if err := target.validateAuthProfile(); err != nil {
		return err
	}
	target.UpdatedAt = time.Now()
	if target.Id > 0 {
		_, err := sess.ID(target.Id).AllCols().Update(target)
		return err
	}
	_, err = sess.Insert(target)
	return err
}
