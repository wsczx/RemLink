package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/admin"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xlzd/gotp"
)

func TestValidatePasswordStrength(t *testing.T) {
	ast := assert.New(t)

	t.Run("ValidPassword", func(t *testing.T) {
		ast.Nil(utils.CheckPasswordPolicy("abcd1234"))
		ast.Nil(utils.CheckPasswordPolicy("ABCD1234"))
		ast.Nil(utils.CheckPasswordPolicy("a1b2c3d4"))
		ast.Nil(utils.CheckPasswordPolicy("Password1"))
	})

	t.Run("TooShort", func(t *testing.T) {
		err := utils.CheckPasswordPolicy("abc1234") // 7 位
		ast.NotNil(err)
		ast.Contains(err.Error(), "不能少于 8 位")
	})

	t.Run("NoDigit", func(t *testing.T) {
		err := utils.CheckPasswordPolicy("abcdefgh")
		ast.NotNil(err)
		ast.Contains(err.Error(), "字母和数字")
	})

	t.Run("NoLetter", func(t *testing.T) {
		err := utils.CheckPasswordPolicy("12345678")
		ast.NotNil(err)
		ast.Contains(err.Error(), "字母和数字")
	})

	t.Run("ChineseChars", func(t *testing.T) {
		// 中文字符不算字母
		err := utils.CheckPasswordPolicy("中文密码1234")
		ast.NotNil(err)
		ast.Contains(err.Error(), "字母和数字")
	})
}

func createPortalLoginSetup(t *testing.T, username, password, groupName string) {
	ast := assert.New(t)

	pt := &dbdata.Policy{
		Name:      "plcy-" + groupName,
		ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}},
		Status:    1,
	}
	err := dbdata.SetPolicy(pt)
	ast.Nil(err)

	g := &dbdata.Group{
		Name:        groupName,
		Status:      1,
		PolicyId:    pt.Id,
		AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`),
	}
	err = dbdata.SetGroup(g)
	ast.Nil(err)

	hashedPwd, err := utils.PasswordHash(password)
	ast.Nil(err)
	u := &dbdata.User{
		Username:   username,
		PinCode:    hashedPwd,
		Groups:     []string{groupName},
		Status:     1,
		DisableOtp: true,
	}
	err = dbdata.SetUser(u)
	ast.Nil(err)
}

func TestPortalLogin_Success(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()
	base.UpdateCfg(func(c *base.ServerConfig) { c.EnableUserPortal = true })

	createPortalLoginSetup(t, "testuser", "testpass123", "default")
	ast := assert.New(t)

	body := `{"username":"testuser","password":"testpass123"}`
	req := httptest.NewRequest("POST", "/portal/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	PortalLogin(w, req)

	ast.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	err := json.NewDecoder(w.Body).Decode(&resp)
	ast.Nil(err)
	ast.Equal(float64(0), resp["code"])

	data := resp["data"].(map[string]any)
	ast.Equal("pass", data["status"])
	ast.NotEmpty(data["token"])
}

func TestPortalLogin_WrongPassword(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()
	base.UpdateCfg(func(c *base.ServerConfig) { c.EnableUserPortal = true })

	createPortalLoginSetup(t, "testuser", "testpass123", "default")
	ast := assert.New(t)

	body := `{"username":"testuser","password":"wrongpassword"}`
	req := httptest.NewRequest("POST", "/portal/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	PortalLogin(w, req)

	ast.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.NotEqual(float64(0), resp["code"])
}

func TestPortalLogin_UserNotFound(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()
	base.UpdateCfg(func(c *base.ServerConfig) { c.EnableUserPortal = true })

	createPortalLoginSetup(t, "testuser", "testpass123", "default")
	ast := assert.New(t)

	body := `{"username":"nonexistent","password":"testpass123"}`
	req := httptest.NewRequest("POST", "/portal/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	PortalLogin(w, req)

	ast.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.NotEqual(float64(0), resp["code"])
}

func TestPortalLogin_Disabled(t *testing.T) {
	base.Test()
	base.SetCfgForTest(&base.ServerConfig{EnableUserPortal: false})

	body := `{"username":"test","password":"test"}`
	req := httptest.NewRequest("POST", "/portal/login", strings.NewReader(body))
	w := httptest.NewRecorder()

	PortalLogin(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusNotFound, w.Code)
}

func createPortalPwdUser(t *testing.T, username, password string) (*dbdata.User, string) {
	ast := assert.New(t)

	pt := &dbdata.Policy{
		Name:      "plcy-pwd",
		ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}},
		Status:    1,
	}
	_ = dbdata.SetPolicy(pt)

	g := &dbdata.Group{
		Name:        "default",
		Status:      1,
		PolicyId:    pt.Id,
		AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`),
	}
	_ = dbdata.SetGroup(g)

	hashedPwd, err := utils.PasswordHash(password)
	ast.Nil(err)

	u := &dbdata.User{
		Username:   username,
		PinCode:    hashedPwd,
		Groups:     []string{"default"},
		Status:     1,
		DisableOtp: true,
	}
	err = dbdata.SetUser(u)
	ast.Nil(err)

	token, err := portalIssueToken(u)
	ast.Nil(err)
	return u, token
}

