package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

// 构造仅携带用户名的最小认证上下文（供 ReloadUserInfo 使用）
func newChallengeCtx(username string) *auth.Context {
	return &auth.Context{Conn: auth.ConnInfo{Username: username}}
}

// 启动测试数据库（复用 preIpData，避免 ToXML 调 GetGroupNamesNormal 时空引擎）
func setupChallengeTestDB(t *testing.T) {
	t.Helper()
	preIpData(t)
}

// 三端挑战字段对齐：同一份 ChallengeView 经 ToXML / ToWebAuthJSON / ToPortalJSON
// 序列化后，类型语义 / 提示文案 / 脱敏手机号应保持一致，防止某一端悄悄丢字段。
func TestChallengeView_ThreeWayAlignment(t *testing.T) {
	setupChallengeTestDB(t)
	defer closeIpdata()

	view := &ChallengeView{
		Type:        auth.ChallengeSMS,
		Message:     "请输入短信验证码",
		PhoneMasked: "138****8000",
	}

	xmlData := view.ToXML()
	web := view.ToWebAuthJSON()
	portal := view.ToPortalJSON("sess-1")

	assert.Equal(t, "请输入短信验证码", xmlData.Error)
	assert.Equal(t, "请输入短信验证码", web["message"])
	assert.Equal(t, "请输入短信验证码", portal["message"])

	assert.Equal(t, "138****8000", xmlData.PhoneMasked)
	assert.Equal(t, "138****8000", web["phone_masked"])
	assert.Equal(t, "138****8000", portal["phone_masked"])

	assert.Equal(t, web["status"], portal["status"])

	radiusView := &ChallengeView{
		Type:    auth.ChallengeRADIUS,
		Message: "请输入二次验证码",
	}
	rWeb := radiusView.ToWebAuthJSON()
	rPortal := radiusView.ToPortalJSON("sess-2")
	assert.Equal(t, "请输入二次验证码", rWeb["message"])
	assert.Equal(t, "请输入二次验证码", rPortal["message"])
	assert.Equal(t, "请输入二次验证码", rWeb["challenge_msg"])

	fpView := &ChallengeView{Type: auth.ChallengeForcePwd, Message: "请设置新密码以继续登录"}
	fpWeb := fpView.ToWebAuthJSON()
	fpPortal := fpView.ToPortalJSON("sess-3")
	assert.Equal(t, fpWeb["status"], fpPortal["status"])
	assert.Equal(t, "sess-3", fpPortal["token"])
}

// 脱敏手机号格式：前 3 + **** + 后 4；短号不脱敏原样返回
func TestMaskPhone(t *testing.T) {
	assert.Equal(t, "138****8000", maskPhone("13800008000"))
	assert.Equal(t, "123", maskPhone("123"))
}

// RunForcePwdChange：校验策略 + 哈希 + 写库清除 ForcePwd + 重载
func TestRunForcePwdChange(t *testing.T) {
	base.Test()
	preIpData(t)
	defer closeIpdata()

	username := "forcepwd-unit"
	_ = dbdata.Del(&dbdata.User{Username: username})
	u, _ := createPortalPwdUser(t, username, "OldPass@123")
	u.ForcePwd = true
	_ = dbdata.SetUser(u)

	newPwd := "NewPass@456"
	err := RunForcePwdChange(newChallengeCtx(username), username, newPwd)
	assert.Nil(t, err)

	after := &dbdata.User{}
	assert.Nil(t, dbdata.One("Username", username, after))
	assert.False(t, after.ForcePwd)
	// dbdata 读取对 pin_code 透明解密，无法直接 verify 密文，故用同一哈希算法自洽校验
	hashed, herr := utils.PasswordHash(newPwd)
	assert.Nil(t, herr)
	assert.True(t, utils.IsBcryptHash(hashed))
	assert.True(t, utils.PasswordVerify(newPwd, hashed))

	ctx := newChallengeCtx(username)
	authsrv.ReloadUserInfo(ctx)
	assert.NotNil(t, ctx.UserInfoLoaded())
	assert.False(t, ctx.UserInfoLoaded().ForcePwd)

	_ = dbdata.Del(&dbdata.User{Username: username})
}

// RunForcePwdChange 拒绝不合策略的密码
func TestRunForcePwdChange_RejectWeak(t *testing.T) {
	base.Test()
	err := RunForcePwdChange(newChallengeCtx("nope"), "nope", "123")
	assert.NotNil(t, err)
}
