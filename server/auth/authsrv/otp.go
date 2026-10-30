package authsrv

import (
	"fmt"
	"net"
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

// 限制 OTP（邮件/短信）验证码发送频率，防止客户端反复触发认证时无限发送
var (
	otpLimiter = struct {
		mu    sync.Mutex
		next  map[string]time.Time    // username → 下次允许发送时间
		ipWin map[string]*ipOtpWindow // 来源IP → 发送计数窗口
	}{next: make(map[string]time.Time), ipWin: make(map[string]*ipOtpWindow)}
)

// 记录某个来源 IP 在一个时间窗口内的 OTP 发送次数
type ipOtpWindow struct {
	count int
	reset time.Time
}

const (
	otpResendLimit    = 60 * time.Second // 同一用户重发间隔
	otpIPWindow       = 1 * time.Minute  // 来源 IP 发送计数窗口
	otpIPMaxPerWindow = 5                // 窗口内单 IP 最多发送条数（防批量轰炸多用户）
)

// 检查是否允许向该用户发送 OTP
func otpSendAllowed(username, remoteAddr string) (bool, int) {
	now := time.Now()
	otpLimiter.mu.Lock()
	defer otpLimiter.mu.Unlock()
	if t, ok := otpLimiter.next[username]; ok && now.Before(t) {
		return false, int(time.Until(t).Seconds())
	}
	if remoteAddr != "" {
		ip := remoteAddr
		if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
			ip = h
		}
		if w, ok := otpLimiter.ipWin[ip]; !ok || now.After(w.reset) {
			otpLimiter.ipWin[ip] = &ipOtpWindow{count: 1, reset: now.Add(otpIPWindow)}
		} else if w.count >= otpIPMaxPerWindow {
			return false, 0
		} else {
			w.count++
		}
	}
	otpLimiter.next[username] = now.Add(otpResendLimit)
	return true, 0
}

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
			if ok, wait := otpSendAllowed(ctx.Conn.Username, ctx.Conn.RemoteAddr); !ok {
				if wait > 0 {
					base.Warn("OTP验证码发送过于频繁，用户:", ctx.Conn.Username, "需等待", wait, "秒")
				} else {
					base.Warn("OTP验证码发送被来源IP限流，用户:", ctx.Conn.Username)
				}
			} else {
				base.Info("OTP首次请求，发送验证码到:", ctx.Conn.Username)
				ctx.GetOTP().Sent = true
				base.SafeGo(func() { sendOtpToUser(ub) })
			}
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
