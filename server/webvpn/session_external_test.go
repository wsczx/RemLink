package webvpn

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/dbdata"
)

// 回归护栏：external + 非空 groups 放行，
// local/ldap（须落库）与空 groups / 空用户名拒绝。
func TestExternalUserFromClaims_ForcePwdFlow(t *testing.T) {
	setupWebVpnDB(t)
	defer dbdata.Stop()
	m := GetManager()

	// 三方用户带有效 groups -> 放行
	ding := m.Session().externalUserFromClaims("zhangsan", map[string]any{
		"webvpn_grant_type": "dingtalk",
		"webvpn_groups":     []string{"groupA"},
	})
	require.NotNil(t, ding, "三方用户应放行")
	assert.Equal(t, "dingtalk", ding.Type)
	assert.Equal(t, []string{"groupA"}, ding.Groups)

	// 兼容 portal_type / portal_groups 字段名
	feishu := m.Session().externalUserFromClaims("lisi", map[string]any{
		"portal_type":   "feishu",
		"portal_groups": []string{"groupB"},
	})
	require.NotNil(t, feishu)
	assert.Equal(t, "feishu", feishu.Type)

	// 本地账户即使带 groups 也拒绝：必须落库，避免凭旧 JWT 重建
	local := m.Session().externalUserFromClaims("admin", map[string]any{
		"webvpn_grant_type": "local",
		"webvpn_groups":     []string{"groupA"},
	})
	assert.Nil(t, local, "local 用户必须落库才认，不凭 JWT 重建")

	// 空 groups 拒绝：无法赋予访问权限
	noGroup := m.Session().externalUserFromClaims("wangwu", map[string]any{
		"webvpn_grant_type": "dingtalk",
		"webvpn_groups":     []string{},
	})
	assert.Nil(t, noGroup, "空 groups 的三方用户必须拒绝")

	assert.Nil(t, m.Session().externalUserFromClaims("", map[string]any{
		"webvpn_grant_type": "dingtalk",
		"webvpn_groups":     []string{"g"},
	}))
}