func TestPortalChangePassword_Success(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
		c.JwtSecret = "test-jwt-secret-for-portal-32b"
	})

	_, token := createPortalPwdUser(t, "testuser", "oldpass123")
	ast := assert.New(t)

	body := `{"old_password":"oldpass123","new_password":"newpass123"}`
	req := httptest.NewRequest("POST", "/portal/change_password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: portalCookieName, Value: token})
	w := httptest.NewRecorder()

	PortalChangePassword(w, req)

	ast.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	err := json.NewDecoder(w.Body).Decode(&resp)
	ast.Nil(err)
	ast.Equal(float64(0), resp["code"])

	loginBody := `{"username":"testuser","password":"newpass123"}`
	loginReq := httptest.NewRequest("POST", "/portal/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	PortalLogin(loginW, loginReq)

	var loginResp map[string]any
	json.NewDecoder(loginW.Body).Decode(&loginResp)
	ast.Equal(float64(0), loginResp["code"])
}

func TestPortalChangePassword_WrongOldPassword(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
		c.JwtSecret = "test-jwt-secret-for-portal-32b"
	})

	_, token := createPortalPwdUser(t, "testuser", "correctpass1")
	ast := assert.New(t)

	body := `{"old_password":"wrongoldpass","new_password":"newpass123"}`
	req := httptest.NewRequest("POST", "/portal/change_password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: portalCookieName, Value: token})
	w := httptest.NewRecorder()

	PortalChangePassword(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.NotEqual(float64(0), resp["code"])
	ast.Contains(resp["msg"], "旧密码错误")
}

func TestPortalChangePassword_WeakNewPassword(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
		c.JwtSecret = "test-jwt-secret-for-portal-32b"
	})

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
		c.JwtSecret = "test-jwt-secret-for-portal-32b"
	})

	_, token := createPortalPwdUser(t, "testuser", "oldpass123")
	ast := assert.New(t)

	body := `{"old_password":"oldpass123","new_password":"12345678"}`
	req := httptest.NewRequest("POST", "/portal/change_password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: portalCookieName, Value: token})
	w := httptest.NewRecorder()

	PortalChangePassword(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.NotEqual(float64(0), resp["code"])
}

func TestPortalChangePassword_TooShort(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
		c.JwtSecret = "test-jwt-secret-for-portal-32b"
	})

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
		c.JwtSecret = "test-jwt-secret-for-portal-32b"
	})

	_, token := createPortalPwdUser(t, "testuser", "oldpass123")
	ast := assert.New(t)

	body := `{"old_password":"oldpass123","new_password":"abc123"}`
	req := httptest.NewRequest("POST", "/portal/change_password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: portalCookieName, Value: token})
	w := httptest.NewRecorder()

	PortalChangePassword(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.NotEqual(float64(0), resp["code"])
}

func TestPortalChangePassword_Unauthorized(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()
	base.UpdateCfg(func(c *base.ServerConfig) { c.EnableUserPortal = true })

	body := `{"old_password":"old","new_password":"newpass123"}`
	req := httptest.NewRequest("POST", "/portal/change_password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	PortalChangePassword(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusUnauthorized, w.Code)
}

func TestPortalChangePassword_ExternalUser(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
		c.JwtSecret = "test-jwt-secret-for-portal-32b"
	})

	ast := assert.New(t)

	pt := &dbdata.Policy{
		Name:      "plcy-ext",
		ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}},
		Status:    1,
	}
	_ = dbdata.SetPolicy(pt)
	_ = dbdata.SetGroup(&dbdata.Group{
		Name:        "default",
		Status:      1,
		PolicyId:    pt.Id,
		AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`),
	})

	u := &dbdata.User{
		Username: "ldapuser",
		PinCode:  "notused",
		Type:     "ldap",
		Groups:   []string{"default"},
		Status:   1,
	}
	err := dbdata.SetUser(u)
	ast.Nil(err)

	token, err := portalIssueToken(u)
	ast.Nil(err)

	body := `{"old_password":"old","new_password":"newpass123"}`
	req := httptest.NewRequest("POST", "/portal/change_password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: portalCookieName, Value: token})
	w := httptest.NewRecorder()

	PortalChangePassword(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.NotEqual(float64(0), resp["code"])
	ast.Contains(resp["msg"], "外部认证")
}

func TestPortalResetPassword_InvalidToken(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
		c.JwtSecret = "test-jwt-secret-for-portal-32b"
	})

	ast := assert.New(t)

	body := `{"token":"invalid-token","new_password":"newpass123"}`
	req := httptest.NewRequest("POST", "/portal/reset_password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	PortalResetPassword(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.NotEqual(float64(0), resp["code"])
}

func TestPortalResetPassword_EmptyFields(t *testing.T) {
	base.Test()
	base.SetCfgForTest(&base.ServerConfig{EnableUserPortal: true})

	ast := assert.New(t)

	body := `{"token":"","new_password":"newpass123"}`
	req := httptest.NewRequest("POST", "/portal/reset_password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	PortalResetPassword(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.NotEqual(float64(0), resp["code"])
}

func TestPortalResetPassword_Success(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()

	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
		c.JwtSecret = "test-jwt-secret-for-portal-32b"
	})

	ast := assert.New(t)

	pt := &dbdata.Policy{
		Name:      "plcy-reset",
		ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}},
		Status:    1,
	}
	err := dbdata.SetPolicy(pt)
	ast.Nil(err)

	g := &dbdata.Group{
		Name:        "default",
		Status:      1,
		PolicyId:    pt.Id,
		AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`),
	}
	err = dbdata.SetGroup(g)
	ast.Nil(err)

	hashedPwd, _ := utils.PasswordHash("oldpass123")
	u := &dbdata.User{
		Username:   "resetuser",
		PinCode:    hashedPwd,
		Groups:     []string{"default"},
		Status:     1,
		DisableOtp: true,
	}
	err = dbdata.SetUser(u)
	ast.Nil(err)

	token, err := admin.SetJwtData(map[string]any{
		"purpose": "portal_reset_password",
		"user_id": float64(u.Id),
		"jti":     "test-jti-123",
	}, 9999999999)
	ast.Nil(err)

	// 重置密码
	body := `{"token":"` + token + `","new_password":"newpass456"}`
	req := httptest.NewRequest("POST", "/portal/reset_password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	PortalResetPassword(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ast.Equal(float64(0), resp["code"])

	loginBody := `{"username":"resetuser","password":"newpass456"}`
	loginReq := httptest.NewRequest("POST", "/portal/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	PortalLogin(loginW, loginReq)

	var loginResp map[string]any
	json.NewDecoder(loginW.Body).Decode(&loginResp)
	ast.Equal(float64(0), loginResp["code"], "should login with new password")
}

func TestPortalLogout(t *testing.T) {
	base.Test()
	base.SetCfgForTest(&base.ServerConfig{EnableUserPortal: true})

	req := httptest.NewRequest("POST", "/portal/logout", nil)
	w := httptest.NewRecorder()

	PortalLogout(w, req)

	ast := assert.New(t)
	ast.Equal(http.StatusOK, w.Code)

	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == portalCookieName && c.MaxAge < 0 {
			found = true
		}
	}
	ast.True(found, "should clear portal_session cookie")
}

