package dbdata

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/security"
	"xorm.io/xorm"
)

type SettingInstall struct {
	Installed bool `json:"installed"`
}

type SettingServerConfig struct {
	Config base.ServerConfig `json:"config"`
}

// 加密启用时对 JwtSecret/AdminOtp 加密。
func (s SettingServerConfig) MarshalJSON() ([]byte, error) {
	if security.IsEnabled() {
		s.Config.JwtSecret = security.EncryptIfNeeded(s.Config.JwtSecret)
		s.Config.AdminOtp = security.EncryptIfNeeded(s.Config.AdminOtp)
	}
	type alias SettingServerConfig
	return json.Marshal(alias(s))
}

// 加密启用时自动解密 JwtSecret/AdminOtp。
func (s *SettingServerConfig) UnmarshalJSON(data []byte) error {
	type alias SettingServerConfig
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*s = SettingServerConfig(a)
	if security.IsEnabled() {
		s.Config.JwtSecret = security.DecryptIfNeeded(s.Config.JwtSecret)
		s.Config.AdminOtp = security.DecryptIfNeeded(s.Config.AdminOtp)
	}
	return nil
}

type SettingProfile struct {
	Content string `json:"content"`
}

type SettingSmtp struct {
	Host       string                   `json:"host"`
	Port       int                      `json:"port"`
	Username   string                   `json:"username"`
	Password   security.EncryptedString `json:"password"`
	From       string                   `json:"from"`
	Encryption string                   `json:"encryption"`
}

type SettingSms struct {
	Provider string `json:"provider"` // 空=关闭 / aliyun / tencent

	// 阿里云
	AliAccessKeyId     string                   `json:"ali_access_key_id"`
	AliAccessKeySecret security.EncryptedString `json:"ali_access_key_secret"`
	AliSignName        string                   `json:"ali_sign_name"`
	AliTemplateCode    string                   `json:"ali_template_code"`

	// 腾讯云
	TencentSecretId   string                   `json:"tencent_secret_id"`
	TencentSecretKey  security.EncryptedString `json:"tencent_secret_key"`
	TencentSdkAppId   string                   `json:"tencent_app_id"`
	TencentSignName   string                   `json:"tencent_sign_name"`
	TencentTemplateId string                   `json:"tencent_template_id"`
	TencentRegion     string                   `json:"tencent_region"` // 地域，默认 ap-guangzhou
}

type SettingAuditLog struct {
	LifeDay   int    `json:"life_day"`
	ClearTime string `json:"clear_time"`
}

type SettingOther struct {
	LinkAddr     string `json:"link_addr"`
	BannerEnable bool   `json:"banner_enable"`
	Banner       string `json:"banner"`
	Homecode     int    `json:"homecode"`
	Homeindex    string `json:"homeindex"`
	AccountMail  string `json:"account_mail"`
	CertMail     string `json:"cert_mail"`
}

// 用户门户、WebAuth 与管理后台登录页共用的品牌展示配置。
// 字段为空时各前端回退到默认展示（RemLink 图标/名称/页脚）。
type SettingPortalBrand struct {
	Title           string `json:"title"`            // 品牌名称，为空时前端回退默认
	Logo            string `json:"logo"`             // 品牌 Logo 图片地址（URL 或 data URI），为空时前端回退默认图标
	Favicon         string `json:"favicon"`          // 网站图标地址（URL 或 data URI），为空时前端使用默认 favicon
	Desc            string `json:"desc"`             // 品牌副标题，为空时各前端回退各自默认文案
	Footer          string `json:"footer"`           // 页脚内容（支持 HTML），为空时前端回退默认页脚
	FeaturesEnabled int    `json:"features_enabled"` // 登录页功能卡片开关：0 未配置（默认显示）/ 1 显示 / 2 关闭
	Features        string `json:"features"`         // 功能卡片自定义内容（JSON 数组 [{label,desc}]），为空时前端用默认三项
}

// 用户门户登录后首页（仪表盘）的自定义配置
type SettingPortalDashboard struct {
	AnnouncementEnabled int    `json:"announcement_enabled"` // 公告横幅开关：0 未配置（默认关闭）/ 1 显示 / 2 关闭
	Announcement        string `json:"announcement"`         // 公告内容（HTML，仅管理员可写）
	AnnouncementLevel   string `json:"announcement_level"`   // 公告样式：info / warning / success / error
	QuickLinksEnabled   int    `json:"quick_links_enabled"`  // 快捷链接开关：0 未配置（默认关闭）/ 1 显示 / 2 关闭
	QuickLinks          string `json:"quick_links"`          // 快捷链接（JSON 数组 [{label,url,icon}]），为空时前端用默认
	CardsVisible        string `json:"cards_visible"`        // 仪表盘各卡片显隐（JSON 对象 {区块名:bool}），缺省视为显示
	ThemeColor          string `json:"theme_color"`          // 主题主色（十六进制，如 #2f7cff），为空时前端回退默认
	CustomCss           string `json:"custom_css"`           // 自定义 CSS（注入 <style>），为空时不注入
	ClientGuide         string `json:"client_guide"`         // 客户端连接指引（JSON 数组 [{name,steps:[html...]}]），为空时前端回退默认 5 平台
	ClientGuideEnabled  int    `json:"client_guide_enabled"` // 客户端连接指引开关：0 未配置（默认关闭）/ 1 显示 / 2 关闭
	ClientDownloadHtml  string `json:"client_download_html"` // 客户端下载内容（HTML），为空时回退默认下载页
}

