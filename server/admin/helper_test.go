package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

func TestReadJSON(t *testing.T) {
	assert := assert.New(t)

	t.Run("正常解析", func(t *testing.T) {
		body := `{"name":"test","value":42}`
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		w := httptest.NewRecorder()
		var v struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}
		err := readJSON(w, r, 1<<16, &v)
		assert.Nil(err)
		assert.Equal("test", v.Name)
		assert.Equal(42, v.Value)
	})

	t.Run("非法JSON返回错误且已写响应", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
		w := httptest.NewRecorder()
		var v map[string]any
		err := readJSON(w, r, 1<<16, &v)
		assert.Error(err)
		assert.Equal(http.StatusOK, w.Code) // RespError 写 200 + 错误体
		assert.Contains(w.Body.String(), "code")
	})

	t.Run("超限body被拒绝", func(t *testing.T) {
		big := strings.Repeat("a", 200)
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"x":"`+big+`"}`))
		w := httptest.NewRecorder()
		var v map[string]any
		err := readJSON(w, r, 64, &v) // 上限 64 字节
		assert.Error(err)
	})
}

// B4 回归：readJSON 出错时自身已写好错误响应，调用方不得再写
// （曾出现"两个 JSON 响应体拼接"，客户端无法解析）
func TestReadJSONErrorSingleResponse(t *testing.T) {
	assert := assert.New(t)

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	var v map[string]any
	err := readJSON(w, r, 1<<16, &v)
	assert.Error(err)

	// 整个响应体必须恰好是一个完整 JSON 对象
	dec := json.NewDecoder(w.Body)
	var resp Resp
	assert.Nil(dec.Decode(&resp))
	assert.Equal(RespParamErr, resp.Code) // JSON 格式错误归为参数错误
	// 解码后不得再有多余内容
	_, err = dec.Token()
	assert.Error(err, "响应体只允许一个 JSON 对象，不允许拼接第二个")
}

// B4 回归（端点侧）：真实 handler 走 readJSON 失败路径时
// 整个响应必须仍是单个 JSON 对象
func TestReadJSONCallerNoDoubleWrite(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"LoginOTP", LoginOTP},
		{"UserSet", UserSet},
		{"GroupSet", GroupSet},
		{"PolicySet", PolicySet},
		{"UserBatchDelete", UserBatchDelete},
		{"UnlockUser", UnlockUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)

			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
			w := httptest.NewRecorder()
			tc.handler(w, r)

			dec := json.NewDecoder(w.Body)
			var resp Resp
			assert.Nil(dec.Decode(&resp))
			assert.Equal(RespParamErr, resp.Code)
			_, err := dec.Token()
			assert.Error(err, "不得出现拼接的第二个 JSON 响应体")
		})
	}
}

func TestParseID(t *testing.T) {
	assert := assert.New(t)

	t.Run("正常id", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Form = url.Values{"id": {"123"}}
		w := httptest.NewRecorder()
		id, ok := parseID(w, r, "Id错误")
		assert.True(ok)
		assert.Equal(123, id)
	})

	t.Run("非法id", func(t *testing.T) {
		for _, bad := range []string{"", "0", "-1", "abc"} {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.Form = url.Values{"id": {bad}}
			w := httptest.NewRecorder()
			_, ok := parseID(w, r, "Id错误")
			assert.False(ok, "id=%q 应判非法", bad)
		}
	})
}

func TestParsePaging(t *testing.T) {
	assert := assert.New(t)

	t.Run("正常参数", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/?page=3&page_size=20", nil)
		page, size := parsePaging(r)
		assert.Equal(3, page)
		assert.Equal(20, size)
	})

	t.Run("缺省回退默认", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		page, size := parsePaging(r)
		assert.Equal(1, page)
		assert.Equal(dbdata.PageSize, size)
	})

	t.Run("非法值回退默认", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/?page=0&page_size=-5", nil)
		page, size := parsePaging(r)
		assert.Equal(1, page)
		assert.Equal(dbdata.PageSize, size)
	})
}

// 验证泛型 Detail/Del 辅助（需 DB，走临时 sqlite）
func TestDetailByIDDelByID(t *testing.T) {
	assert := assert.New(t)
	base.Test()
	base.SetCfgForTest(&base.ServerConfig{JwtSecret: "test-secret-for-helper"})

	tmpDb := t.TempDir() + "/helper_test.db"
	base.UpdateCfg(func(c *base.ServerConfig) {
		c.DbType = "sqlite3"
		c.DbSource = tmpDb
	})
	dbdata.Start()
	defer dbdata.Stop()

	// 造一条策略记录
	p := dbdata.Policy{Name: "helper-test-policy", Status: 1}
	assert.Nil(dbdata.Add(&p))

	t.Run("Detail查询存在记录", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Form = url.Values{"id": {itoa(p.Id)}}
		w := httptest.NewRecorder()
		detailByID[dbdata.Policy](w, r, "Id错误")
		assert.Contains(w.Body.String(), "helper-test-policy")
	})

	t.Run("Detail查询不存在记录返回错误", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Form = url.Values{"id": {"99999"}}
		w := httptest.NewRecorder()
		detailByID[dbdata.Policy](w, r, "Id错误")
		assert.NotContains(w.Body.String(), "helper-test-policy")
	})

	t.Run("Del删除并执行回调", func(t *testing.T) {
		p2 := dbdata.Policy{Name: "to-delete", Status: 1}
		assert.Nil(dbdata.Add(&p2))

		called := false
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Form = url.Values{"id": {itoa(p2.Id)}}
		w := httptest.NewRecorder()
		delByID(w, r, "Id错误", func(rec *dbdata.Policy) {
			called = true
			assert.Equal("to-delete", rec.Name)
		})
		assert.True(called, "onDeleted 回调应被执行")

		// 验证已删除
		var check dbdata.Policy
		assert.Error(dbdata.One("Id", p2.Id, &check))
	})

	t.Run("Del不存在记录返回错误", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Form = url.Values{"id": {"99999"}}
		w := httptest.NewRecorder()
		called := false
		delByID(w, r, "Id错误", func(rec *dbdata.Policy) { called = true })
		assert.False(called, "记录不存在时不应执行回调")
	})
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return strings.Trim(string(b), `"`)
}
