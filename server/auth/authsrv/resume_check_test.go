package authsrv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/dbdata"
)

func TestReloadAndCheckUser(t *testing.T) {
	preTestData(t)
	ast := assert.New(t)

	createTestUser("resumecheck", "testpass123", true)

	ctx := &auth.Context{Conn: auth.ConnInfo{Username: "resumecheck", GroupName: "default"}}
	ast.NoError(ReloadAndCheckUser(ctx))
	ast.NotNil(ctx.UserInfoLoaded(), "应刷新 UserInfo")

	ctxNoRec := &auth.Context{Conn: auth.ConnInfo{Username: "never-synced", GroupName: "default"}}
	ast.NoError(ReloadAndCheckUser(ctxNoRec), "未同步的三方用户应放行")

	// 用户名为空 → 跳过
	ast.NoError(ReloadAndCheckUser(&auth.Context{}))

	// 停用 → 拒绝
	u := &dbdata.User{}
	ast.NoError(dbdata.One("Username", "resumecheck", u))
	u.Status = 2
	ast.NoError(dbdata.SetUser(u))
	ctxDisabled := &auth.Context{Conn: auth.ConnInfo{Username: "resumecheck", GroupName: "default"}}
	err := ReloadAndCheckUser(ctxDisabled)
	ast.Error(err, "停用用户必须被拒绝")
	ast.Contains(err.Error(), "禁用")

	// 过期 → 拒绝
	u2 := &dbdata.User{}
	ast.NoError(dbdata.One("Username", "resumecheck", u2))
	u2.Status = 1
	past := u2.CreatedAt.AddDate(-1, 0, 0)
	u2.LimitTime = &past
	ast.NoError(dbdata.SetUser(u2))
	ctxExpired := &auth.Context{Conn: auth.ConnInfo{Username: "resumecheck", GroupName: "default"}}
	err = ReloadAndCheckUser(ctxExpired)
	ast.Error(err, "过期用户必须被拒绝")
	ast.Contains(err.Error(), "过期")
}
