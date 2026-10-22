package dbdata

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
)

func setupUserAuthTest(t *testing.T) {
	for _, name := range []string{"local", "ldap", "radius", "otp", "cert", "saml", "wxwork", "feishu"} {
		if !auth.Registry.IsRegistered(name) {
			n := name
			auth.Registry.Register(n, func() auth.Authenticator { return &testAuthStub{name: n} })
		}
	}
	base.Test()
	tmpDb := path.Join(t.TempDir(), "userauth_test.db")
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.DbType = "sqlite3"
		c.DbSource = tmpDb
	})
	initDb()
	t.Cleanup(func() { xdb.Close() })
}

func TestSyncExternalUsers_Basic(t *testing.T) {
	assert := assert.New(t)
	setupUserAuthTest(t)

	g := &Group{Name: "g1"}
	assert.Nil(Add(g))

	remote := map[string]RemoteUser{
		"u1": {ID: "u1", Name: "用户一", Phone: "13800000001", Email: "u1@x.com"},
		"u2": {ID: "u2", Name: "用户二", Phone: "13800000002", Email: "u2@x.com"},
	}
	old := &User{Type: "wxwork", Username: "u1", Nickname: "旧名", Groups: []string{"g1"}, Status: 1}
	assert.Nil(Add(old))
	other := &User{Type: "local", Username: "u2", Nickname: "本地用户", Groups: []string{"g1"}, Status: 1}
	assert.Nil(Add(other))
	absent := &User{Type: "wxwork", Username: "gone", Nickname: "失联", Groups: []string{"g1"}, Status: 1}
	assert.Nil(Add(absent))

	err := syncExternalUsers(g, "wxwork", "企微", remote, true, nil, false)
	assert.Nil(err)

	var u1 User
	assert.Nil(One("username", "u1", &u1))
	assert.Equal("用户一", u1.Nickname)
	assert.Equal("13800000001", u1.Phone)
	assert.False(u1.DisableOtp, "needOTP=true 应启用 OTP（DisableOtp=!needOTP）")

	var u2 User
	assert.Nil(One("username", "u2", &u2))
	assert.Equal("local", u2.Type)
	assert.Equal("本地用户", u2.Nickname)

	var gone User
	assert.Error(One("username", "gone", &gone), "失联用户应被删除")
}

func TestSyncExternalUsers_FillEmptyOnly(t *testing.T) {
	assert := assert.New(t)
	setupUserAuthTest(t)

	g := &Group{Name: "g1"}
	assert.Nil(Add(g))

	local := &User{Type: "ldap", Username: "l1", Nickname: "本地名", Email: "local@x.com", Phone: "13900000000", Groups: []string{"g1"}, Status: 1}
	assert.Nil(Add(local))

	remote := map[string]RemoteUser{
		"l1": {ID: "l1", Name: "LDAP名", Phone: "13800000001", Email: "ldap@x.com"},
	}
	err := syncExternalUsers(g, "ldap", "LDAP", remote, false, nil, true)
	assert.Nil(err)

	var u User
	assert.Nil(One("username", "l1", &u))
	assert.Equal("local@x.com", u.Email, "fillEmptyOnly: 本地邮箱应保留")
	assert.Equal("13900000000", u.Phone, "fillEmptyOnly: 本地手机应保留")
	assert.Equal("LDAP名", u.Nickname, "昵称始终以远端为准")
	assert.True(u.DisableOtp, "needOTP=false 应禁用 OTP（DisableOtp=!needOTP）")
}

