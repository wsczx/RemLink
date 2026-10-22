package auth

import (
	"strings"
	"testing"

	"github.com/go-ldap/ldap"
)

func TestLDAPConfig_Defaults(t *testing.T) {
	c := &LDAPConfig{}
	c.Defaults()
	// ObjectClass 缺省应回退为 person
	if c.ObjectClass != "person" {
		t.Fatalf("Defaults() ObjectClass = %q, want \"person\"", c.ObjectClass)
	}

	// 已显式设置时不应被覆盖
	c2 := &LDAPConfig{ObjectClass: "user"}
	c2.Defaults()
	if c2.ObjectClass != "user" {
		t.Fatalf("Defaults() 覆盖了已设置的 ObjectClass = %q", c2.ObjectClass)
	}
}

func TestLDAPConfig_ValidateConfig(t *testing.T) {
	base := LDAPConfig{
		Addr:        "ldap.example.com:389",
		BindName:    "cn=admin,dc=example,dc=com",
		BindPwd:     "secret",
		BaseDn:      "dc=example,dc=com",
		ObjectClass: "person",
		SearchAttr:  "cn",
	}

	if err := base.ValidateConfig(); err != nil {
		t.Fatalf("合法配置应校验通过，实得错误: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(c *LDAPConfig)
	}{
		{"地址格式错误(无端口)", func(c *LDAPConfig) { c.Addr = "ldap.example.com" }},
		{"地址格式错误(非主机)", func(c *LDAPConfig) { c.Addr = "999.999.999.999:389" }},
		{"管理员DN为空", func(c *LDAPConfig) { c.BindName = "" }},
		{"管理员密码为空", func(c *LDAPConfig) { c.BindPwd = "" }},
		{"BaseDn格式错误", func(c *LDAPConfig) { c.BaseDn = "example" }},
		{"ObjectClass为空", func(c *LDAPConfig) { c.ObjectClass = "" }},
		{"SearchAttr为空", func(c *LDAPConfig) { c.SearchAttr = "" }},
		{"MemberOf格式错误", func(c *LDAPConfig) { c.MemberOf = "not-a-dn" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mutate(&c)
			if err := c.ValidateConfig(); err == nil {
				t.Fatalf("场景 %q 应校验失败，但未报错", tc.name)
			}
		})
	}

	// MemberOf 为空允许，且合法 DN 通过
	ok := base
	ok.MemberOf = "ou=vpn,dc=example,dc=com"
	if err := ok.ValidateConfig(); err != nil {
		t.Fatalf("带合法 MemberOf 应校验通过，实得: %v", err)
	}
}

// username 来自未认证的 POST / 登录请求，进入过滤器前必须转义，
// 否则注入的 * ) ( 可改变 & 合取结构、取消 memberOf 限制（CWE-90）
func TestLDAPConfig_SearchFilter(t *testing.T) {
	c := &LDAPConfig{
		BaseDn:      "dc=example,dc=com",
		ObjectClass: "person",
		SearchAttr:  "sAMAccountName",
		MemberOf:    "ou=vpn,dc=example,dc=com",
	}

	cases := []struct {
		name     string
		username string
		expected string
	}{
		{"普通用户名原样通过", "zhangsan",
			"(&(objectClass=person)(sAMAccountName=zhangsan)(memberOf:=ou=vpn,dc=example,dc=com))"},
		{"注入通配符", "*",
			"(&(objectClass=person)(sAMAccountName=\\2a)(memberOf:=ou=vpn,dc=example,dc=com))"},
		{"注入永真条件", "zhangsan)(objectClass=*",
			"(&(objectClass=person)(sAMAccountName=zhangsan\\29\\28objectClass=\\2a)(memberOf:=ou=vpn,dc=example,dc=com))"},
		{"注入取反条件", "zhangsan)(!(memberOf=ou=vpn,dc=example,dc=com",
			"(&(objectClass=person)(sAMAccountName=zhangsan\\29\\28!\\28memberOf=ou=vpn,dc=example,dc=com)(memberOf:=ou=vpn,dc=example,dc=com))"},
		{"注入或运算", "a)(|(uid=",
			"(&(objectClass=person)(sAMAccountName=a\\29\\28|\\28uid=)(memberOf:=ou=vpn,dc=example,dc=com))"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filter := c.SearchFilter(tc.username)
			if filter != tc.expected {
				t.Fatalf("SearchFilter(%q) = %q, want %q", tc.username, filter, tc.expected)
			}
			if _, err := ldap.CompileFilter(filter); err != nil {
				t.Fatalf("转义后的过滤器应可编译，实得错误: %v", err)
			}
			// 受限组合取项必须仍在
			if !strings.Contains(filter, "(memberOf:=ou=vpn,dc=example,dc=com)") {
				t.Fatalf("memberOf 合取项丢失: %q", filter)
			}
		})
	}

	// 空用户名走存在断言分支（用户同步路径）
	empty := c.SearchFilter("")
	if want := "(&(objectClass=person)(sAMAccountName=*)(memberOf:=ou=vpn,dc=example,dc=com))"; empty != want {
		t.Fatalf("SearchFilter(\"\") = %q, want %q", empty, want)
	}

	// 未配置 MemberOf 时不追加合取项
	noMember := &LDAPConfig{ObjectClass: "person", SearchAttr: "sAMAccountName"}
	if want := "(&(objectClass=person)(sAMAccountName=zhangsan))"; noMember.SearchFilter("zhangsan") != want {
		t.Fatalf("SearchFilter() 无 MemberOf = %q, want %q", noMember.SearchFilter("zhangsan"), want)
	}
}

func TestCheckAccountStatus(t *testing.T) {
	cases := []struct {
		name    string
		uac     string // userAccountControl，空=不设置该属性
		shadow  string // shadowExpire，空=不设置
		wantErr bool
	}{
		{"正常账号(NORMAL=512)", "512", "", false},
		{"禁用账号(ACCOUNTDISABLE=2)", "2", "", true},
		{"锁定账号(LOCKOUT=16)", "16", "", true},
		{"shadowExpire已停用(1)", "", "1", true},
		{"无状态属性不误杀", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var attrs []*ldap.EntryAttribute
			if tc.uac != "" {
				attrs = append(attrs, &ldap.EntryAttribute{Name: "userAccountControl", Values: []string{tc.uac}})
			}
			if tc.shadow != "" {
				attrs = append(attrs, &ldap.EntryAttribute{Name: "shadowExpire", Values: []string{tc.shadow}})
			}
			err := CheckAccountStatus(&ldap.SearchResult{Entries: []*ldap.Entry{{Attributes: attrs}}})
			if tc.wantErr && err == nil {
				t.Fatalf("场景 %q 应报错但通过", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("场景 %q 应通过但报错: %v", tc.name, err)
			}
		})
	}
}
