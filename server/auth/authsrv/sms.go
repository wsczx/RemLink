package authsrv

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/notify"
)

// smsCode 验证码缓存
type smsCode struct {
	code    string
	expires time.Time
	retries int // 允许重试次数（独立于锁定）
	user    string
}

var (
	smsCodes   = make(map[string]*smsCode)
	smsCodeMu  sync.Mutex
	smsLimiter = struct {
		mu    sync.Mutex
		next  map[string]time.Time    // phone → 下次允许发送时间
		ipWin map[string]*ipSmsWindow // 来源IP → 发送计数窗口（防多目标短信轰炸）
	}{next: make(map[string]time.Time), ipWin: make(map[string]*ipSmsWindow)}
)

// 记录某个来源 IP 在一个时间窗口内的短信发送次数
type ipSmsWindow struct {
	count int
	reset time.Time
}

const (
	smsCodeLength     = 6
	smsCodeTTL        = 5 * time.Minute
	smsIPWindow       = 1 * time.Minute // 来源 IP 发送计数窗口
	smsIPMaxPerWindow = 5               // 窗口内单 IP 最多发送条数（防多目标短信轰炸）

	smsResendLimit  = 60 * time.Second // 重发间隔
	smsMaxRetries   = 5                // 单个验证码最多验证次数
	smsCleanupEvery = 30 * time.Second
)

func init() {
	auth.Registry.Register("sms", func() auth.Authenticator {
		return &SmsAuth{}
	})

	go func() {
		ticker := time.NewTicker(smsCleanupEvery)
		defer ticker.Stop()
		for range ticker.C {
			smsCodeMu.Lock()
			now := time.Now()
			for k, v := range smsCodes {
				if now.After(v.expires) {
					delete(smsCodes, k)
				}
			}
			smsCodeMu.Unlock()
		}
	}()
}

// 生成 6 位随机数字验证码
func generateSmsCode() string {
	code := make([]byte, smsCodeLength)
	for i := range smsCodeLength {
		n, _ := rand.Int(rand.Reader, big.NewInt(10))
		code[i] = byte('0') + byte(n.Int64())
	}
	return string(code)
}

// SendSmsCode 发送短信验证码（供 Portal API 直接调用）
// 返回验证码和错误，调用方不应向外部泄露 code。
func SendSmsCode(phone string, fromIP ...string) (string, error) {
	now := time.Now()
	// 同一手机号 60 秒内只能发一次
	smsLimiter.mu.Lock()
	if t, ok := smsLimiter.next[phone]; ok && now.Before(t) {
		smsLimiter.mu.Unlock()
		return "", fmt.Errorf("发送过于频繁，请 %d 秒后重试", int(time.Until(t).Seconds()))
	}
	smsLimiter.next[phone] = now.Add(smsResendLimit)

	// 同一来源 IP 在窗口内最多发 smsIPMaxPerWindow 次，防止一个 IP 批量轰炸多个手机号
	if len(fromIP) > 0 {
		ip := fromIP[0]
		if h, _, err := net.SplitHostPort(ip); err == nil {
			ip = h
		}
		if ip != "" {
			w, ok := smsLimiter.ipWin[ip]
			if !ok || now.After(w.reset) {
				smsLimiter.ipWin[ip] = &ipSmsWindow{count: 1, reset: now.Add(smsIPWindow)}
			} else if w.count >= smsIPMaxPerWindow {
				smsLimiter.mu.Unlock()
				return "", fmt.Errorf("发送过于频繁，请稍后重试")
			} else {
				w.count++
			}
		}
	}
	smsLimiter.mu.Unlock()

	code := generateSmsCode()

	err := notify.GetNotify().SendSms(notify.Message{
		To:     phone,
		Body:   "",
		Params: map[string]string{"1": code, "2": "5"},
	})
	if err != nil {
		return "", fmt.Errorf("短信发送失败: %w", err)
	}

	smsCodeMu.Lock()
	smsCodes[phone] = &smsCode{
		code:    code,
		expires: time.Now().Add(smsCodeTTL),
		retries: 0,
		user:    "",
	}
	smsCodeMu.Unlock()

	base.Info("短信验证码已发送到:", phone)
	return code, nil
}

// 验证短信验证码
func VerifySmsCode(phone, code string) (string, error) {
	smsCodeMu.Lock()
	defer smsCodeMu.Unlock()

	cached, ok := smsCodes[phone]
	if !ok {
		return "", fmt.Errorf("验证码不存在或已过期，请重新获取")
	}
	if time.Now().After(cached.expires) {
		delete(smsCodes, phone)
		return "", fmt.Errorf("验证码已过期，请重新获取")
	}
	cached.retries++
	if cached.retries > smsMaxRetries {
		delete(smsCodes, phone)
		return "", fmt.Errorf("验证码错误次数过多，请重新获取")
	}

	if cached.code != code {
		return "", fmt.Errorf("验证码错误")
	}

	username := cached.user
	delete(smsCodes, phone)
	return username, nil
}

// SmsAuth 短信验证码认证器，作为管道中的一步。
// 首次进入发送验证码并返回 StepPending；恢复时校验用户输入的验证码。
type SmsAuth struct {
	auth.NopChallenger
}

func (a *SmsAuth) Name() string { return "sms" }

func (a *SmsAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	sms := ctx.GetSMS()
	if sms.Phone == "" {
		// 优先从已加载的 UserInfo 读取手机号；未加载时通过 LoadUserInfo 加载一次并缓存。
		if ub := ctx.UserInfoLoaded(); ub != nil && ub.Phone != "" {
			sms.Phone = ub.Phone
		} else if ctx.Conn.Username != "" {
			LoadUserInfo(ctx)
			if ub := ctx.UserInfoLoaded(); ub != nil && ub.Phone != "" {
				sms.Phone = ub.Phone
			}
		}
		if sms.Phone == "" {
			return auth.StepFail, fmt.Errorf("未找到用户手机号，无法发送短信验证码")
		}
		sms.Sent = false
	}

	if sms.Code == "" {
		if !sms.Sent {
			code, err := SendSmsCode(sms.Phone)
			if err != nil {
				return auth.StepFail, fmt.Errorf("短信验证码发送失败: %w", err)
			}
			smsCodeMu.Lock()
			if c, ok := smsCodes[sms.Phone]; ok {
				c.user = ctx.Conn.Username
			}
			smsCodeMu.Unlock()
			sms.Sent = true
			_ = code // 不暴露到日志
			base.Info("SMS认证: 验证码已发送到 ", sms.Phone)
		}
		return auth.StepPending, nil
	}

	_, err := VerifySmsCode(sms.Phone, sms.Code)
	if err != nil {
		sms.Code = "" // 清除旧码，允许重试
		return auth.StepPending, nil
	}
	base.Info("SMS认证成功: user=", dbdata.UserLabel(ctx.Conn.Username, ctx.Conn.Nickname))
	return auth.StepPass, nil
}

func (a *SmsAuth) Challenge() *auth.ChallengeInfo {
	return &auth.ChallengeInfo{
		Type:     auth.ChallengeSMS,
		Template: "sms",
		Data:     map[string]any{},
	}
}