func TestSyncExternalUsers_RemoteWins(t *testing.T) {
	assert := assert.New(t)
	setupUserAuthTest(t)

	g := &Group{Name: "g1"}
	assert.Nil(Add(g))

	local := &User{Type: "feishu", Username: "f1", Nickname: "旧", Email: "old@x.com", Groups: []string{"g1"}, Status: 1}
	assert.Nil(Add(local))

	remote := map[string]RemoteUser{
		"f1": {ID: "f1", Name: "新", Phone: "13800000001", Email: "new@x.com"},
	}
	err := syncExternalUsers(g, "feishu", "飞书", remote, false, nil, false)
	assert.Nil(err)

	var u User
	assert.Nil(One("username", "f1", &u))
	assert.Equal("new@x.com", u.Email, "远端为准：邮箱应被覆盖")
	assert.Equal("13800000001", u.Phone)
}
func TestSyncExternalUsers_RemoteEmptyNoOverwrite(t *testing.T) {
	assert := assert.New(t)
	setupUserAuthTest(t)

	g := &Group{Name: "g1"}
	assert.Nil(Add(g))

	local := &User{Type: "wxwork", Username: "w1", Nickname: "旧", Email: "local@x.com", Phone: "13900000000", Groups: []string{"g1"}, Status: 1}
	assert.Nil(Add(local))

	// 远端仅有 ID/Name：模拟被拒名单用户跳过详情 API 的形态
	remote := map[string]RemoteUser{
		"w1": {ID: "w1", Name: "新名"},
	}
	err := syncExternalUsers(g, "wxwork", "企微", remote, false, nil, false)
	assert.Nil(err)

	var u User
	assert.Nil(One("username", "w1", &u))
	assert.Equal("local@x.com", u.Email, "远端邮箱为空不得抹除本地邮箱")
	assert.Equal("13900000000", u.Phone, "远端手机为空不得抹除本地手机")
	assert.Equal("新名", u.Nickname, "昵称始终以远端为准")
}

func TestSyncExternalUsers_Blocked(t *testing.T) {
	assert := assert.New(t)
	setupUserAuthTest(t)

	g := &Group{Name: "g1"}
	assert.Nil(Add(g))

	// 已存在的被拒绝用户（在清理中应被移除）
	blockedExisting := &User{Type: "dingtalk", Username: "b1", Nickname: "被拒", Groups: []string{"g1"}, Status: 1}
	assert.Nil(Add(blockedExisting))

	remote := map[string]RemoteUser{
		"b1": {ID: "b1", Name: "被拒用户"},
		"ok": {ID: "ok", Name: "正常"},
	}
	isBlocked := func(id string) bool { return id == "b1" }

	err := syncExternalUsers(g, "dingtalk", "钉钉", remote, false, isBlocked, false)
	assert.Nil(err)

	// 被拒用户不新增/更新，且因不在 syncedUsers 中被清理删除
	var b1 User
	assert.Error(One("username", "b1", &b1), "被拒用户应被清理")

	// 正常用户已新增
	var ok User
	assert.Nil(One("username", "ok", &ok))
	assert.Equal("dingtalk", ok.Type)
}

// 失联用户属多组时仅移出当前组
func TestSyncExternalUsers_MultiGroupCleanup(t *testing.T) {
	assert := assert.New(t)
	setupUserAuthTest(t)

	g1 := &Group{Name: "g1"}
	g2 := &Group{Name: "g2"}
	assert.Nil(Add(g1))
	assert.Nil(Add(g2))

	multi := &User{Type: "wxwork", Username: "m1", Nickname: "多组", Groups: []string{"g1", "g2"}, Status: 1}
	assert.Nil(Add(multi))

	// g1 同步空列表（remote 为空但不清错：直接传空 map）
	err := syncExternalUsers(g1, "wxwork", "企微", map[string]RemoteUser{}, false, nil, false)
	assert.Nil(err)

	var u User
	assert.Nil(One("username", "m1", &u))
	assert.Equal([]string{"g2"}, u.Groups, "失联用户应仅从 g1 移出，保留 g2")
}

// 配置解析共用逻辑
func TestResolveProviderConfigMap(t *testing.T) {
	assert := assert.New(t)
	setupUserAuthTest(t)

	t.Run("无匹配步骤", func(t *testing.T) {
		g := &Group{Name: "g1", AuthProfile: []byte(`{"step":[{"type":"local"}]}`)}
		_, err := resolveProviderConfigMap(g, "wxwork", "企微")
		assert.Error(err)
		assert.Contains(err.Error(), "未找到")
	})

	t.Run("步骤未设置Provider", func(t *testing.T) {
		g := &Group{Name: "g1", AuthProfile: []byte(`{"step":[{"type":"wxwork"}]}`)}
		_, err := resolveProviderConfigMap(g, "wxwork", "企微")
		assert.Error(err)
		assert.Contains(err.Error(), "Provider")
	})

	t.Run("非法AuthProfile", func(t *testing.T) {
		g := &Group{Name: "g1", AuthProfile: []byte(`not-json`)}
		_, err := resolveProviderConfigMap(g, "wxwork", "企微")
		assert.Error(err)
	})
}
