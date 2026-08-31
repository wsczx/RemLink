package authsrv

import (
	"fmt"
	"sync"
	"time"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xlzd/gotp"
)

// 管理员 OTP 动态码防重放攻击。
var (
	adminOtpUsed   = make(map[string]time.Time)
	adminOtpUsedMu sync.Mutex
)

func init() {
	auth.Registry.Register("admin", func() auth.Authenticator {
		return &AdminAuth{}
	})

	// 每 10 秒清理超过 90 秒的旧 OTP 记录
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			adminOtpUsedMu.Lock()
			expire := time.Now().Add(-90 * time.Second)
			for code, t := range adminOtpUsed {
				if t.Before(expire) {
					delete(adminOtpUsed, code)
				}
			}
			adminOtpUsedMu.Unlock()
		}
	}()
}

type AdminAuth struct {
	auth.NopChallenger
}

func (a *AdminAuth) Name() string { return "admin" }

func CheckAdminPassword(username, password string) error {
	cfg := base.GetCfg()
	if username != cfg.AdminUser || !utils.PasswordVerify(password, cfg.AdminPass) {
		return fmt.Errorf("管理员用户名或密码错误")
	}
	return nil
}

func VerifyAdminOTP(otpCode string) error {
	cfg := base.GetCfg()
	if cfg.AdminOtp == "" {
		return nil // 未启用 OTP，直接通过
	}

	// 重放防护：同一 OTP 码只能使用一次
	adminOtpUsedMu.Lock()
	if _, used := adminOtpUsed[otpCode]; used {
		adminOtpUsedMu.Unlock()
		return fmt.Errorf("管理员 OTP 验证失败：动态码已被使用")
	}
	adminOtpUsedMu.Unlock()

	totp := gotp.NewDefaultTOTP(cfg.AdminOtp)
	now := time.Now().Unix()
	// 前后 ±60 秒容错
	if !totp.Verify(otpCode, now) &&
		!totp.Verify(otpCode, now-30) && !totp.Verify(otpCode, now+30) &&
		!totp.Verify(otpCode, now-60) && !totp.Verify(otpCode, now+60) {
		return fmt.Errorf("管理员 OTP 验证失败")
	}

	adminOtpUsedMu.Lock()
	adminOtpUsed[otpCode] = time.Now()
	adminOtpUsedMu.Unlock()

	return nil
}

// OTP 验证不由本方法处理：Web 登录走两步 Login/LoginOTP 流程，
// Pipeline 中如需 OTP 应配置独立的 "otp" step。
func (a *AdminAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	if err := CheckAdminPassword(ctx.Conn.Username, ctx.Conn.Password); err != nil {
		return auth.StepFail, err
	}

	ctx.SetInfo("管理员登录成功")
	return auth.StepPass, nil
}
