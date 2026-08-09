package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/security"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xlzd/gotp"
)

func TestSessionStore(t *testing.T) {
	ast := assert.New(t)

	// 测试会话存储基本功能
	sessionID := "test-session-123"

	// 创建测试会话数据
	authSession := &AuthSession{
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{Username: "test-user", GroupName: "test-group"},
		},
		UserActLog: &dbdata.UserActLog{
			Username: "test-user",
			Status:   dbdata.UserAuthSuccess,
		},
	}

	// 测试保存会话
	AuthSessionManager.Save(sessionID, authSession)

	// 测试获取会话
	retrievedSession, err := AuthSessionManager.Get(sessionID)
	ast.Nil(err)
	ast.NotNil(retrievedSession)
	ast.Equal("test-user", retrievedSession.Ctx.Conn.Username)

	// 测试获取不存在的会话
	_, err = AuthSessionManager.Get("nonexistent-session")
	ast.NotNil(err)
	ast.Contains(err.Error(), "会话未找到")

	// 测试删除会话
	AuthSessionManager.Delete(sessionID)
	_, err = AuthSessionManager.Get(sessionID)
	ast.NotNil(err)
}

func TestGenerateSessionID(t *testing.T) {
	ast := assert.New(t)

	// 测试会话ID生成
	sessionID := GenerateSessionID()
	ast.NotEmpty(sessionID)
	ast.Equal(32, len(sessionID))

	// 测试生成的ID唯一性
	sessionID2 := GenerateSessionID()
	ast.NotEqual(sessionID, sessionID2)
}

func TestCookieOperations(t *testing.T) {
	ast := assert.New(t)

	// 测试设置和获取Cookie
	w := httptest.NewRecorder()
	SetCookie(w, "test-cookie", "test-value", 3600)

	cookies := w.Result().Cookies()
	ast.Equal(1, len(cookies))
	ast.Equal("test-cookie", cookies[0].Name)
	ast.Equal("test-value", cookies[0].Value)
	ast.True(cookies[0].HttpOnly)
	ast.True(cookies[0].Secure)

	// 测试从请求中获取Cookie
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookies[0])

	value, err := GetCookie(req, "test-cookie")
	ast.Nil(err)
	ast.Equal("test-value", value)

	// 测试获取不存在的Cookie
	_, err = GetCookie(req, "nonexistent-cookie")
	ast.NotNil(err)

	// 测试删除Cookie
	w2 := httptest.NewRecorder()
	DeleteCookie(w2, "test-cookie")
	deleteCookies := w2.Result().Cookies()
	ast.Equal(1, len(deleteCookies))
	ast.Equal("test-cookie", deleteCookies[0].Name)
	ast.Equal("", deleteCookies[0].Value)
	ast.Equal(-1, deleteCookies[0].MaxAge)
}

