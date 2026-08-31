package authsrv

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/dbdata"
)

// 将 PEM 字符串解析为 x509 证书（仅测试用）。
func parseCertFromPEM(t *testing.T, pemStr string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(pemStr))
	require.NotNil(t, block, "PEM 解析失败")
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "x509 解析失败")
	return cert
}

func setupCertGroup(t *testing.T, group, username string) *x509.Certificate {
	t.Helper()

	require.NoError(t, dbdata.GenerateClientCA())

	// 策略 + 组
	policy := &dbdata.Policy{Name: group + "-policy", Status: 1, ClientDns: []dbdata.ValData{{Val: "8.8.8.8"}}}
	require.NoError(t, dbdata.SetPolicy(policy))
	require.NoError(t, dbdata.SetGroup(&dbdata.Group{Name: group, Status: 1, PolicyId: policy.Id}))

	require.NoError(t, dbdata.SetUser(&dbdata.User{Username: username, Groups: []string{group}, Status: 1}))

	certData, err := dbdata.GenerateClientCert(username, group, false, 3)
	require.NoError(t, err, "生成客户端证书失败")

	return parseCertFromPEM(t, certData.Certificate)
}

// 验证手动认证路径（init 未带证书、
// auth-reply 才带证书，Conn.Username 为空）下，证书 CN 会回填到 Conn.Username，
// 避免末尾一致性检查误拒合法证书用户。
func TestCertAuth_ManualPath_EmptyUsername_Backfill(t *testing.T) {
	preTestData(t)
	group := "cert-empty-group"
	username := "cert-empty-user"
	cert := setupCertGroup(t, group, username)

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "", // 手动路径：用户未键入用户名
			GroupName: group,
			TLS:       &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}},
		},
	}

	result, err := (&CertAuth{}).Authenticate(ctx)
	assert.Equal(t, auth.StepPass, result)
	assert.NoError(t, err)
	// 关键断言：证书 CN 回填为认证身份
	assert.Equal(t, username, ctx.Conn.Username, "空用户名应被证书 CN 回填")
	assert.Equal(t, username, ctx.Identity, "Identity 应为证书 CN")
}

// 走完整管道验证：
// [cert] 管道在手动路径（空用户名）下，末尾一致性检查应通过。
// 否则 runFrom 末尾会因 Conn.Username(空) != Identity(cert CN) 返回 StepFail。
func TestCertPipeline_ManualPath_EmptyUsername_PassesConsistency(t *testing.T) {
	preTestData(t)
	group := "cert-pipe-group"
	username := "cert-pipe-user"
	cert := setupCertGroup(t, group, username)

	profile := auth.GroupAuthProfile{Step: []auth.AuthMethodConfig{{Type: "cert"}}}
	pipeline, err := auth.GetPipeline(profile, dbdata.ResolveProviderConfig)
	require.NoError(t, err)

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "",
			GroupName: group,
			TLS:       &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}},
		},
	}

	result, err := pipeline.Run(ctx)
	assert.Equal(t, auth.StepPass, result)
	assert.NoError(t, err, "手动路径空用户名不应被一致性检查拒绝")
	assert.Equal(t, username, ctx.Conn.Username)
}

// 验证安全语义仍生效：
// 当用户键入身份与证书 CN 冲突时，一致性检查应拒绝。
func TestCertPipeline_ConflictingUsername_Fails(t *testing.T) {
	preTestData(t)
	group := "cert-conflict-group"
	username := "cert-conflict-user"
	cert := setupCertGroup(t, group, username)

	profile := auth.GroupAuthProfile{Step: []auth.AuthMethodConfig{{Type: "cert"}}}
	pipeline, err := auth.GetPipeline(profile, dbdata.ResolveProviderConfig)
	require.NoError(t, err)

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username:  "another-user", // 与证书 CN 冲突
			GroupName: group,
			TLS:       &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}},
		},
	}

	result, err := pipeline.Run(ctx)
	assert.Equal(t, auth.StepFail, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不一致")
}
