package admin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/skip2/go-qrcode"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xlzd/gotp"
)

var (
	otpPreviewSecrets = make(map[string]string)
	otpPreviewMu      sync.Mutex
)

// 登陆接口（两步登录：先验密码，OTP 启用时返回 otp_required）
func Login(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	adminUser := r.PostFormValue("admin_user")
	adminPass := r.PostFormValue("admin_pass")

	lm := auth.GetLockManager()

	// 防暴力破解：检查是否已被锁定
	if !lm.Check(adminUser, r.RemoteAddr) {
		RespError(w, 1, "登录过于频繁，请稍后再试")
		base.Error(adminUser, "管理员登录被锁定:", r.RemoteAddr)
		dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: adminUser, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "账号已被锁定，请稍后重试", Client: dbdata.UserAdminClient, IsLockedFail: true}, r.UserAgent())
		return
	}

	// 先验证基础密码
	if err := authsrv.CheckAdminPassword(adminUser, adminPass); err != nil {
		lm.Fail(adminUser, r.RemoteAddr)
		RespError(w, RespUserOrPassErr)
		base.Error(adminUser, "管理员用户名或密码错误")
		dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: adminUser, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "管理员登录失败", Client: dbdata.UserAdminClient}, r.UserAgent())
		return
	}

	// 启用了 OTP：返回 otp_required，等待第二步验证
	if base.GetCfg().AdminOtp != "" {
		// 密码验证成功，清除密码阶段的失败计数，OTP 阶段从头计数
		lm.Success(adminUser, r.RemoteAddr)
		otpToken, err := SetJwtData(map[string]any{"otp_user": adminUser}, time.Now().Unix()+300)
		if err != nil {
			RespError(w, 1, err)
			return
		}
		RespSucess(w, map[string]any{
			"otp_required": true,
			"otp_token":    otpToken,
		})
		dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: adminUser, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthSuccess, Info: "管理员登录成功", Client: dbdata.UserAdminClient}, r.UserAgent())
		return
	}

	// 未启用 OTP：直接签发 JWT
	lm.Success(adminUser, r.RemoteAddr)
	dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: adminUser, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthSuccess, Info: "管理员登录成功", Client: dbdata.UserAdminClient}, r.UserAgent())
	issueLoginJWT(w, r, adminUser)
}