func TestLinkAuthOtp(t *testing.T) {
	base.Test()
	ast := assert.New(t)

	base.SetCfgForTest(&base.ServerConfig{DisplayError: true})

	// 设置测试数据库
	preIpData(t)
	defer closeIpdata()

	// 创建测试策略
	dns := []dbdata.ValData{{Val: "8.8.8.8"}}
	pt := dbdata.Policy{Name: "otp-test-policy", Status: 1, ClientDns: dns}
	err := dbdata.SetPolicy(&pt)
	ast.Nil(err)
	// 创建测试组（配置 [local, otp] 认证管线）
	group := "otp-test-group"
	profile := auth.GroupAuthProfile{
		Step: []auth.AuthMethodConfig{
			{Type: "local"},
			{Type: "otp"},
		},
	}
	profileBytes, _ := json.Marshal(profile)
	g := dbdata.Group{Name: group, Status: 1, PolicyId: pt.Id, AuthProfile: profileBytes}
	err = dbdata.SetGroup(&g)
	ast.Nil(err)

	// 创建测试用户
	username := "otp-test-user"
	otpSecret := "JBSWY3DPEHPK3PXP"
	u := dbdata.User{
		Username:  username,
		Groups:    []string{group},
		Status:    1,
		OtpSecret: otpSecret,
	}
	err = dbdata.SetUser(&u)
	ast.Nil(err)

	// 生成有效的OTP代码
	totp := gotp.NewDefaultTOTP(otpSecret)
	validOtp := totp.Now()

	// 创建测试会话（模拟 local 步骤已通过，停在 OTP 步骤）
	sessionID := "test-otp-session"
	authSession := &AuthSession{
		Ctx: &auth.Context{
			Conn:     auth.ConnInfo{Username: username, GroupName: group, UserAgent: "test-agent"},
			UserInfo: &auth.UserInfo{OtpSecret: otpSecret},
		},
		UserActLog: &dbdata.UserActLog{
			Username: username,
			Status:   dbdata.UserAuthSuccess,
		},
	}
	authSession.Ctx.SetStepIdx(2) // 含 forcepwd 插入后索引：local=0, forcepwd=1, otp=2
	AuthSessionManager.Save(sessionID, authSession)

	// 测试成功的OTP验证
	t.Run("ValidOTP", func(t *testing.T) {
		ast := assert.New(t)

		// 创建OTP验证请求
		clientReq := ClientRequest{
			Auth: authData{
				SecondaryPassword: validOtp,
			},
		}
		reqBody, _ := xml.Marshal(clientReq)

		req := httptest.NewRequest("POST", "/otp-verification", bytes.NewReader(reqBody))
		req.AddCookie(&http.Cookie{Name: "auth-session-id", Value: sessionID})
		w := httptest.NewRecorder()

		LinkAuth_otp(w, req)

		ast.Equal(http.StatusOK, w.Code)
		// 验证会话已被删除
		_, err := AuthSessionManager.Get(sessionID)
		ast.NotNil(err)
	})

	// 测试无效的OTP代码
	t.Run("InvalidOTP", func(t *testing.T) {
		ast := assert.New(t)

		// 重新创建会话（因为上一个测试中被删除了）
		AuthSessionManager.Save(sessionID+"2", authSession)

		clientReq := ClientRequest{
			Auth: authData{
				SecondaryPassword: "123456", // 无效的OTP
			},
		}
		reqBody, _ := xml.Marshal(clientReq)

		req := httptest.NewRequest("POST", "/otp-verification", bytes.NewReader(reqBody))
		req.AddCookie(&http.Cookie{Name: "auth-session-id", Value: sessionID + "2"})
		w := httptest.NewRecorder()

		LinkAuth_otp(w, req)

		ast.Equal(http.StatusOK, w.Code)
		// OTP 错误返回 StepPending 重试（tpl_otp 渲染），包含 "动态码错误" 提示
		ast.Contains(w.Body.String(), "动态码错误")
		// 验证会话仍然存在（未被删除，允许重试）
		_, err := AuthSessionManager.Get(sessionID + "2")
		ast.Nil(err, "OTP 错误应保留会话以允许重试")
	})

	// 测试无效会话
	t.Run("InvalidSession", func(t *testing.T) {
		ast := assert.New(t)

		clientReq := ClientRequest{
			Auth: authData{
				SecondaryPassword: validOtp,
			},
		}
		reqBody, _ := xml.Marshal(clientReq)

		req := httptest.NewRequest("POST", "/otp-verification", bytes.NewReader(reqBody))
		req.AddCookie(&http.Cookie{Name: "auth-session-id", Value: "invalid-session"})
		w := httptest.NewRecorder()

		LinkAuth_otp(w, req)

		ast.Equal(http.StatusUnauthorized, w.Code)
	})

	// 测试缺少会话Cookie
	t.Run("MissingSessionCookie", func(t *testing.T) {
		ast := assert.New(t)

		clientReq := ClientRequest{
			Auth: authData{
				SecondaryPassword: validOtp,
			},
		}
		reqBody, _ := xml.Marshal(clientReq)

		req := httptest.NewRequest("POST", "/otp-verification", bytes.NewReader(reqBody))
		w := httptest.NewRecorder()

		LinkAuth_otp(w, req)

		ast.Equal(http.StatusUnauthorized, w.Code)
	})
}

func TestHandleSsoTokenPreservesIdentityForOtpPending(t *testing.T) {
	base.Test()
	ast := assert.New(t)

	base.SetCfgForTest(&base.ServerConfig{DisplayError: true})
	preIpData(t)
	defer closeIpdata()

	// 创建测试策略
	pt := &dbdata.Policy{
		Name:      "sso-otp-test-policy",
		ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}},
		Status:    1,
	}
	err := dbdata.SetPolicy(pt)
	ast.Nil(err)

	group := "sso-otp-test-group"

	// 创建测试 wxwork Provider
	_ = dbdata.SetProvider(&dbdata.Provider{
		Name:   "test-wxwork-sso-otp",
		Type:   "wxwork",
		Status: 1,
		Config: security.EncryptedJSON[json.RawMessage]{Data: json.RawMessage(`{"corp_id":"test","agent_id":"test","secret":"test"}`)},
	})

	profile := auth.GroupAuthProfile{
		Step: []auth.AuthMethodConfig{
			{Type: "wxwork", Provider: "test-wxwork-sso-otp"},
			{Type: "otp"},
		},
	}
	profileBytes, _ := json.Marshal(profile)
	err = dbdata.SetGroup(&dbdata.Group{
		Name:        group,
		Status:      1,
		PolicyId:    pt.Id,
		AuthProfile: profileBytes,
	})
	ast.Nil(err)

	username := "sso-otp-user"
	otpSecret := "JBSWY3DPEHPK3PXP"
	err = dbdata.SetUser(&dbdata.User{
		Username:  username,
		Groups:    []string{group},
		Status:    1,
		OtpSecret: otpSecret,
	})
	ast.Nil(err)

	ssoToken := "test-sso-token"
	AuthSessionManager.Save(ssoToken, &AuthSession{
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{Username: username, GroupName: group},
			SSO: &auth.SSOState{
				Authenticated: true,
				UserID:        username,
			},
		},
	})

	encodedToken := base64.StdEncoding.EncodeToString([]byte(ssoToken))
	cr := &ClientRequest{
		RemoteAddr: "192.168.1.100:12345",
		UserAgent:  "cisco anyconnect vpn agent",
		Auth: authData{
			SsoToken: encodedToken,
		},
	}
	sessionData := &AuthSession{
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{RemoteAddr: "192.168.1.100:12345", UserAgent: "cisco anyconnect vpn agent"},
		},
		UserActLog: &dbdata.UserActLog{
			RemoteAddr: "192.168.1.100:12345",
			Status:     dbdata.UserAuthSuccess,
		},
	}

	req := httptest.NewRequest("POST", "/", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	handleSsoToken(w, req, cr, sessionData)

	ast.Equal(http.StatusOK, w.Code)
	// SSO 回调后走管道：SSO 步骤 StepPass → OTP 步骤触发挑战（组 Profile: [otp]）
	ast.Contains(w.Body.String(), "secondary_password")
	ast.NotContains(w.Body.String(), "<session-id>")

	// SSO 临时会话应在处理后清理
	_, err = AuthSessionManager.Get(ssoToken)
	ast.NotNil(err, "SSO 临时会话应在处理后清理")
}