// 证书独立存储，避免每次修改普通配置时序列化 PEM 大字段
type SettingTLSCert struct {
	CertContent    string                   `json:"cert_content"`
	CertKeyContent security.EncryptedString `json:"cert_key_content"`
}

// WebVPN 泛域名证书，用于 *.WebVpnDomain 子域。
type SettingTLSCertWild struct {
	CertContent    string                   `json:"cert_content"`
	CertKeyContent security.EncryptedString `json:"cert_key_content"`
}

type SettingClientCA struct {
	CertContent string                   `json:"cert_content"`
	KeyContent  security.EncryptedString `json:"key_content"`
}

// 记录已使用的密码重置 token，防止重复使用。
type SettingPortalResetTokens struct {
	Tokens map[string]int64 `json:"tokens"` // jti -> used_at unix timestamp
}

func StructName(data any) string {
	t := reflect.TypeOf(data)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

func SettingSessAdd(sess *xorm.Session, data any) error {
	name := StructName(data)
	v, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	s := &Setting{Name: name, Data: v}
	_, err = sess.InsertOne(s)
	return err
}

func SettingSave(data any) error {
	name := StructName(data)
	v, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	// 尝试插入
	_, err = xdb.InsertOne(&Setting{Name: name, Data: v})
	if err == nil {
		return nil
	}
	// 插入失败，回退到更新
	exist := &Setting{}
	if getErr := One("name", name, exist); getErr != nil {
		return getErr
	}
	_, err = xdb.ID(exist.Id).Cols("Data").Update(&Setting{Id: exist.Id, Data: v})
	return err
}

func SettingGet(data any) error {
	name := StructName(data)
	s := &Setting{}
	err := One("name", name, s)
	if err != nil {
		return err
	}
	err = json.Unmarshal(s.Data, data)
	return err
}

// 在指定 session 内读取 (用于事务)
func SettingSessGet(sess *xorm.Session, data any) error {
	name := StructName(data)
	s := &Setting{}
	has, err := sess.Where("name = ?", name).Get(s)
	if err != nil {
		return err
	}
	if !has {
		return fmt.Errorf("setting %s not found", name)
	}
	return json.Unmarshal(s.Data, data)
}

func SettingGetAuditLog() (SettingAuditLog, error) {
	data := SettingAuditLog{}
	err := SettingGet(&data)
	if err == nil {
		return data, nil
	}
	if CheckErrNotFound(err) {
		return SettingGetAuditLogDefault(), nil
	}
	return data, err
}

func SettingGetAuditLogDefault() SettingAuditLog {
	return SettingAuditLog{
		LifeDay:   0,
		ClearTime: "05:00",
	}
}

func SettingLoadServerConfig() error {
	data := &SettingServerConfig{}
	err := SettingGet(data)
	if err == nil {
		base.LoadPersistedConfig(data.Config)
		return nil
	}
	if !CheckErrNotFound(err) {
		return err
	}
	// DB 中无记录，写入默认值（首次全新安装）
	data.Config = *base.GetCfg()
	base.CompleteConfig(&data.Config)

	name := StructName(data)
	v, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	if err := Add(&Setting{Name: name, Data: v}); err != nil {
		return err
	}
	base.LoadPersistedConfig(data.Config)
	return nil
}

func SettingSaveServerConfig() error {
	data := &SettingServerConfig{Config: *base.GetCfg()}
	return saveServerConfig(data)
}

func SettingSaveServerConfigWith(cfg *base.ServerConfig) error {
	data := &SettingServerConfig{Config: *cfg}
	return saveServerConfig(data)
}

func saveServerConfig(data *SettingServerConfig) error {
	return SettingSave(data)
}

func SettingSaveTLSCert(certContent, keyContent string) error {
	tls := SettingTLSCert{
		CertContent:    certContent,
		CertKeyContent: security.EncryptedString(keyContent),
	}
	return SettingSave(&tls)
}

// 保存 WebVPN 泛域名证书
func SettingSaveTLSCertWild(certContent, keyContent string) error {
	tls := SettingTLSCertWild{
		CertContent:    certContent,
		CertKeyContent: security.EncryptedString(keyContent),
	}
	return SettingSave(&tls)
}

func SettingSaveClientCA(certContent, keyContent string) error {
	ca := SettingClientCA{
		CertContent: certContent,
		KeyContent:  security.EncryptedString(keyContent),
	}
	if err := SettingSave(&ca); err != nil {
		return err
	}
	resetClientCA()
	return nil
}

func SettingGetProfile() (SettingProfile, error) {
	data := SettingProfile{}
	err := SettingGet(&data)
	if err == nil {
		return data, nil
	}
	if CheckErrNotFound(err) {
		return SettingProfile{Content: base.DefaultProfileXML}, nil
	}
	return data, err
}

func SettingSetProfile(content string) error {
	data := &SettingProfile{Content: content}
	return SettingSave(data)
}

func GetProfileXML() ([]byte, error) {
	data, err := SettingGetProfile()
	if err != nil {
		return nil, err
	}
	return []byte(data.Content), nil
}

func GetProfileHash() (string, error) {
	b, err := GetProfileXML()
	if err != nil {
		return "", err
	}
	ha := sha1.Sum(b)
	return hex.EncodeToString(ha[:]), nil
}