func TestPortalLogin_ForcePwd(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()
	base.UpdateCfg(func(c *base.ServerConfig) { c.EnableUserPortal = true })

	createPortalLoginSetup(t, "forceuser", "oldpass123", "default")
	ast := assert.New(t)
	_, err := dbdata.GetXdb().Where("username = ?", "forceuser").Cols("change_pwd").
		Update(&dbdata.User{ForcePwd: true})
	ast.Nil(err)

	// 首次登录应返回 change_pwd 挑战（走管道 forcepwd 步骤）
	loginReq := httptest.NewRequest("POST", "/portal/login", strings.NewReader(`{"username":"forceuser","password":"oldpass123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	PortalLogin(loginW, loginReq)
	var loginResp map[string]any
	json.NewDecoder(loginW.Body).Decode(&loginResp)
	ast.Equal(float64(0), loginResp["code"])
	data := loginResp["data"].(map[string]any)
	ast.Equal("change_pwd", data["status"])
	token, _ := data["token"].(string)
	ast.NotEmpty(token)

	chgBody := `{"token":"` + token + `","new_password":"newpass123","new_password_confirm":"newpass123"}`
	chgReq := httptest.NewRequest("POST", "/portal/force_change_password", strings.NewReader(chgBody))
	chgReq.Header.Set("Content-Type", "application/json")
	chgW := httptest.NewRecorder()
	PortalForceChangePassword(chgW, chgReq)
	var chgResp map[string]any
	json.NewDecoder(chgW.Body).Decode(&chgResp)
	ast.Equal(float64(0), chgResp["code"])

	// 用新密码可正常登录
	login2Req := httptest.NewRequest("POST", "/portal/login", strings.NewReader(`{"username":"forceuser","password":"newpass123"}`))
	login2Req.Header.Set("Content-Type", "application/json")
	login2W := httptest.NewRecorder()
	PortalLogin(login2W, login2Req)
	var login2Resp map[string]any
	json.NewDecoder(login2W.Body).Decode(&login2Resp)
	ast.Equal(float64(0), login2Resp["code"])
}

func TestPortalLogin_OTP(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.EnableUserPortal = true
		c.SendOtp = false
	})

	pt := &dbdata.Policy{Name: "plcy-otp", ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}, Status: 1}
	_ = dbdata.SetPolicy(pt)
	_ = dbdata.SetGroup(&dbdata.Group{
		Name:        "otpgrp",
		Status:      1,
		PolicyId:    pt.Id,
		AuthProfile: json.RawMessage(`{"step":[{"type":"local"},{"type":"otp"}]}`),
	})
	hashed, _ := utils.PasswordHash("oldpass123")
	u := &dbdata.User{
		Username:  "otpuser",
		PinCode:   hashed,
		Groups:    []string{"otpgrp"},
		Status:    1,
		OtpSecret: "JBSWY3DPEHPK3PXP",
	}
	_ = dbdata.SetUser(u)

	ast := assert.New(t)
	loginReq := httptest.NewRequest("POST", "/portal/login", strings.NewReader(`{"username":"otpuser","password":"oldpass123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	PortalLogin(loginW, loginReq)
	var loginResp map[string]any
	json.NewDecoder(loginW.Body).Decode(&loginResp)
	ast.Equal(float64(0), loginResp["code"])
	data := loginResp["data"].(map[string]any)
	ast.Equal("otp", data["status"])
	sessionID, _ := data["session_id"].(string)
	ast.NotEmpty(sessionID)

	// 计算当前 TOTP 码并提交
	code := gotp.NewDefaultTOTP("JBSWY3DPEHPK3PXP").Now()
	verifyBody := fmt.Sprintf(`{"session_id":"%s","code":"%s"}`, sessionID, code)
	verifyReq := httptest.NewRequest("POST", "/portal/verify", strings.NewReader(verifyBody))
	verifyReq.Header.Set("Content-Type", "application/json")
	verifyW := httptest.NewRecorder()
	PortalVerify(verifyW, verifyReq)
	var verifyResp map[string]any
	json.NewDecoder(verifyW.Body).Decode(&verifyResp)
	ast.Equal(float64(0), verifyResp["code"])
	authData := verifyResp["data"].(map[string]any)
	ast.Equal("pass", authData["status"])
}
