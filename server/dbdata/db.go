package dbdata

import (
	"net/http"
	"strings"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/wsczx/remlink/base"
	"xorm.io/xorm"
	"xorm.io/xorm/core"
	"xorm.io/xorm/dialects"
)

var (
	xdb *xorm.Engine
)

func GetXdb() *xorm.Engine {
	return xdb
}
func TableModels() []any {
	return []any{
		&User{},
		&Setting{},
		&Group{},
		&IpMap{},
		&AccessAudit{},
		&AdminOpLog{},
		&Policy{},
		&StatsNetwork{},
		&StatsCpu{},
		&StatsMem{},
		&StatsOnline{},
		&UserActLog{},
		&PasswordReset{},
		&ClientCertData{},
		&Provider{},
		&WebVpnApp{},
		&WebVpnAudit{},
		&WebVpnRevoke{},
	}
}

func initDb() {
	var err error
	dbType := base.GetCfg().DbType
	dbSource := base.GetCfg().DbSource

	// SQLite 单写锁：默认加 busy_timeout 让驱动在拿不到写锁时等待而非立即报
	// "database is locked"。否则高并发写（流控自增、审计批量写）会频繁撞锁
	if dbType == "sqlite3" {
		if !strings.Contains(dbSource, "_busy_timeout=") {
			sep := "?"
			if strings.Contains(dbSource, "?") {
				sep = "&"
			}
			dbSource += sep + "_busy_timeout=5000"
		}
		// 注入 SQLite 锁退避重试（见 db_sqlite_retry.go）
		sqlDB, dErr := newRetrySqliteDB(dbSource)
		if dErr != nil {
			base.Fatal(dErr)
		}
		dialect, dErr := dialects.OpenDialect(dbType, dbSource)
		if dErr != nil {
			base.Fatal(dErr)
		}
		xdb, err = xorm.NewEngineWithDialectAndDB(dbType, dbSource, dialect, core.FromDB(sqlDB))
	} else {
		xdb, err = xorm.NewEngine(dbType, dbSource)
	}
	if err != nil {
		base.Fatal(err)
	}

	// 初始化xorm时区
	xdb.DatabaseTZ = time.Local
	xdb.TZLocation = time.Local

	if base.GetCfg().ShowSQL {
		xdb.ShowSQL(true)
	}

	// 初始化数据库
	err = xdb.Sync2(TableModels()...)
	if err != nil {
		base.Fatal(err)
	}
}

func initData() {
	var (
		err error
	)

	// 判断是否初次使用
	install := &SettingInstall{}
	err = SettingGet(install)

	if err == nil && install.Installed {
		// 已经安装过
		return
	}

	// 发生错误
	if err != ErrNotFound {
		base.Fatal(err)
	}

	err = addInitData()
	if err != nil {
		base.Fatal(err)
	}

}

func addInitData() error {
	var (
		err error
	)

	sess := xdb.NewSession()
	defer sess.Close()

	err = sess.Begin()
	if err != nil {
		return err
	}
	defer sess.Rollback()

	// SettingSmtp
	smtp := &SettingSmtp{
		Host:       "127.0.0.1",
		Port:       25,
		From:       "vpn@xx.com",
		Encryption: "None",
	}
	err = SettingSessAdd(sess, smtp)
	if err != nil {
		return err
	}

	// SettingAuditLog
	auditLog := SettingGetAuditLogDefault()
	err = SettingSessAdd(sess, auditLog)
	if err != nil {
		return err
	}

	// ServerConfig
	cfg := *base.GetCfg()
	base.CompleteConfig(&cfg)
	serverConfig := &SettingServerConfig{Config: cfg}
	err = SettingSessAdd(sess, serverConfig)
	if err != nil {
		return err
	}

	// Profile
	profile := &SettingProfile{Content: base.DefaultProfileXML}
	err = SettingSessAdd(sess, profile)
	if err != nil {
		return err
	}

	// SettingDnsProvider
	provider := &SettingLetsEncrypt{
		Domain:      "vpn.xxx.com",
		Legomail:    "legomail",
		Name:        "aliyun",
		Renew:       false,
		DNSProvider: DNSProvider{},
	}
	err = SettingSessAdd(sess, provider)
	if err != nil {
		return err
	}
	// LegoUser
	legouser := &LegoUserData{}
	err = SettingSessAdd(sess, legouser)
	if err != nil {
		return err
	}
	// SettingOther
	other := &SettingOther{
		LinkAddr:    "vpn.xx.com",
		Banner:      "您已接入公司网络，请按照公司规定使用。\n请勿进行非工作下载及视频行为！",
		Homecode:    http.StatusOK,
		Homeindex:   "RemLink 是一个企业级远程办公 sslvpn 的软件，可以支持多人同时在线使用。",
		AccountMail: accountMail,
		CertMail:    certMail,
	}
	err = SettingSessAdd(sess, other)
	if err != nil {
		return err
	}

	err = sess.Commit()
	if err != nil {
		return err
	}

	// 创建内置策略：全局代理
	pAll := &Policy{
		Name:         "全局代理",
		Note:         "系统初始化内置策略",
		AllowLan:     true,
		ClientDns:    []ValData{{Val: "114.114.114.114", Note: "默认dns"}},
		RouteInclude: []ValData{{Val: ALL}},
		Status:       1,
	}
	err = SetPolicy(pAll)
	if err != nil {
		return err
	}

	// 创建内置策略：仅内网分流
	pOps := &Policy{
		Name:         "仅内网分流",
		Note:         "系统初始化内置策略",
		AllowLan:     true,
		ClientDns:    []ValData{{Val: "114.114.114.114", Note: "默认dns"}},
		RouteInclude: []ValData{{Val: "10.0.0.0/8"}},
		Status:       1,
	}
	err = SetPolicy(pOps)
	if err != nil {
		return err
	}

	// 创建用户组，引用策略
	g1 := Group{
		Name:     "all",
		PolicyId: pAll.Id,
		Status:   1,
	}
	err = SetGroup(&g1)
	if err != nil {
		return err
	}

	g2 := Group{
		Name:     "ops",
		PolicyId: pOps.Id,
		Status:   1,
	}
	err = SetGroup(&g2)
	if err != nil {
		return err
	}

	install := &SettingInstall{Installed: true}
	if err = SettingSave(install); err != nil {
		return err
	}
	base.LoadPersistedConfig(cfg)

	return nil
}

func CheckErrNotFound(err error) bool {
	return err == ErrNotFound
}

// base64 图片
// 用户动态码(请妥善保存):<br/>
// <img src="{{.OtpImgBase64}}"/><br/>
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

// 返回默认证书邮件模板，供管理后台获取初始值。
func CertMailTemplate() string {
	return certMail
}
