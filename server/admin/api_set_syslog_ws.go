package admin

import "net/http"

// SyslogWSHandler WebSocket 系统日志实时推送处理函数，由 handler 包在启动时注入
var SyslogWSHandler func(w http.ResponseWriter, r *http.Request)

// WebSocket 系统日志实时推送入口
func SyslogWS(w http.ResponseWriter, r *http.Request) {
	if SyslogWSHandler != nil {
		SyslogWSHandler(w, r)
	}
}
