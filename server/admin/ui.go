package admin

import (
	"net/http"
	"strings"

	"github.com/wsczx/remlink/dbdata"
)

// 提供前端静态资源；未知子路径回退 index.html，由前端展示 404 页面。
func ServeUI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/ui/")
		if p != "" {
			if _, err := UiData.Open("ui/" + p); err == nil {
				http.FileServer(http.FS(UiData)).ServeHTTP(w, r)
				return
			}
		}
		ServeIndex(w, r)
	})
}

// 默认品牌展示，未配置自定义品牌时回退
const (
	defaultBrandTitle   = "RemLink"
	defaultBrandFavicon = ""
)

// 写入前端 index.html；读取失败则返回 404。
func ServeIndex(w http.ResponseWriter, r *http.Request) {
	data, err := UiData.ReadFile("ui/index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	brand := dbdata.SettingPortalBrand{}
	_ = dbdata.SettingGet(&brand)

	title := brand.Title
	if title == "" {
		title = defaultBrandTitle
	}
	favicon := brand.Favicon

	html := string(data)
	html = strings.ReplaceAll(html, "{{BRAND_TITLE}}", title)
	html = strings.ReplaceAll(html, "{{BRAND_FAVICON}}", favicon)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// 未知 GET 路径回退 index.html（前端展示 404 页）
func NotFound() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			ServeIndex(w, r)
			return
		}
		http.NotFound(w, r)
	})
}
