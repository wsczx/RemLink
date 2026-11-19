package handler

import (
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/webvpn"
	"net/http"
)

// 在 WebVPN 子域本地渲染「无权限」错误页
func webVpnForbiddenPage(m *webvpn.WebVPNManager, w http.ResponseWriter, r *http.Request, username, appName string) {
	msg := "您当前没有访问该应用的权限，如需开通请联系系统管理员。"
	if username != "" {
		msg = username + "，您当前没有访问「" + appName + "」的权限，如需开通请联系系统管理员。"
	}
	webVpnErrorHTML(m, w, r, webVpnErrorData{
		Reason:   "forbidden",
		Title:    "无权访问该应用",
		Subtitle: msg,
		AppName:  appName,
	})
}

// 应用不存在/已禁用时,同样在 WebVPN 子域本地渲染错误页
// reason 取值：notfound（不存在，返回 404）/ disabled（已禁用，返回 403）
func webVpnAppErrorPage(m *webvpn.WebVPNManager, w http.ResponseWriter, r *http.Request, appName, reason, msg string) {
	title := "应用不存在"
	status := http.StatusNotFound
	if reason == "disabled" {
		title = "应用已停用"
		status = http.StatusForbidden
	}
	webVpnErrorHTML(m, w, r, webVpnErrorData{
		Reason:     reason,
		Title:      title,
		Subtitle:   msg,
		AppName:    appName,
		StatusCode: status,
	})
}

// 错误页模板数据
type webVpnErrorData struct {
	Reason     string
	Title      string
	Subtitle   string
	AppName    string
	StatusCode int
}

func webVpnErrorHTML(m *webvpn.WebVPNManager, w http.ResponseWriter, r *http.Request, data webVpnErrorData) {
	if data.AppName != "" {
		if app, err := m.Apps().GetByName(data.AppName); err == nil && app != nil {
			writeWebVpnCORSHeadersForApp(w, r, app)
		}
	}
	if data.Title == "" {
		data.Title = "无法访问"
	}
	if data.Subtitle == "" {
		data.Subtitle = "无权访问该应用"
	}
	if data.StatusCode == 0 {
		data.StatusCode = http.StatusForbidden
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(data.StatusCode)
	if err := webVpnErrorTmpl.Execute(w, data); err != nil {
		base.Error("WebVPN 错误页渲染失败:", err)
	}
}
