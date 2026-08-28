package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/skip2/go-qrcode"
	"github.com/wsczx/remlink/admin"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/sessdata"
	"github.com/xlzd/gotp"
)

func portalCheckLocalPassword(user *dbdata.User, password string) error {
	if password == "" || !dbdata.VerifyPassword(password, user.PinCode) {
		return fmt.Errorf("密码错误")
	}
	return nil
}

func portalIssueToken(user *dbdata.User) (string, error) {
	return admin.SetJwtData(map[string]any{
		"portal_user_id": user.Id,
		"portal_user":    user.Username,
		"portal_type":    user.Type,
		"portal_groups":  user.Groups,
	}, time.Now().Unix()+3600*3)
}

func portalCurrentUser(r *http.Request) (*dbdata.User, bool) {
	cookie, err := r.Cookie(portalCookieName)
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	data, err := admin.GetJwtData(cookie.Value)
	if err != nil {
		return nil, false
	}
	username, ok := data["portal_user"].(string)
	if !ok || username == "" {
		return nil, false
	}
	user := &dbdata.User{}
	if err := dbdata.One("Username", username, user); err == nil {
		return user, user.Status == 1 && !dbdata.IsUserExpired(user)
	}

	userType, _ := data["portal_type"].(string)
	if userType == "local" || userType == "ldap" {
		base.Warn("本地用户已删除但仍持有有效 JWT:", username)
		return nil, false
	}

	groupNames, _ := data["portal_groups"].([]any)
	groups := make([]string, 0, len(groupNames))
	for _, group := range groupNames {
		if name, ok := group.(string); ok && name != "" {
			groups = append(groups, name)
		}
	}
	if username == "" || len(groups) == 0 {
		return nil, false
	}
	return &dbdata.User{
		Type:     userType,
		Username: username,
		Groups:   groups,
		Status:   1,
	}, true
}

func portalUserInfo(user *dbdata.User, r *http.Request) map[string]any {
	cfg := base.GetCfg()
	serverAddr := cfg.ServerAddr
	if r != nil {
		serverAddr = getServerAddr(r)
	}
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
		"groups_detail":       portalGroupsDetail(user.Groups, user.PolicyId),
		"user_policy":         portalPolicyInfo(user.PolicyId),
		"traffic_used":        user.TrafficUsed,
		"traffic_reset_at":    user.TrafficResetAt,
	}

	dash := dbdata.SettingPortalDashboard{}
	if err := dbdata.SettingGet(&dash); err != nil {
		if dbdata.CheckErrNotFound(err) {
			// 默认值
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
func portalGroupsDetail(groupNames []string, userPolicyId int) []map[string]any {
	allGroups, err := dbdata.GetAllGroups()
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
	if err := dbdata.One("Id", policyId, &policy); err != nil {
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

func PortalMyGroups(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}
	portalOK(w, portalGroupsDetail(user.Groups, user.PolicyId))
}

func PortalOTPStatus(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}
	if user.OtpSecret == "" {
		portalOK(w, map[string]any{
			"enabled": false,
		})
		return
	}
	// 已设置 OTP 时返回状态及密钥，方便用户绑定多设备
	qrBase64, _ := portalGenerateOtpQr(user.Email, user.OtpSecret)
	portalOK(w, map[string]any{
		"enabled":   true,
		"disabled":  user.DisableOtp,
		"secret":    user.OtpSecret,
		"qr_base64": qrBase64,
	})
}

func PortalOTPRegenerate(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}
	if user.Type != "" && user.Type != "local" {
		portalError(w, "外部认证用户不支持自助重置二次验证")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		portalError(w, "参数错误")
		return
	}
	if req.Password == "" {
		portalError(w, "请输入当前密码确认")
		return
	}
	if err := portalCheckLocalPassword(user, req.Password); err != nil {
		portalError(w, "密码错误")
		return
	}

	secret := gotp.RandomSecret(32)
	if err := dbdata.Update("Id", user.Id, &dbdata.User{OtpSecret: secret, DisableOtp: false}); err != nil {
		base.Error("用户门户重置 OTP 失败:", err)
		portalError(w, "重置失败")
		return
	}
	qrBase64, _ := portalGenerateOtpQr(user.Email, secret)
	portalOK(w, map[string]any{
		"secret":    secret,
		"qr_base64": qrBase64,
	})
}

func portalGenerateOtpQr(email, secret string) (string, error) {
	issuer := url.QueryEscape(base.GetCfg().Issuer)
	qrstr := fmt.Sprintf("otpauth://totp/%s:%s?issuer=%s&secret=%s", issuer, email, issuer, secret)
	qr, err := qrcode.New(qrstr, qrcode.High)
	if err != nil {
		return "", err
	}
	png, err := qr.PNG(256)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(png), nil
}

// 返回当前用户的所有客户端证书（不含私钥）
func PortalCertList(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}

	certs, err := dbdata.GetClientCertsByUsername(user.Username)
	if err != nil {
		portalError(w, "获取证书列表失败")
		return
	}

	type certItem struct {
		Id                   int       `json:"id"`
		Groupname            string    `json:"groupname"`
		Status               int       `json:"status"`
		StatusText           string    `json:"status_text"`
		IsCSRBased           bool      `json:"is_csr_based"`
		SerialNumber         string    `json:"serial_number"`
		NotAfter             time.Time `json:"not_after"`
		CreatedAt            time.Time `json:"created_at"`
		DeviceBindingEnabled bool      `json:"device_binding_enabled"`
		MaxDevices           int       `json:"max_devices"`
		DeviceCount          int       `json:"device_count"`
	}

	items := make([]certItem, 0, len(certs))
	for i := range certs {
		c := &certs[i]
		statusText := "有效"
		switch c.Status {
		case dbdata.CertStatusDisabled:
			statusText = "已禁用"
		case dbdata.CertStatusExpired:
			statusText = "已过期"
		}
		items = append(items, certItem{
			Id:                   c.Id,
			Groupname:            c.Groupname,
			Status:               c.Status,
			StatusText:           statusText,
			IsCSRBased:           c.IsCSRBased,
			SerialNumber:         c.SerialNumber,
			NotAfter:             c.NotAfter,
			CreatedAt:            c.CreatedAt,
			DeviceBindingEnabled: c.DeviceBindingEnabled,
			MaxDevices:           c.MaxDevices,
			DeviceCount:          len(c.DeviceId),
		})
	}
	portalOK(w, items)
}