// OTP 第二步验证
func LoginOTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	var req struct {
		OtpToken string `json:"otp_token"`
		OtpCode  string `json:"otp_code"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		RespError(w, RespParamErr, "参数错误")
		return
	}
	if req.OtpToken == "" || req.OtpCode == "" {
		RespError(w, RespParamErr, "参数不能为空")
		return
	}

	// 验证 otp_token
	data, err := GetJwtData(req.OtpToken)
	if err != nil {
		RespError(w, RespTokenErr)
		base.Error("otp_token 验证失败:", err)
		return
	}
	adminUser, ok := data["otp_user"].(string)
	if !ok || adminUser != base.GetCfg().AdminUser {
		RespError(w, RespTokenErr)
		return
	}

	// 检查是否已被锁定
	lm := auth.GetLockManager()
	if !lm.Check(adminUser, r.RemoteAddr) {
		RespError(w, RespParamErr, "登录过于频繁，请稍后再试")
		base.Error(adminUser, "管理员 OTP 登录尝试次数超限，已锁定:", r.RemoteAddr)
		dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: adminUser, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "账号已被锁定，请稍后重试", Client: dbdata.UserAdminClient, IsLockedFail: true}, r.UserAgent())
		return
	}

	// 验证 OTP
	if err := authsrv.VerifyAdminOTP(req.OtpCode); err != nil {
		lm.Fail(adminUser, r.RemoteAddr)
		RespError(w, RespParamErr, "OTP 验证码错误")
		base.Error(adminUser, "管理员 OTP 验证失败")
		dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: adminUser, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "管理员登录失败", Client: dbdata.UserAdminClient}, r.UserAgent())
		return
	}

	// 验证成功，清除锁定计数
	lm.Success(adminUser, r.RemoteAddr)
	dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: adminUser, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthSuccess, Info: "管理员登录成功", Client: dbdata.UserAdminClient}, r.UserAgent())
	issueLoginJWT(w, r, adminUser)
}

// 管理员修改自身密码
func ChangeAdminPassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		RespError(w, RespParamErr, "参数错误")
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		RespError(w, RespParamErr, "新旧密码不能为空")
		return
	}
	if err := utils.CheckPasswordPolicy(req.NewPassword); err != nil {
		RespError(w, RespParamErr, err.Error())
		return
	}

	// 验证旧密码
	if err := authsrv.CheckAdminPassword(base.GetCfg().AdminUser, req.OldPassword); err != nil {
		RespError(w, RespUserOrPassErr, "旧密码错误")
		return
	}

	// 哈希新密码
	hashed, err := utils.PasswordHash(req.NewPassword)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	// 先构造新配置快照并持久化到数据库
	cfg := base.GetCfg()
	newCfg := *cfg
	newCfg.AdminPass = hashed
	newCfg.AdminTemp = false
	if err := dbdata.SettingSaveServerConfigWith(&newCfg); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	// DB 保存成功后，再原子切换到新配置
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.AdminPass = hashed
		c.AdminTemp = false
	})
	base.Info("管理员密码已修改")

	dbdata.AdminLog("安全设置", "管理员密码", "修改了管理员登录密码", r.RemoteAddr)
	RespSucess(w, map[string]string{"message": "密码修改成功"})
}

// AdminOtpQr 查看管理员 OTP 密钥二维码
// GET：仅返回启用状态（供前端检查 OTP 是否已启用）
// POST：需提交密码+当前动态验证码，验证通过后返回密钥和二维码
func AdminOtpQr(w http.ResponseWriter, r *http.Request) {
	cfg := base.GetCfg()
	if cfg.AdminOtp == "" {
		RespError(w, RespParamErr, "OTP 未启用")
		return
	}

	// GET：仅返回启用状态，不需要生成二维码
	if r.Method == http.MethodGet {
		RespSucess(w, map[string]any{
			"enabled": true,
		})
		return
	}

	// POST：需提交密码+当前动态验证码，验证通过后返回密钥和二维码
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	var req struct {
		Password string `json:"password"`
		OtpCode  string `json:"otp_code"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Password == "" || req.OtpCode == "" {
		RespError(w, RespParamErr, "参数错误")
		return
	}

	if err := authsrv.CheckAdminPassword(cfg.AdminUser, req.Password); err != nil {
		RespError(w, RespUserOrPassErr)
		return
	}
	if err := authsrv.VerifyAdminOTP(req.OtpCode); err != nil {
		RespError(w, RespParamErr, "验证码错误")
		return
	}

	// 验证通过，生成二维码并返回密钥信息
	qrBase64, err := generateAdminOTPQrBase64(cfg.Issuer, cfg.AdminUser, cfg.AdminOtp)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	dbdata.AdminLog("安全设置", "两步验证", "查看了OTP密钥和二维码", r.RemoteAddr)
	RespSucess(w, map[string]any{
		"enabled":   true,
		"secret":    cfg.AdminOtp,
		"qr_base64": qrBase64,
	})
}

// 生成新的管理员 OTP 密钥（预览模式，不持久化）
func AdminOtpGenerate(w http.ResponseWriter, r *http.Request) {
	cfg := base.GetCfg()
	secret := gotp.RandomSecret(32)

	// 存储预览密钥，等待用户扫码验证
	otpPreviewMu.Lock()
	otpPreviewSecrets[cfg.AdminUser] = secret
	otpPreviewMu.Unlock()

	qrBase64, err := generateAdminOTPQrBase64(cfg.Issuer, cfg.AdminUser, secret)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	dbdata.AdminLog("安全设置", "两步验证", "生成了新的OTP密钥（预览）", r.RemoteAddr)
	RespSucess(w, map[string]any{
		"secret":    secret,
		"qr_base64": qrBase64,
	})
}

