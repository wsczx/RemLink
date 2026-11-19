package handler

import (
	"fmt"
	"github.com/wsczx/remlink/base"
	"net/http"
	"net/url"
)

func (p *PortalService) SSOLogin(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	ssoType := r.URL.Query().Get("type")
	if !p.sso.TypeEnabled(ssoType) {
		http.Error(w, "不支持的第三方登录类型", http.StatusBadRequest)
		return
	}
	redirect := r.URL.Query().Get("redirect")

	if _, ok := webVpnHostPrefix(r.Host); ok {
		main := portalMainDomain(r)
		if main == "" {
			http.Error(w, "未配置 WebVPN 第三方登录专用门户域名，请先在系统设置中填写 webvpn_sso_domain", http.StatusBadRequest)
			return
		}
		target := main + "/portal/api/sso?type=" + url.QueryEscape(ssoType)
		if redirect != "" {
			target += "&redirect=" + url.QueryEscape(redirect)
		}
		http.Redirect(w, r, target, http.StatusFound)
		return
	}

	group, err := portalSSOGroup(ssoType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	target := fmt.Sprintf("/+CSCOE+/saml/sp/login?tgname=%s&ssotype=%s&from=portal",
		url.QueryEscape(group), url.QueryEscape(ssoType))
	if redirect != "" {
		target += "&redirect=" + url.QueryEscape(redirect)
	}
	http.Redirect(w, r, target, http.StatusFound)
}