// 下载当前用户的 P12 客户端证书（POST，密码在请求体中）
func PortalCertDownload(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}

	var req struct {
		Groupname string `json:"groupname"`
		Password  string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		portalError(w, "参数错误")
		return
	}

	if req.Groupname == "" {
		portalError(w, "用户组不能为空")
		return
	}
	if req.Password == "" {
		portalError(w, "P12加密密码不能为空")
		return
	}

	// 安全校验：确保证书属于当前用户
	cert, err := dbdata.GetClientCert(user.Username, req.Groupname)
	if err != nil {
		if dbdata.CheckErrNotFound(err) {
			portalError(w, "未找到该证书")
		} else {
			portalError(w, "获取证书失败")
		}
		return
	}

	// CSR 模式直接返回 PEM 证书
	if cert.IsCSRBased {
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.cer", user.Username))
		w.Write([]byte(cert.Certificate))
		return
	}

	// 生成 P12
	p12Data, err := dbdata.GenerateClientP12FromDB(user.Username, req.Groupname, req.Password)
	if err != nil {
		portalError(w, fmt.Sprintf("证书下载失败: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/x-pkcs12")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.p12", user.Username))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(p12Data)))
	w.Write(p12Data)
}

// 返回当前用户的在线设备列表
func PortalDevices(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}

	sessions := sessdata.GetOnlineSess("username", user.Username, false)
	type deviceItem struct {
		Token            string `json:"token"`
		Ip               string `json:"ip"`
		MacAddr          string `json:"mac_addr"`
		RemoteAddr       string `json:"remote_addr"`
		Transport        string `json:"transport"`
		Client           string `json:"client"`
		DeviceType       string `json:"device_type"`
		PlatformVersion  string `json:"platform_version"`
		BandwidthUp      string `json:"bandwidth_up"`
		BandwidthDown    string `json:"bandwidth_down"`
		BandwidthUpAll   string `json:"bandwidth_up_all"`
		BandwidthDownAll string `json:"bandwidth_down_all"`
		LastLogin        string `json:"last_login"`
	}
	items := make([]deviceItem, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, deviceItem{
			Token:            s.Token,
			Ip:               s.Ip.String(),
			MacAddr:          s.MacAddr,
			RemoteAddr:       s.RemoteAddr,
			Transport:        s.TransportProtocol,
			Client:           s.Client,
			DeviceType:       s.DeviceType,
			PlatformVersion:  s.PlatformVersion,
			BandwidthUp:      s.BandwidthUp,
			BandwidthDown:    s.BandwidthDown,
			BandwidthUpAll:   s.BandwidthUpAll,
			BandwidthDownAll: s.BandwidthDownAll,
			LastLogin:        s.LastLogin.Format("2006-01-02 15:04:05"),
		})
	}
	portalOK(w, items)
}

// 踢下线指定设备
func PortalDeviceOffline(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		portalError(w, "参数错误")
		return
	}

	// 校验该 session 属于当前用户
	sessions := sessdata.GetOnlineSess("username", user.Username, false)
	found := false
	for _, s := range sessions {
		if s.Token == req.Token {
			found = true
			break
		}
	}
	if !found {
		portalError(w, "设备不存在或已离线")
		return
	}

	sessdata.CloseSess(req.Token, dbdata.UserLogoutClient)
	portalOK(w, map[string]string{"message": "已断开该设备连接"})
}

func portalOK(w http.ResponseWriter, data any) {
	portalJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"msg":  "ok",
		"data": data,
	})
}

func portalError(w http.ResponseWriter, msg string) {
	portalJSON(w, http.StatusOK, map[string]any{
		"code": 1,
		"msg":  msg,
	})
}

func portalUnauthorized(w http.ResponseWriter) {
	portalJSON(w, http.StatusUnauthorized, map[string]any{
		"code": 401,
		"msg":  "请先登录",
	})
}

func portalJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