// AdminOtpConfirm 验证 OTP 动态码并启用两步验证。
// 首次绑定：仅需验证新密钥的动态码。
// 重新绑定（已启用 OTP 时）：额外要求输入管理员密码 + 当前 OTP 动态码。
func AdminOtpConfirm(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	var req struct {
		OtpCode        string `json:"otp_code"`
		Password       string `json:"password"`
		CurrentOtpCode string `json:"current_otp_code"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.OtpCode == "" {
		RespError(w, RespParamErr, "参数错误")
		return
	}

	cfg := base.GetCfg()

	// 取出预览密钥
	otpPreviewMu.Lock()
	secret := otpPreviewSecrets[cfg.AdminUser]
	otpPreviewMu.Unlock()

	if secret == "" {
		RespError(w, RespParamErr, "未找到 OTP 预览密钥，请重新生成")
		return
	}

	// 已启用 OTP 时，重新绑定额外要求验证管理员密码 + 当前 OTP
	if cfg.AdminOtp != "" {
		if req.Password == "" || req.CurrentOtpCode == "" {
			RespError(w, RespParamErr, "重新绑定两步验证需要输入管理员密码和当前动态验证码")
			return
		}
		if err := authsrv.CheckAdminPassword(cfg.AdminUser, req.Password); err != nil {
			RespError(w, RespUserOrPassErr, "管理员密码错误")
			return
		}
		if err := authsrv.VerifyAdminOTP(req.CurrentOtpCode); err != nil {
			RespError(w, RespParamErr, "当前动态验证码错误")
			return
		}
	}

	// 验证新密钥动态码（前后 ±60 秒容错）
	totp := gotp.NewDefaultTOTP(secret)
	now := time.Now().Unix()
	if !totp.Verify(req.OtpCode, now) &&
		!totp.Verify(req.OtpCode, now-30) && !totp.Verify(req.OtpCode, now+30) &&
		!totp.Verify(req.OtpCode, now-60) && !totp.Verify(req.OtpCode, now+60) {
		RespError(w, RespParamErr, "验证码错误，请重新生成密钥后再试")
		return
	}

	// 所有校验通过，删除预览密钥
	otpPreviewMu.Lock()
	delete(otpPreviewSecrets, cfg.AdminUser)
	otpPreviewMu.Unlock()

	// 验证通过，先构造新配置快照并持久化
	cfg = base.GetCfg()
	newCfg := *cfg
	newCfg.AdminOtp = secret
	if err := dbdata.SettingSaveServerConfigWith(&newCfg); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	// DB 保存成功后，原子切换到新配置
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.AdminOtp = secret
	})
	base.Info("管理员两步验证已启用")

	action := "启用了两步验证"
	if cfg.AdminOtp != "" {
		action = "重新绑定了两步验证密钥"
	}
	dbdata.AdminLog("安全设置", "两步验证", action, r.RemoteAddr)
	RespSucess(w, map[string]string{"message": "两步验证已启用"})
}

// 禁用管理员两步验证（必须输入当前动态验证码）
func AdminOtpDisable(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	var req struct {
		OtpCode string `json:"otp_code"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		RespError(w, RespParamErr, "参数错误")
		return
	}

	cfg := base.GetCfg()
	if cfg.AdminOtp == "" {
		RespError(w, RespParamErr, "OTP 未启用")
		return
	}

	// 必须输入当前动态验证码
	if req.OtpCode == "" {
		RespError(w, RespParamErr, "请输入当前动态验证码")
		return
	}
	if err := authsrv.VerifyAdminOTP(req.OtpCode); err != nil {
		RespError(w, RespParamErr, "验证码错误")
		return
	}

	// 清理预览密钥
	otpPreviewMu.Lock()
	delete(otpPreviewSecrets, cfg.AdminUser)
	otpPreviewMu.Unlock()

	// 先构造新配置快照并持久化
	newCfg := *cfg
	newCfg.AdminOtp = ""
	if err := dbdata.SettingSaveServerConfigWith(&newCfg); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	// DB 保存成功后，原子切换到新配置
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.AdminOtp = ""
	})
	base.Info("管理员两步验证已禁用")

	dbdata.AdminLog("安全设置", "两步验证", "禁用了两步验证", r.RemoteAddr)
	RespSucess(w, map[string]string{"message": "两步验证已禁用"})
}

