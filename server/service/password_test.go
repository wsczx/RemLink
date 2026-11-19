package service

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/dbtest"
	"github.com/wsczx/remlink/pkg/jwt"
	"github.com/wsczx/remlink/pkg/utils"
)

func TestResetTokenRoundTrip(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	u := &dbdata.User{Id: 42}
	token, err := IssueResetToken(u)
	ast.NoError(err)
	ast.NotEmpty(token)

	claims, err := ParseResetClaims(token)
	ast.NoError(err)
	ast.Equal(42, claims.UserID)
	ast.NotEmpty(claims.JTI, "一次性令牌标记必须存在")
}

func TestResetTokenDoesNotReuseJTI(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	u := &dbdata.User{Id: 7}
	t1, err := IssueResetToken(u)
	ast.NoError(err)
	time.Sleep(time.Nanosecond)
	t2, err := IssueResetToken(u)
	ast.NoError(err)
	c1, _ := ParseResetClaims(t1)
	c2, _ := ParseResetClaims(t2)
	ast.NotEqual(c1.JTI, c2.JTI, "每次签发的令牌应带不同 jti")
}

func TestParseResetClaimsErrors(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)

	cases := []struct {
		name    string
		token   func(t *testing.T) string
		wantErr error
	}{
		{
			name:    "非法令牌按过期处理",
			token:   func(*testing.T) string { return "not-a-jwt" },
			wantErr: ErrResetTokenExpired,
		},
		{
			name: "用途不符",
			token: func(t *testing.T) string {
				tok, err := jwt.SetJwtData(map[string]any{
					"purpose": "other", "jti": "x", "user_id": float64(1),
				}, time.Now().Unix()+900)
				if err != nil {
					t.Fatal(err)
				}
				return tok
			},
			wantErr: ErrResetTokenInvalid,
		},
		{
			name: "缺少 jti",
			token: func(t *testing.T) string {
				// SetJwtData 会自动生成 jti；显式空值覆盖以触发防御分支
				tok, err := jwt.SetJwtData(map[string]any{
					"purpose": ResetTokenPurpose, "jti": "", "user_id": float64(1),
				}, time.Now().Unix()+900)
				if err != nil {
					t.Fatal(err)
				}
				return tok
			},
			wantErr: ErrResetTokenInvalid,
		},
		{
			name: "已过期令牌",
			token: func(t *testing.T) string {
				tok, err := jwt.SetJwtData(map[string]any{
					"purpose": ResetTokenPurpose, "jti": "expired", "user_id": float64(1),
				}, time.Now().Unix()-1)
				if err != nil {
					t.Fatal(err)
				}
				return tok
			},
			wantErr: ErrResetTokenExpired,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseResetClaims(c.token(t))
			if !assert.Error(t, err) {
				return
			}
			assert.True(t, errors.Is(err, c.wantErr), "期望 %v，实际 %v", c.wantErr, err)
			assert.Equal(t, c.wantErr.Error(), err.Error(), "用户可见文案必须保持")
		})
	}
}

func TestLocalPasswordUser(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)
	group := seedUserGroup(t, "pwd")

	local := &dbdata.User{Username: "pwd-local", Status: 1, Groups: []string{group}}
	ast.NoError(dbdata.Users.SetUser(local))
	got, err := LocalPasswordUser(local.Id)
	ast.NoError(err)
	ast.Equal("pwd-local", got.Username)

	off := &dbdata.User{Username: "pwd-off", Status: 0, Groups: []string{group}}
	ast.NoError(dbdata.Users.SetUser(off))
	_, err = LocalPasswordUser(off.Id)
	ast.ErrorIs(err, ErrUserUnavailable, "停用用户不可自助改密")

	external := &dbdata.User{Username: "pwd-external", Status: 1, Type: "ldap", Groups: []string{group}}
	ast.NoError(dbdata.Users.SetUser(external))
	_, err = LocalPasswordUser(external.Id)
	ast.ErrorIs(err, ErrExternalUser, "外部认证用户不可自助改密")

	_, err = LocalPasswordUser(999999)
	ast.ErrorIs(err, ErrUserUnavailable)
}

func TestChangeLocalPassword(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)
	spy := useSpy(t)
	group := seedUserGroup(t, "change")

	u := &dbdata.User{Username: "change-user", Status: 1, Groups: []string{group}, PinCode: "orig-pass"}
	ast.NoError(dbdata.Users.SetUser(u))
	var stored dbdata.User
	ast.NoError(dbdata.Users.Get(u.Id, &stored))

	// 旧口令错误时不得写库、不得吊销会话
	n, err := ChangeLocalPassword(&stored, "wrong", "new-pass")
	ast.ErrorIs(err, ErrOldPassword)
	ast.Zero(n)
	ast.Empty(spy.jwtRevoked)
	var unchanged dbdata.User
	ast.NoError(dbdata.Users.Get(u.Id, &unchanged))
	ast.Equal(stored.PinCode, unchanged.PinCode)

	// 空旧口令同样拒绝
	_, err = ChangeLocalPassword(&stored, "", "new-pass")
	ast.ErrorIs(err, ErrOldPassword)

	// 正确旧口令：写入新口令并吊销门户会话
	n, err = ChangeLocalPassword(&stored, "orig-pass", "new-pass")
	ast.NoError(err)
	ast.Equal(0, n, "spy 的 RevokeJwt 返回 0")
	ast.Equal([]string{"change-user"}, spy.jwtRevoked)
	var updated dbdata.User
	ast.NoError(dbdata.Users.Get(u.Id, &updated))
	ast.True(utils.VerifyPassword("new-pass", updated.PinCode))
	ast.False(utils.VerifyPassword("orig-pass", updated.PinCode))
}

func TestCommitPasswordKeepsHashedValue(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)
	spy := useSpy(t)
	group := seedUserGroup(t, "commit")

	u := &dbdata.User{Username: "commit-user", Status: 1, Groups: []string{group}}
	ast.NoError(dbdata.Users.SetUser(u))
	hashed, err := utils.PasswordHash("committed-pass")
	ast.NoError(err)

	n, err := CommitPassword(u.Username, hashed)
	ast.NoError(err)
	ast.Zero(n)
	ast.Equal([]string{"commit-user"}, spy.jwtRevoked)

	var stored dbdata.User
	ast.NoError(dbdata.Users.Get(u.Id, &stored))
	ast.Equal(hashed, stored.PinCode, "CommitPassword 应直接写入给定的哈希，不再二次哈希")
	ast.True(utils.VerifyPassword("committed-pass", stored.PinCode))
}

// 声明缺失或非法的 user_id 不得退化为 Id=0 查询
func TestParseResetClaimsRejectsBadUserID(t *testing.T) {
	base.Test()
	dbtest.SetupDB(t)
	ast := assert.New(t)

	for _, claims := range []map[string]any{
		{"purpose": ResetTokenPurpose, "jti": "x"},
		{"purpose": ResetTokenPurpose, "jti": "x", "user_id": float64(0)},
		{"purpose": ResetTokenPurpose, "jti": "x", "user_id": "7"},
	} {
		tok, err := jwt.SetJwtData(claims, time.Now().Unix()+900)
		ast.NoError(err)
		_, err = ParseResetClaims(tok)
		ast.ErrorIs(err, ErrResetTokenInvalid)
	}
}
