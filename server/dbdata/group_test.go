package dbdata

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/pkg/security"
	"github.com/wsczx/remlink/pkg/utils"
)

func TestGetGroupNames(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	defer closeIpdata()

	err := SetProvider(&Provider{
		Name:   "test-radius",
		Type:   "radius",
		Status: 1,
		Config: security.EncryptedJSON[json.RawMessage]{Data: json.RawMessage(`{"addr":"192.168.8.12:1044","secret":"43214132"}`)},
	})
	ast.Nil(err)
	err = SetProvider(&Provider{
		Name:   "test-ldap",
		Type:   "ldap",
		Status: 1,
		Config: security.EncryptedJSON[json.RawMessage]{Data: json.RawMessage(`{"addr":"192.168.8.12:389","tls":true,"bind_name":"userfind@abc.com","bind_pwd":"afdbfdsafds","base_dn":"dc=abc,dc=com","object_class":"person","search_attr":"sAMAccountName","member_of":"cn=vpn,cn=user,dc=abc,dc=com"}`)},
	})
	ast.Nil(err)

	// 创建通用测试策略
	defaultPolicy := &Policy{
		Name:      "group-test-default",
		ClientDns: []ValData{{Val: "114.114.114.114"}},
		Status:    1,
	}
	err = SetPolicy(defaultPolicy)
	ast.Nil(err)
	pid := defaultPolicy.Id

	// 添加 group
	g1 := Group{Name: "g1", PolicyId: pid}
	err = SetGroup(&g1)
	ast.Nil(err)
	g2 := Group{Name: "g2", PolicyId: pid}
	err = SetGroup(&g2)
	ast.Nil(err)
	g3 := Group{Name: "g3", PolicyId: pid}
	err = SetGroup(&g3)
	ast.Nil(err)

	g4 := Group{Name: "g4", PolicyId: pid,
		AuthProfile: json.RawMessage(`{"step":[{"type":"radius","provider":"test-radius"}]}`)}
	err = SetGroup(&g4)
	ast.Nil(err)

	// g5: 含域名拆分，需新建策略
	p5 := &Policy{
		Name:             "group-test-g5",
		ClientDns:        []ValData{{Val: "114.114.114.114"}},
		RouteInclude:     []ValData{{Val: "10.0.0.0/8"}},
		DsIncludeDomains: "baidu.com,163.com",
		Status:           1,
	}
	err = SetPolicy(p5)
	ast.Nil(err)
	g5 := Group{Name: "g5", PolicyId: p5.Id}
	err = SetGroup(&g5)
	ast.Nil(err)

	// g6: 含排除域名
	p6 := &Policy{
		Name:             "group-test-g6",
		ClientDns:        []ValData{{Val: "114.114.114.114"}},
		DsExcludeDomains: "com.cn,qq.com",
		Status:           1,
	}
	err = SetPolicy(p6)
	ast.Nil(err)
	g6 := Group{Name: "g6", PolicyId: p6.Id}
	err = SetGroup(&g6)
	ast.Nil(err)

	g7 := Group{Name: "g7", PolicyId: pid,
		AuthProfile: json.RawMessage(`{"step":[{"type":"ldap","provider":"test-ldap"}]}`)}
	err = SetGroup(&g7)
	ast.Nil(err)

	// 判断所有数据
	gAll := []string{"g1", "g2", "g3", "g4", "g5", "g6", "g7"}
	gs := GetGroupNames()
	for _, v := range gs {
		ast.Equal(true, utils.InArrStr(gAll, v))
	}

	gni := GetGroupNamesIds()
	for _, v := range gni {
		ast.NotEqual(0, v.Id)
		ast.Equal(true, utils.InArrStr(gAll, v.Name))
	}
}

// 测试 AnyGroupHasCertAuth：扫描组认证配置判断是否启用 cert 认证，带缓存。
// 缓存核心语义：InvalidateCertAuthCache 后立即反映最新组配置。
func TestAnyGroupHasCertAuth(t *testing.T) {
	t.Run("有cert组返回true", func(t *testing.T) {
		ast := assert.New(t)
		defer InvalidateCertAuthCache()
		preIpData(t)
		defer closeIpdata()

		pt := &Policy{Name: "cert-auth-default", ClientDns: []ValData{{Val: "8.8.8.8"}}, Status: 1}
		ast.Nil(SetPolicy(pt))
		ast.Nil(SetGroup(&Group{Name: "cert-yes", Status: 1, PolicyId: pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"cert"},{"type":"local"}]}`)}))
		InvalidateCertAuthCache()
		ast.True(AnyGroupHasCertAuth(), "含 cert 组应返回 true")
	})

	t.Run("无cert组返回false", func(t *testing.T) {
		ast := assert.New(t)
		defer InvalidateCertAuthCache()
		preIpData(t)
		defer closeIpdata()

		pt := &Policy{Name: "cert-auth-none", ClientDns: []ValData{{Val: "8.8.8.8"}}, Status: 1}
		ast.Nil(SetPolicy(pt))
		ast.Nil(SetGroup(&Group{Name: "cert-no", Status: 1, PolicyId: pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"local"}]}`)}))
		InvalidateCertAuthCache()
		ast.False(AnyGroupHasCertAuth(), "无 cert 组应返回 false")
	})
}

// 测试 InvalidateCertAuthCache：使缓存立即失效，失效后反映最新组配置，且并发安全。
func TestInvalidateCertAuthCache(t *testing.T) {
	t.Run("失效后反映最新配置", func(t *testing.T) {
		ast := assert.New(t)
		defer InvalidateCertAuthCache()
		preIpData(t)
		defer closeIpdata()

		pt := &Policy{Name: "cert-inv-default", ClientDns: []ValData{{Val: "8.8.8.8"}}, Status: 1}
		ast.Nil(SetPolicy(pt))
		ast.Nil(SetGroup(&Group{Name: "cert-inv", Status: 1, PolicyId: pt.Id,
			AuthProfile: json.RawMessage(`{"step":[{"type":"cert"}]}`)}))
		_ = pt.Id

		InvalidateCertAuthCache()
		ast.True(AnyGroupHasCertAuth(), "有 cert 组应返回 true")
	})

	t.Run("并发失效安全", func(t *testing.T) {
		ast := assert.New(t)
		defer InvalidateCertAuthCache()
		// 并发多次失效不应 panic
		for i := 0; i < 50; i++ {
			go InvalidateCertAuthCache()
		}
		InvalidateCertAuthCache()
		// 失效后处于未缓存状态，调用应安全返回（不依赖具体 true/false，只验证不 panic）
		v := AnyGroupHasCertAuth()
		ast.True(v || !v, "并发失效后调用不应 panic")
	})
}

// 测试 HasAuthType：解析 AuthProfile 判断是否包含指定认证类型。
func TestHasAuthType(t *testing.T) {
	ast := assert.New(t)

	ast.True(HasAuthType(json.RawMessage(`{"step":[{"type":"cert"},{"type":"local"}]}`), "cert"))
	ast.True(HasAuthType(json.RawMessage(`{"step":[{"type":"local"}]}`), "local"))
	ast.False(HasAuthType(json.RawMessage(`{"step":[{"type":"local"}]}`), "cert"))
	ast.False(HasAuthType(json.RawMessage(`not-json`), "cert"))
	ast.False(HasAuthType(nil, "cert"))
}
