package service

import (
	"encoding/json"
	"time"

	"github.com/wsczx/remlink/pkg/utils"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/jwt"
)

// 签发门户 JWT 并登记，便于改密/停用/删号时批量吊销
func IssuePortalToken(user *dbdata.User) (string, error) {
	token, err := jwt.SetJwtData(map[string]any{
		"portal_user_id": user.Id,
		"portal_user":    user.Username,
		"portal_type":    user.Type,
		"portal_groups":  user.Groups,
	}, time.Now().Unix()+3600*3)
	if err != nil {
		return "", err
	}
	jwt.RegisterJwt(user.Username, token)
	return token, nil
}

// 门户用户信息
func PortalUserInfo(user *dbdata.User, serverAddr string) map[string]any {
	cfg := base.GetCfg()
	result := map[string]any{
		"id":                  user.Id,
		"username":            user.Username,
		"name":                user.Nickname,
		"email":               user.Email,
		"groups":              user.Groups,
		"type":                user.Type,
		"status":              user.Status,
		"limittime":           user.LimitTime,
		"mtu":                 user.Mtu,
		"disable_otp":         user.DisableOtp,
		"otp_enabled":         user.OtpSecret != "" && !user.DisableOtp,
		"can_change_password": user.Type == "" || user.Type == "local",
		"created_at":          user.CreatedAt.Unix(),
		"server_addr":         serverAddr,
		"issuer":              cfg.Issuer,
		"groups_detail":       PortalGroupsDetail(user.Groups, user.PolicyId),
		"user_policy":         portalPolicyInfo(user.PolicyId),
		"traffic_used":        user.TrafficUsed,
		"traffic_reset_at":    user.TrafficResetAt,
	}

	dash := dbdata.SettingPortalDashboard{}
	if err := dbdata.Settings.Get(&dash); err != nil {
		if utils.CheckErrNotFound(err) {
			dash.ClientDownloadHtml = base.DefaultDownloadHtml
		}
	}
	result["dashboard"] = map[string]any{
		"announcement_enabled": dash.AnnouncementEnabled,
		"announcement":         dash.Announcement,
		"announcement_level":   dash.AnnouncementLevel,
		"quick_links_enabled":  dash.QuickLinksEnabled,
		"quick_links":          dash.QuickLinks,
		"cards_visible":        dash.CardsVisible,
		"theme_color":          dash.ThemeColor,
		"custom_css":           dash.CustomCss,
		"client_guide":         dash.ClientGuide,
		"client_guide_enabled": dash.ClientGuideEnabled,
		"client_download_html": dash.ClientDownloadHtml,
	}
	return result
}

// 构建所属组详情（认证方式、拆分 DNS、策略摘要）
func PortalGroupsDetail(groupNames []string, userPolicyId int) []map[string]any {
	allGroups, err := dbdata.Groups.GetAllGroups()
	if err != nil {
		return nil
	}
	groupMap := make(map[string]dbdata.Group, len(allGroups))
	for _, g := range allGroups {
		groupMap[g.Name] = g
	}

	result := make([]map[string]any, 0, len(groupNames))
	for _, gname := range groupNames {
		g, ok := groupMap[gname]
		if !ok {
			continue
		}
		info := map[string]any{
			"name":       g.Name,
			"note":       g.Note,
			"auth_types": portalAuthTypeLabels(g.AuthProfile),
			"dns":        portalDnsList(g.SplitDns),
			"status":     g.Status,
		}
		if userPolicyId == 0 && g.PolicyId > 0 {
			info["policy"] = portalPolicyInfo(g.PolicyId)
		}
		result = append(result, info)
	}
	return result
}

func portalAuthTypeLabels(raw json.RawMessage) []string {
	profile, err := auth.ParseAuthProfile(raw)
	if err != nil {
		return nil
	}
	labelMap := map[string]string{
		"local":    "本地密码",
		"ldap":     "LDAP",
		"radius":   "RADIUS",
		"cert":     "TLS证书",
		"otp":      "动态验证码",
		"wxwork":   "企微",
		"feishu":   "飞书",
		"dingtalk": "钉钉",
	}
	labels := make([]string, 0, len(profile.Step))
	for _, step := range profile.Step {
		label, ok := labelMap[step.Type]
		if !ok {
			label = step.Type
		}
		labels = append(labels, label)
	}
	return labels
}

func portalDnsList(splitDns []dbdata.ValData) []string {
	vals := make([]string, 0, len(splitDns))
	for _, d := range splitDns {
		if d.Val != "" {
			vals = append(vals, d.Val)
		}
	}
	return vals
}

func portalPolicyInfo(policyId int) map[string]any {
	if policyId <= 0 {
		return nil
	}
	var policy dbdata.Policy
	if err := dbdata.Policies.Get(policyId, &policy); err != nil {
		return nil
	}
	return map[string]any{
		"id":               policy.Id,
		"name":             policy.Name,
		"note":             policy.Note,
		"allow_lan":        policy.AllowLan,
		"bandwidth":        policy.Bandwidth,
		"bandwidth_up":     policy.BandwidthUp,
		"traffic_quota":    policy.TrafficQuota,
		"traffic_reset":    policy.TrafficReset,
		"route_include":    len(policy.RouteInclude),
		"route_exclude":    len(policy.RouteExclude),
		"ds_include_count": len(policy.DsIncludeDomains),
		"ds_exclude_count": len(policy.DsExcludeDomains),
		"acl_count":        len(policy.LinkAcl),
	}
}
