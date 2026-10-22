package jwt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
)

func TestJwtData(t *testing.T) {
	assert := assert.New(t)
	base.SetCfgForTest(&base.ServerConfig{JwtSecret: "dsfasfdfsadfasdfasd3sdaf"})
	data := map[string]any{
		"key": "value",
	}
	expiresAt := time.Now().Add(time.Minute).Unix()
	token, err := SetJwtData(data, expiresAt)
	assert.Nil(err)
	dataN, err := GetJwtData(token)
	assert.Nil(err)
	assert.Equal(dataN["key"], "value")
}

// 改密/停用/删号后，该主体全部未过期 JWT 立即失效，且不影响其他主体（M-3）
func TestRevokeSubjectJwt(t *testing.T) {
	assert := assert.New(t)
	base.SetCfgForTest(&base.ServerConfig{JwtSecret: "dsfasfdfsadfasdfasd3sdaf"})

	jwtIssuedMu.Lock()
	jwtIssued = make(map[string]jwtIssuedRecord)
	jwtIssuedMu.Unlock()

	expiresAt := time.Now().Add(time.Hour).Unix()

	tokenA1, err := SetJwtData(map[string]any{"portal_user": "alice"}, expiresAt)
	assert.Nil(err)
	tokenA2, err := SetJwtData(map[string]any{"portal_user": "alice"}, expiresAt)
	assert.Nil(err)
	tokenB1, err := SetJwtData(map[string]any{"portal_user": "bob"}, expiresAt)
	assert.Nil(err)

	RegisterJwt("alice", tokenA1)
	RegisterJwt("alice", tokenA2)
	RegisterJwt("bob", tokenB1)

	_, err = GetJwtData(tokenA1)
	assert.Nil(err)
	_, err = GetJwtData(tokenA2)
	assert.Nil(err)
	_, err = GetJwtData(tokenB1)
	assert.Nil(err)

	n := RevokeSubjectJwt("alice")
	assert.Equal(2, n)

	_, err = GetJwtData(tokenA1)
	assert.Error(err, "alice 的会话应被吊销")
	_, err = GetJwtData(tokenA2)
	assert.Error(err, "alice 的会话应被吊销")

	_, err = GetJwtData(tokenB1)
	assert.Nil(err, "bob 的会话不应被影响")

	assert.Equal(0, RevokeSubjectJwt("alice"))

	assert.Equal(0, RevokeSubjectJwt(""))
}

func TestRegisterJwtInvalid(t *testing.T) {
	assert := assert.New(t)
	base.SetCfgForTest(&base.ServerConfig{JwtSecret: "dsfasfdfsadfasdfasd3sdaf"})

	jwtIssuedMu.Lock()
	before := len(jwtIssued)
	jwtIssuedMu.Unlock()

	// 不应 panic
	RegisterJwt("", "some-token")
	RegisterJwt("user", "")
	RegisterJwt("user", "not-a-jwt")

	// 静默忽略：不得登记任何索引条目
	jwtIssuedMu.Lock()
	after := len(jwtIssued)
	jwtIssuedMu.Unlock()
	assert.Equal(before, after, "无效输入不得在 jwtIssued 索引中留下条目")
}