func TestCreateSession(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("在GitHub Actions中跳过此测试")
		return
	}
	base.Test()
	ast := assert.New(t)

	preIpData(t)
	defer closeIpdata()

	other := &dbdata.SettingOther{Banner: "测试横幅内容", BannerEnable: true}
	err := dbdata.SettingSave(other)
	ast.Nil(err)

	// 创建测试数据
	group := "session-test-group"
	username := "session-test-user"

	dns := []dbdata.ValData{{Val: "8.8.8.8"}}
	pt := &dbdata.Policy{Name: "session-test-policy", Status: 1, ClientDns: dns}
	err = dbdata.SetPolicy(pt)
	ast.Nil(err)
	g := dbdata.Group{Name: group, Status: 1, PolicyId: pt.Id}
	err = dbdata.SetGroup(&g)
	ast.Nil(err)

	u := dbdata.User{Username: username, Groups: []string{group}, Status: 1}
	err = dbdata.SetUser(&u)
	ast.Nil(err)

	// 创建认证会话数据
	authSession := &AuthSession{
		Ctx: &auth.Context{
			Conn: auth.ConnInfo{
				Username:   username,
				GroupName:  group,
				UserAgent:  "test-agent",
				DeviceID:   "test-device-id",
				MacAddr:    "00:11:22:33:44:55",
				RemoteAddr: "192.168.1.100",
			},
		},
		UserActLog: &dbdata.UserActLog{
			Username:        username,
			Status:          dbdata.UserAuthSuccess,
			DeviceType:      "test-device",
			PlatformVersion: "test-platform",
		},
	}

	// 测试会话创建
	w := httptest.NewRecorder()

	CreateSession(w, authSession)

	ast.Equal(http.StatusOK, w.Code)
	// 验证响应包含会话信息
	ast.Contains(w.Body.String(), "session-token")
	ast.Contains(w.Body.String(), "测试横幅内容")

	// 关闭 Banner 后不显示横幅
	other.BannerEnable = false
	err = dbdata.SettingSave(other)
	ast.Nil(err)

	w2 := httptest.NewRecorder()
	CreateSession(w2, authSession)

	ast.Equal(http.StatusOK, w2.Code)
	ast.NotContains(w2.Body.String(), "测试横幅内容")
}

func preIpData(t *testing.T) {
	// 设置测试模式
	base.Test()

	// 每个测试独立的临时数据库，消除跨测试污染
	tmpDb := path.Join(t.TempDir(), "remlink_otp_test.db")

	// 设置数据库配置
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.DbType = "sqlite3"
		c.DbSource = tmpDb
		c.Ipv4CIDR = "192.168.3.0/24"
		c.Ipv4Gateway = "192.168.3.1"
		c.Ipv4Start = "192.168.3.100"
		c.Ipv4End = "192.168.3.150"
		c.MaxClient = 100
		c.MaxUserClient = 3
		c.IpLease = 5
	})

	// 启动数据库
	dbdata.Start()
}

func closeIpdata() {
	// 释放异步日志 worker pool（UserActLogIns 使用 grpool 异步写 DB）
	dbdata.UserActLogIns.Pool.Release()
	dbdata.UserActLogIns.Pool = utils.NewWorkerPool(1, 100)
	// 关闭全局数据库
	_ = dbdata.Stop()
	// DB 文件由 t.TempDir() 自动清理，无需手动 os.Remove
}
