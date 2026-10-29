package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 既无 Origin 也无 Referer 的写请求应拒绝，防跨站借用管理员 Cookie
func TestAdminSameOrigin_NoOriginNoReferer_Rejected(t *testing.T) {
	ast := assert.New(t)
	req := httptest.NewRequest(http.MethodPost, "http://admin.example.com/api/user", nil)
	req.Host = "admin.example.com"
	req.Header.Del("Origin")
	req.Header.Del("Referer")
	ast.False(adminSameOrigin(req), "既无 Origin 也无 Referer 的写请求应拒绝(防 CSRF)")
}

func TestAdminSameOrigin_SameOriginAllowed(t *testing.T) {
	ast := assert.New(t)
	req := httptest.NewRequest(http.MethodPost, "http://admin.example.com/api/user", nil)
	req.Host = "admin.example.com"
	req.Header.Set("Origin", "http://admin.example.com")
	ast.True(adminSameOrigin(req), "同源 Origin 应放行")
}

func TestAdminSameOrigin_CrossOriginRejected(t *testing.T) {
	ast := assert.New(t)
	req := httptest.NewRequest(http.MethodPost, "http://admin.example.com/api/user", nil)
	req.Host = "admin.example.com"
	req.Header.Set("Origin", "http://evil.example.com")
	ast.False(adminSameOrigin(req), "跨站 Origin 应拒绝")
}

// 用户名含路径分隔符时，zip 条目名必须收敛为基名，防路径遍历
func TestClientP12ZipEntryName_SanitizesPathTraversal(t *testing.T) {
	ast := assert.New(t)
	ast.Equal("user.p12", clientP12ZipEntryName("user"))
	ast.Equal("evil.p12", clientP12ZipEntryName("../../etc/evil"))
	ast.Equal("c.p12", clientP12ZipEntryName("a/b/c"))
	ast.Equal("user.p12", clientP12ZipEntryName(""))   // 空用户名兜底
	ast.Equal("z.p12", clientP12ZipEntryName(`x\y\z`)) // Windows 分隔符收敛为末段
}
