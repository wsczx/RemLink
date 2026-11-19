package handler

import (
	"net/http"
	"net/url"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/notify"
	"github.com/wsczx/remlink/service"
)

const portalCookieName = "portal_session"

func (p *PortalService) Home(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	target := "/ui/#/portal"
	if redirect := r.URL.Query().Get("redirect"); redirect != "" {
		target += "?redirect=" + url.QueryEscape(redirect)
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// 返回 WebVPN 第三方登录使用的门户域名，未配置时返回空字符串
func portalMainDomain(r *http.Request) string {
	domain := base.GetCfg().WebVpnSsoDomain
	if domain == "" {
		return ""
	}
	host := stripPort(domain)
	port := portOf(domain)
	if port == "" {
		port = portOf(r.Host)
	}
	main := "https://" + host
	if port != "" {
		main += ":" + port
	}
	return main
}

func (p *PortalService) Me(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}
	portalOK(w, service.PortalUserInfo(user, getServerAddr(r)))
}

// 返回登录页品牌与认证方式开关，供门户、WebAuth、管理后台登录页在未登录时渲染
func (p *PortalService) LoginConfig(w http.ResponseWriter, r *http.Request) {
	brand := dbdata.SettingPortalBrand{}
	// 豁免：品牌配置缺失时用零值
	_ = dbdata.Settings.Get(&brand)
	portalOK(w, map[string]any{
		"title":             brand.Title,
		"logo":              brand.Logo,
		"favicon":           brand.Favicon,
		"desc":              brand.Desc,
		"footer":            brand.Footer,
		"features_enabled":  brand.FeaturesEnabled,
		"features":          brand.Features,
		"issuer":            base.GetCfg().Issuer,
		"domain":            base.GetCfg().WebVpnDomain,
		"webvpn_sso_domain": base.GetCfg().WebVpnSsoDomain,
		"sso_types":         p.sso.EnabledTypes(),
		"sms_enabled":       notify.IsSmsConfigured(),
	})
}

// 返回门户支持且至少存在一个组已配置的 SSO 类型
func (h *SSOHandler) EnabledTypes() []string {
	types := make([]string, 0, len(h.providers))
	for t := range h.providers {
		if _, err := portalSSOGroup(t); err == nil {
			types = append(types, t)
		}
	}
	return types
}

// 判断该 SSO 类型是否在 SSOHandler 注册
func (h *SSOHandler) TypeEnabled(ssoType string) bool {
	if ssoType == "" {
		return false
	}
	_, ok := h.providers[ssoType]
	return ok
}
