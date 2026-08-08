package auth

import "testing"

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
		Addr:       "ldap.example.com:389",
		BindName:   "cn=admin,dc=example,dc=com",
		BindPwd:    "secret",
		BaseDn:     "dc=example,dc=com",
		ObjectClass: "person",
		SearchAttr: "cn",
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
