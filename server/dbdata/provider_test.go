package dbdata

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/pkg/security"
)

func TestSetProviderStatusDisable(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	cfg := security.EncryptedJSON[json.RawMessage]{Data: json.RawMessage(`{"addr":"192.168.8.12:1812","secret":"s3cret-long-enough"}`)}

	// 新建即停用：status=0 不应被改回 1
	p := &Provider{Name: "p-disabled", Type: "radius", Status: 0, Config: cfg}
	ast.Nil(SetProvider(p))
	got := &Provider{}
	ast.Nil(One("Name", "p-disabled", got))
	ast.Equal(int8(0), got.Status)

	// 启用 → 停用
	p2 := &Provider{Name: "p-toggle", Type: "radius", Status: 1, Config: cfg}
	ast.Nil(SetProvider(p2))
	_, err := ResolveProviderConfig("p-toggle", "radius")
	ast.Nil(err)
	ast.Contains(ProviderNamesByType("radius"), "p-toggle")

	p2.Status = 0
	ast.Nil(SetProvider(p2))
	got = &Provider{}
	ast.Nil(One("Name", "p-toggle", got))
	ast.Equal(int8(0), got.Status)

	// 停用后认证流程解析与可选列表都应跳过
	_, err = ResolveProviderConfig("p-toggle", "radius")
	ast.NotNil(err)
	ast.NotContains(ProviderNamesByType("radius"), "p-toggle")
	ast.NotContains(ProviderNamesByType(""), "p-disabled")

	// 停用 → 启用
	p2.Status = 1
	ast.Nil(SetProvider(p2))
	_, err = ResolveProviderConfig("p-toggle", "radius")
	ast.Nil(err)

	// 非法状态
	p3 := &Provider{Name: "p-bad", Type: "radius", Status: 5, Config: cfg}
	ast.NotNil(SetProvider(p3))
}
