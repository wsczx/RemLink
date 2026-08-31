package authsrv

import (
	"fmt"
	"sync"
	"time"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/xlzd/gotp"
)

var (
	userOtpMux = sync.Mutex{}
	userOtp    = map[string]time.Time{}
)

func init() {
	auth.Registry.Register("otp", func() auth.Authenticator {
		return &OTPAuth{}
	})

	// 每 10 秒清理超过 120 秒的旧 OTP 记录（与验证窗口 ±60s 对齐，跨度 120s）
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			userOtpMux.Lock()
			expire := time.Now().Add(-120 * time.Second)
			for k, v := range userOtp {
				if v.Before(expire) {
					delete(userOtp, k)
				}
			}
			userOtpMux.Unlock()
		}
	}()
}

func CheckOtp(name, otp, secret string) bool {
	key := fmt.Sprintf("%s:%s", name, otp)

	userOtpMux.Lock()
	defer userOtpMux.Unlock()

	if _, ok := userOtp[key]; ok {
		return false
	}

	totp := gotp.NewDefaultTOTP(secret)
	now := time.Now().Unix()
	// 前后 ±60 秒容错
	verify := totp.Verify(otp, now) ||
		totp.Verify(otp, now-30) || totp.Verify(otp, now+30) ||
		totp.Verify(otp, now-60) || totp.Verify(otp, now+60)
	if verify {
		userOtp[key] = time.Now()
	}
	return verify
}

type OTPAuth struct {
	auth.NopChallenger
}

func (a *OTPAuth) Name() string { return "otp" }

func (a *OTPAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	// 确保用户信息已加载：首步非 local 时由 Authenticate 预加载，否则在此通过 LoadUserInfo 加载一次。
	if ctx.UserInfoLoaded() == nil {
		LoadUserInfo(ctx)
	}
	ub := ctx.UserInfoLoaded()
	if ub == nil {
		base.Warn("OTP验证失败：用户不在本地库中:", ctx.Conn.Username)
		return auth.StepFail, fmt.Errorf("用户 %s 未在本地数据库中找到，无法进行 OTP 验证，请先将该用户同步到本地并开启 OTP 动态验证", ctx.Conn.Username)
	}
	otpSecret := ub.OtpSecret
	disabled := ub.DisableOtp

	base.Debug("OTP验证开始: user=", ctx.Conn.Username, "secret_len=", len(otpSecret), "disabled=", disabled)

	if disabled {
		base.Warn("OTP被用户级 DisableOtp 跳过（组要求 OTP 但用户已禁用）: user=", ctx.Conn.Username, "group=", ctx.Conn.GroupName)
		return auth.StepPass, nil
	}
	if otpSecret == "" {
		base.Warn("OTP验证失败：未配置密钥 user=", ctx.Conn.Username)
		return auth.StepFail, fmt.Errorf("用户 %s 未启用 OTP 动态验证，请联系管理员", ctx.Conn.Username)
	}

	otp := ctx.OTP
	// 独立窗口模式
	code := ""
	sent := false
	if otp != nil {
		code = otp.Code
		sent = otp.Sent
	}
	base.Debug("OTP独立窗口模式: user=", ctx.Conn.Username, "code_len=", len(code), "send_otp=", base.GetCfg().SendOtp)
	if code == "" {
		if base.GetCfg().SendOtp && !sent {
			base.Info("OTP首次请求，发送验证码到:", ctx.Conn.Username)
			ctx.GetOTP().Sent = true
			go sendOtpToUser(ub)
		}
		return auth.StepPending, nil
	}

	if !CheckOtp(ub.Username, code, otpSecret) {
		// 保留会话允许重试，锁定计数由上层 LockManager 处理。
		ctx.GetOTP().Code = "" // 清除旧码，等待用户重新输入
		base.Warn("OTP验证失败：动态码错误 user=", ctx.Conn.Username)
		return auth.StepPending, nil
	}

	base.Info("OTP验证成功: user=", dbdata.UserLabel(ctx.Conn.Username, ctx.Conn.Nickname))
	return auth.StepPass, nil
}

func (a *OTPAuth) Challenge() *auth.ChallengeInfo {
	return &auth.ChallengeInfo{
		Type:     auth.ChallengeOTP,
		Template: "otp",
		Data:     map[string]any{},
	}
}

// SendOtpFunc 由 handler 层注入，避免 auth 与 handler 循环依赖。
var SendOtpFunc func(info *auth.UserInfo) error

func sendOtpToUser(info *auth.UserInfo) {
	if SendOtpFunc == nil {
		base.Warn("SendOtpFunc 未注入，无法发送 OTP")
		return
	}
	if err := SendOtpFunc(info); err != nil {
		base.Error("发送 OTP 失败:", err)
		return
	}
	base.Info(info.Username, "OTP 验证码已发送")
}
