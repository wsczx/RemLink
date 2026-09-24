package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
)

// 用 mux.Router 承载 authMiddleware，使中间件内的 mux.CurrentRoute 能取到非 nil 路由
func setupClusterAuthRouter() *mux.Router {
	base.SetCfgForTest(&base.ServerConfig{
		JwtSecret: "test-secret-cluster-jwt-gating",
		AdminUser: "admin",
	})
	r := mux.NewRouter()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Handle("/cluster/nodes", authMiddleware(next)).Name("cluster_nodes")
	r.Handle("/cluster/status", authMiddleware(next)).Name("cluster_status")
	r.Handle("/set/restart", authMiddleware(next)).Name("set_restart")
	r.Handle("/set/profile", authMiddleware(next)).Name("set_profile")
	return r
}

func doAuthReq(router *mux.Router, method, path, jwt string) int {
	req := httptest.NewRequest(method, path, nil)
	if jwt != "" {
		req.Header.Set("Jwt", jwt)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

func adminUserJwt(t *testing.T) string {
	tok, err := SetJwtData(map[string]any{"admin_user": "admin"}, time.Now().Add(time.Minute).Unix())
	assert.Nil(t, err)
	return tok
}

func clusterNodeJwt(t *testing.T) string {
	tok, err := SetClusterJwtData()
	assert.Nil(t, err)
	return tok
}

// 验证节点端点鉴权门控：
// - 无 token → 401
// - 管理员 token → 所有路径放行
// - 节点间专用 token → 仅 /cluster/* 与 /set/restart 放行，访问普通管理端点返回 401
func TestAuthMiddleware_ClusterGating(t *testing.T) {
	ast := assert.New(t)
	router := setupClusterAuthRouter()

	ast.Equal(401, doAuthReq(router, "GET", "/set/profile", ""), "无 token 应 401")
	ast.Equal(401, doAuthReq(router, "GET", "/cluster/nodes", ""), "无 token 应 401")

	admin := adminUserJwt(t)
	ast.Equal(200, doAuthReq(router, "GET", "/set/profile", admin), "管理员 token 应可访问普通管理端点")
	ast.Equal(200, doAuthReq(router, "GET", "/cluster/nodes", admin), "管理员 token 应可访问节点端点")
	ast.Equal(200, doAuthReq(router, "GET", "/cluster/status", admin))
	ast.Equal(200, doAuthReq(router, "GET", "/set/restart", admin))

	cluster := clusterNodeJwt(t)
	ast.Equal(200, doAuthReq(router, "GET", "/cluster/nodes", cluster), "节点间 token 应可访问 /cluster/*")
	ast.Equal(200, doAuthReq(router, "GET", "/cluster/status", cluster))
	ast.Equal(200, doAuthReq(router, "GET", "/set/restart", cluster), "节点间 token 应可访问 /set/restart")
	ast.Equal(401, doAuthReq(router, "GET", "/set/profile", cluster), "节点间 token 不应访问普通管理端点")
}
