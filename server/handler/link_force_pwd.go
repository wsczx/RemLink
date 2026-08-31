package handler

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
)

// auth_request_force_pwd 强制改密挑战 XML
var auth_request_force_pwd = `<?xml version="1.0" encoding="UTF-8"?>
<config-auth client="vpn" type="auth-request" aggregate-auth-version="2">
    <opaque is-for="sg">
        <tunnel-group>{{.Group}}</tunnel-group>
        <group-alias>{{.Group}}</group-alias>
        <aggauth-handle>168179266</aggauth-handle>
        <config-hash>1595829378234</config-hash>
        <auth-method>single-sign-on-v2</auth-method>
    </opaque>
    <auth id="main">
        <title>首次登录，请修改密码</title>
        <message>首次登录，请修改密码后继续</message>
        <banner></banner>
        <sso-v2-login>{{.ServerAddr}}/+CSCOE+/force-pwd?state={{.State}}</sso-v2-login>
        <sso-v2-login-final>{{.ServerAddr}}/+CSCOE+/saml_ac_login.html</sso-v2-login-final>
        <sso-v2-token-cookie-name>acSamlv2Token</sso-v2-token-cookie-name>
        <form>
            <input type="sso" name="sso-token"></input>
        </form>
    </auth>
</config-auth>`

var forcePwdPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>首次登录，请修改密码</title>
    <style>
        * { box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "PingFang SC", "Microsoft YaHei", sans-serif; background: linear-gradient(135deg, #1b2138 0%, #2a3a5c 40%, #1a3668 100%); display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; padding: 16px; }
        .container { background: #ffffff; border-radius: 16px; box-shadow: 0 20px 60px rgba(0,0,0,0.3); padding: 40px 40px 32px; width: 100%; max-width: 380px; }
        .icon { width: 56px; height: 56px; margin: 0 auto 16px; border-radius: 14px; background: linear-gradient(135deg, #409eff, #66b1ff); display: flex; align-items: center; justify-content: center; }
        .icon span { color: #fff; font-size: 28px; line-height: 1; }
        h2 { margin: 0 0 8px; text-align: center; color: #303133; font-size: 20px; font-weight: 600; }
        .hint { text-align: center; color: #909399; font-size: 13px; margin: 0 0 24px; }
        form { display: flex; flex-direction: column; gap: 14px; }
        input { height: 44px; padding: 0 14px; border: 1px solid #dcdfe6; border-radius: 4px; font-size: 14px; color: #303133; outline: none; transition: border-color .2s; }
        input:focus { border-color: #409eff; }
        button { height: 44px; background: #409eff; color: #fff; border: none; border-radius: 4px; font-size: 15px; font-weight: 600; letter-spacing: 4px; cursor: pointer; transition: background .2s; }
        button:hover { background: #66b1ff; }
        .error { color: #f56c6c; font-size: 13px; margin: 0; text-align: center; }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon"><span>&#128274;</span></div>
        <h2>首次登录，请修改密码</h2>
        <p class="hint">新密码规则：至少8位且须包含字母和数字</p>
        <form method="post" action="/+CSCOE+/force-pwd/submit">
            <input type="hidden" name="state" value="{{.State}}">
            <input type="password" name="new_password" placeholder="新密码" autocomplete="new-password" required>
            <input type="password" name="new_password_confirm" placeholder="确认新密码" autocomplete="new-password" required>
            <button type="submit">修改并继续</button>
        </form>
        {{if .Error}}<p class="error">{{.Error}}</p>{{end}}
    </div>
</body>
</html>`

// 改密页（GET /+CSCOE+/force-pwd?state=xxx）。
func ForcePwdPage(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "缺少认证参数", http.StatusBadRequest)
		return
	}
	if _, err := AuthSessionManager.Get(state); err != nil {
		http.Error(w, "认证会话已过期，请重新连接", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	t, _ := template.New("force_pwd").Parse(forcePwdPageHTML)
	_ = t.Execute(w, map[string]string{"State": state})
}

// 处理改密提交（POST /+CSCOE+/force-pwd/submit）。
func ForcePwdSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := r.ParseForm(); err != nil {
		forcePwdMessage(w, "请求参数错误", "", false)
		return
	}
	state := r.FormValue("state")
	newPwd := r.FormValue("new_password")
	confirm := r.FormValue("new_password_confirm")
	if state == "" || newPwd == "" {
		forcePwdMessage(w, "参数错误", "", false)
		return
	}
	if newPwd != confirm {
		forcePwdMessage(w, "两次输入的密码不一致", state, false)
		return
	}
	if err := utils.CheckPasswordPolicy(newPwd); err != nil {
		forcePwdMessage(w, err.Error(), state, false)
		return
	}
	sess, err := AuthSessionManager.Get(state)
	if err != nil {
		forcePwdMessage(w, "认证会话已过期，请重新连接 VPN", "", false)
		return
	}
	if !sess.ForcePwd {
		forcePwdMessage(w, "会话状态异常", "", false)
		return
	}
	username := sess.Ctx.Conn.Username
	if !lockManager.Check(username, r.RemoteAddr) {
		forcePwdMessage(w, "账户已被锁定，请联系管理员", "", false)
		return
	}
	// 复用共享的强制改密核心：校验策略 → 哈希 → 写库（pin_code + 清除 change_pwd）→ 重载会话用户。
	if err := RunForcePwdChange(sess.Ctx, username, newPwd); err != nil {
		base.Error("强制改密失败:", err)
		forcePwdMessage(w, "修改密码失败", state, false)
		return
	}
	lockManager.Success(username, r.RemoteAddr)
	// 强制改密只走 Cisco AnyConnect 内置浏览器
	encodeState := base64.StdEncoding.EncodeToString([]byte(state))
	SetCookie(w, "acSamlv2Token", encodeState, 0)
	http.Redirect(w, r, "/+CSCOE+/saml_ac_login.html", http.StatusFound)
}

// 渲染提示页：带 state 时回显错误表单，否则显示成功/提示文案
func forcePwdMessage(w http.ResponseWriter, msg, state string, success bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if state != "" {
		t, _ := template.New("force_pwd_err").Parse(forcePwdPageHTML)
		_ = t.Execute(w, map[string]string{"State": state, "Error": msg})
		return
	}
	iconColor, iconChar, title := "linear-gradient(135deg,#67c23a,#85ce61)", "&#10003;", "操作完成"
	if !success {
		iconColor, iconChar, title = "linear-gradient(135deg,#f56c6c,#f78989)", "&#10007;", "操作失败"
	}
	fmt.Fprintf(w, `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>提示</title>
<style>*{box-sizing:border-box}body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,"PingFang SC","Microsoft YaHei",sans-serif;background:linear-gradient(135deg,#1b2138 0%%,#2a3a5c 40%%,#1a3668 100%%);display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;padding:16px}.container{background:#fff;border-radius:16px;box-shadow:0 20px 60px rgba(0,0,0,0.3);padding:40px;max-width:380px;text-align:center}.icon{width:56px;height:56px;margin:0 auto 16px;border-radius:14px;background:%s;display:flex;align-items:center;justify-content:center}.icon span{color:#fff;font-size:28px;line-height:1}h2{margin:0 0 8px;color:#303133;font-size:20px;font-weight:600}p{color:#909399;font-size:14px;line-height:1.6;margin:0}</style>
</head><body><div class="container"><div class="icon"><span>%s</span></div><h2>%s</h2><p>%s</p></div></body></html>`, iconColor, iconChar, title, template.HTMLEscapeString(msg))
}
