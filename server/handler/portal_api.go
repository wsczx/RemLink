package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/skip2/go-qrcode"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/jwt"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/service"
	"github.com/xlzd/gotp"
)

func portalCurrentUser(r *http.Request) (*dbdata.User, bool) {
	cookie, err := r.Cookie(portalCookieName)
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	data, err := jwt.GetJwtData(cookie.Value)
	if err != nil {
		return nil, false
	}
	username, ok := data["portal_user"].(string)
	if !ok || username == "" {
		return nil, false
	}
	user := &dbdata.User{}
	if err := dbdata.Users.GetByUsername(username, user); err == nil {
		return user, user.Status == 1 && !user.IsExpired()
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

func (p *PortalService) MyGroups(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}
	portalOK(w, service.PortalGroupsDetail(user.Groups, user.PolicyId))
}

func (p *PortalService) OTPStatus(w http.ResponseWriter, r *http.Request) {
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
	qrBase64, _ := generateOTPQR(user.Email, user.OtpSecret)
	portalOK(w, map[string]any{
		"enabled":   true,
		"disabled":  user.DisableOtp,
		"secret":    user.OtpSecret,
		"qr_base64": qrBase64,
	})
}

func (p *PortalService) RegenerateOTP(w http.ResponseWriter, r *http.Request) {
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
	if err := service.VerifyLocalPassword(user, req.Password); err != nil {
		portalError(w, "密码错误")
		return
	}

	secret := gotp.RandomSecret(32)
	if err := dbdata.GetDB().Update("Id", user.Id, &dbdata.User{OtpSecret: secret, DisableOtp: false}); err != nil {
		base.Error("用户门户重置 OTP 失败:", err)
		portalError(w, "重置失败")
		return
	}
	qrBase64, _ := generateOTPQR(user.Email, secret)
	portalOK(w, map[string]any{
		"secret":    secret,
		"qr_base64": qrBase64,
	})
}

func generateOTPQR(email, secret string) (string, error) {
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
func (p *PortalService) CertList(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}

	certs, err := dbdata.Certs.GetClientCertsByUsername(user.Username)
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
func (p *PortalService) DownloadCert(w http.ResponseWriter, r *http.Request) {
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
	cert, err := dbdata.Certs.GetClientCert(user.Username, req.Groupname)
	if err != nil {
		if utils.CheckErrNotFound(err) {
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
	p12Data, err := dbdata.Certs.GenerateClientP12FromDB(user.Username, req.Groupname, req.Password)
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
func (p *PortalService) Devices(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}

	sessions := p.sessions.GetOnlineSess("username", user.Username, false)
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
func (p *PortalService) DeviceOffline(w http.ResponseWriter, r *http.Request) {
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
	sessions := p.sessions.GetOnlineSess("username", user.Username, false)
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

	p.sessions.CloseSess(req.Token, dbdata.UserLogoutClient)
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
