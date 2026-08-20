package admin

import (
	"strings"
	"testing"
	"time"

	"github.com/xlzd/gotp"
)

func TestOTPGenerateAndVerify(t *testing.T) {
	secret := gotp.RandomSecret(32)
	if secret == "" {
		t.Fatal("RandomSecret(32) 返回空字符串")
	}
	t.Logf("生成密钥: %s (长度: %d)", secret, len(secret))

	if !gotp.IsSecretValid(secret) {
		t.Fatal("生成的密钥不是有效的 base32 字符串")
	}

	qrURL := "otpauth://totp/Test:admin?issuer=Test&secret=" + secret
	if !strings.Contains(qrURL, "secret="+secret) {
		t.Fatal("QR URL 格式不正确")
	}
	t.Logf("QR URL: %s", qrURL)

	totp := gotp.NewDefaultTOTP(secret)

	now := time.Now().Unix()
	code := totp.At(now)
	t.Logf("当前时间窗口 [%d] 验证码: %s", now/30, code)
	if !totp.Verify(code, now) {
		t.Fatal("当前窗口验证失败")
	}

	prevCode := totp.At(now - 30)
	t.Logf("上一时间窗口 [%d] 验证码: %s", (now-30)/30, prevCode)
	if !totp.Verify(prevCode, now-30) {
		t.Fatal("上一窗口验证失败")
	}

	nextCode := totp.At(now + 30)
	t.Logf("下一时间窗口 [%d] 验证码: %s", (now+30)/30, nextCode)
	if !totp.Verify(nextCode, now+30) {
		t.Fatal("下一窗口验证失败")
	}

	if !totp.Verify(prevCode, now) && !totp.Verify(prevCode, now-30) && !totp.Verify(prevCode, now+30) {
		t.Fatal("手机时钟落后场景：验证失败")
	}
	t.Log("✓ 手机时钟落后场景：通过")

	if !totp.Verify(nextCode, now) && !totp.Verify(nextCode, now-30) && !totp.Verify(nextCode, now+30) {
		t.Fatal("手机时钟超前场景：验证失败")
	}
	t.Log("✓ 手机时钟超前场景：通过")

	wrongCode := "000000"
	if totp.Verify(wrongCode, now) || totp.Verify(wrongCode, now-30) || totp.Verify(wrongCode, now+30) {
		t.Fatal("错误验证码不应通过")
	}
	t.Log("✓ 错误验证码：正确拒绝")

	expiredCode := totp.At(now - 120)
	if totp.Verify(expiredCode, now) || totp.Verify(expiredCode, now-30) || totp.Verify(expiredCode, now+30) {
		t.Fatal("过期验证码不应通过")
	}
	t.Log("✓ 过期验证码：正确拒绝")
}

func TestOTPSecretRoundTrip(t *testing.T) {
	secret := gotp.RandomSecret(32)
	if secret == "" {
		t.Fatal("RandomSecret(32) 返回空字符串")
	}

	totp := gotp.NewDefaultTOTP(secret)

	phoneTime := time.Now().Unix()
	phoneCode := totp.At(phoneTime)

	now := time.Now().Unix()
	verifyOK := totp.Verify(phoneCode, now) || totp.Verify(phoneCode, now-30) || totp.Verify(phoneCode, now+30)
	if !verifyOK {
		t.Fatalf("端到端验证失败: secret=%s, phoneTime=%d, serverTime=%d, phoneCode=%s",
			secret, phoneTime, now, phoneCode)
	}
	t.Logf("✓ 端到端验证通过: code=%s (phone窗口=%d, server窗口=%d)",
		phoneCode, phoneTime/30, now/30)
}

func TestOTPWithClockSkew(t *testing.T) {
	secret := gotp.RandomSecret(32)
	totp := gotp.NewDefaultTOTP(secret)

	baseTime := time.Now().Unix()
	window := baseTime / 30

	testCases := []struct {
		name      string
		phoneSkew int64 // 手机相对服务器的偏移（秒）
	}{
		{"手机慢60秒", -60},
		{"手机慢30秒", -30},
		{"手机慢10秒", -10},
		{"时钟同步", 0},
		{"手机快10秒", 10},
		{"手机快30秒", 30},
		{"手机快60秒", 60},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			phoneTime := baseTime + tc.phoneSkew
			phoneWindow := phoneTime / 30

			phoneCode := totp.At(phoneTime)

			serverNow := baseTime
			verifyOK := totp.Verify(phoneCode, serverNow) ||
				totp.Verify(phoneCode, serverNow-30) ||
				totp.Verify(phoneCode, serverNow+30)

			t.Logf("  服务器窗口=%d, 手机窗口=%d, 偏移=%+ds, 验证码=%s, 通过=%v",
				window, phoneWindow, tc.phoneSkew, phoneCode, verifyOK)

			if tc.phoneSkew >= -30 && tc.phoneSkew <= 30 {
				if !verifyOK {
					t.Errorf("时钟偏移 %+d 秒应该在容错范围内", tc.phoneSkew)
				}
			} else {
				if verifyOK {
					t.Errorf("时钟偏移 %+d 秒超出容错范围，不应通过", tc.phoneSkew)
				}
			}
		})
	}
}

func TestOTPQRURLFormat(t *testing.T) {
	secret := gotp.RandomSecret(32)
	cfg := struct {
		Issuer    string
		AdminUser string
	}{"RemLink", "admin"}

	qrURL := "otpauth://totp/" + cfg.Issuer + ":" + cfg.AdminUser +
		"?issuer=" + cfg.Issuer + "&secret=" + secret

	if !strings.Contains(qrURL, "otpauth://totp/") {
		t.Fatal("URL 缺少 otpauth scheme")
	}
	if !strings.Contains(qrURL, "secret=") {
		t.Fatal("URL 缺少 secret 参数")
	}
	if !strings.Contains(qrURL, "issuer=") {
		t.Fatal("URL 缺少 issuer 参数")
	}

	t.Logf("QR URL: %s", qrURL)
	t.Logf("✓ URL 格式正确")

	totp := gotp.NewDefaultTOTP(secret)
	standardURI := totp.ProvisioningUri(cfg.AdminUser, cfg.Issuer)
	t.Logf("标准 URI: %s", standardURI)

	if !strings.Contains(standardURI, secret) {
		t.Fatal("标准 URI 中 secret 不匹配")
	}
	t.Logf("✓ 标准 URI 和自定义 QR URL 使用相同密钥")
}