// 生成管理员 OTP 二维码的 base64 编码
func generateAdminOTPQrBase64(issuer, adminUser, secret string) (string, error) {
	issuerEscaped := url.QueryEscape(issuer)
	qrstr := fmt.Sprintf("otpauth://totp/%s:%s?issuer=%s&secret=%s",
		issuerEscaped, adminUser, issuerEscaped, secret)
	qr, err := qrcode.New(qrstr, qrcode.High)
	if err != nil {
		return "", err
	}
	png, err := qr.PNG(300)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(png), nil
}

// 签发管理员登录 JWT token
func issueLoginJWT(w http.ResponseWriter, r *http.Request, adminUser string) {
	expiresAt := time.Now().Unix() + 3600*3
	jwtData := map[string]any{"admin_user": adminUser}
	tokenString, err := SetJwtData(jwtData, expiresAt)
	if err != nil {
		RespError(w, 1, err)
		return
	}

	// JWT token 签发成功，返回数据（不暴露前端）
	data := make(map[string]any)
	data["admin_user"] = adminUser
	data["expires_at"] = expiresAt
	data["admin_temp"] = base.GetCfg().AdminTemp

	ck := &http.Cookie{
		Name:     "jwt",
		Value:    tokenString,
		Path:     "/",
		HttpOnly: true,
		Secure:   isTLS(r),
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, ck)

	RespSucess(w, data)
}

// 验证当前登录态，返回管理员用户名
func AuthCheck(w http.ResponseWriter, r *http.Request) {
	jwtToken := getJwt(r)
	data, err := GetJwtData(jwtToken)
	if err != nil || base.GetCfg().AdminUser != fmt.Sprint(data["admin_user"]) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	RespSucess(w, map[string]any{
		"admin_user": data["admin_user"],
	})
}

// 管理员退出登录，清除 JWT HttpOnly Cookie
func Logout(w http.ResponseWriter, r *http.Request) {
	// 吊销当前 JWT，避免登出后令牌仍可用于管理后台
	if cc, err := r.Cookie("jwt"); err == nil && cc.Value != "" {
		RevokeJwtToken(cc.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isTLS(r),
		SameSite: http.SameSiteLaxMode,
	})
	RespSucess(w, nil)
}

// 判断请求是否通过 TLS/HTTPS 传输
func isTLS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// 从请求头、表单或 Cookie 中提取 JWT token
func getJwt(r *http.Request) string {
	jwtToken := r.Header.Get("Jwt")
	if jwtToken == "" {
		jwtToken = r.FormValue("jwt")
	}
	if jwtToken == "" {
		if cc, err := r.Cookie("jwt"); err == nil {
			jwtToken = cc.Value
		}
	}
	return jwtToken
}

func authMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		route := mux.CurrentRoute(r)
		name := route.GetName()
		if utils.InArrStr([]string{"login", "login_otp", "auth_check", "logout", "index", "static", "login_config"}, name) {
			// 不进行鉴权
			next.ServeHTTP(w, r)
			return
		}

		// 进行登陆鉴权
		jwtToken := getJwt(r)
		data, err := GetJwtData(jwtToken)
		if err != nil || base.GetCfg().AdminUser != fmt.Sprint(data["admin_user"]) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

func recoverHttp(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				stack := debug.Stack()
				base.Error(err, string(stack))
				// http.Error(w, "Internal Server Error", 500)
				RespError(w, 500, "Internal Server Error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
