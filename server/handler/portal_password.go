package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/notify"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/service"
	"html"
	"net"
	"net/http"
	"net/url"
	"time"
)

func (p *PortalService) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	user, ok := portalCurrentUser(r)
	if !ok {
		portalUnauthorized(w)
		return
	}
	if user.Type != "" && user.Type != "local" {
		portalError(w, "外部认证用户请到对应身份源修改密码")
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		portalError(w, "参数错误")
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		portalError(w, "新旧密码不能为空")
		return
	}
	if err := utils.CheckPasswordPolicy(req.NewPassword); err != nil {
		portalError(w, err.Error())
		return
	}
	n, err := service.ChangeLocalPassword(user, req.OldPassword, req.NewPassword)
	if err != nil {
		if errors.Is(err, service.ErrOldPassword) {
			portalError(w, "旧密码错误")
			return
		}
		base.Error("用户门户修改密码失败:", err)
		portalError(w, "修改密码失败")
		return
	}
	if n > 0 {
		base.Info("用户改密，已吊销", n, "个有效会话:", user.Username)
	}
	portalOK(w, map[string]string{"message": "密码修改成功"})
}

// 处理门户首次登录的强制改密，并继续认证流程
func (p *PortalService) ForceChangePassword(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
		Confirm     string `json:"new_password_confirm"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		portalError(w, "参数错误")
		return
	}
	if req.Token == "" || req.NewPassword == "" {
		portalError(w, "参数不能为空")
		return
	}
	if req.NewPassword != req.Confirm {
		portalError(w, "两次输入的密码不一致")
		return
	}
	if err := utils.CheckPasswordPolicy(req.NewPassword); err != nil {
		portalError(w, err.Error())
		return
	}

	sess, err := p.authSessions.Get(req.Token)
	if err != nil || sess.Ctx == nil {
		portalError(w, "改密会话已过期，请重新登录")
		return
	}
	username := sess.Ctx.Conn.Username
	if username == "" {
		portalError(w, "改密会话数据异常")
		return
	}
	user := &dbdata.User{}
	if err := dbdata.Users.GetByUsername(username, user); err != nil || user.Status != 1 {
		portalError(w, "用户不存在或已停用")
		return
	}
	if user.Type != "" && user.Type != "local" {
		portalError(w, "外部认证用户请到对应身份源修改密码")
		return
	}
	if !user.ForcePwd {
		portalError(w, "无需修改密码或会话已失效")
		return
	}

	if err := RunForcePasswordChange(sess.Ctx, username, req.NewPassword); err != nil {
		base.Error("用户门户强制改密失败:", err)
		portalError(w, "修改密码失败")
		return
	}

	sess.Ctx.Conn.RemoteAddr = r.RemoteAddr
	result := authsrv.Resume(sess.Ctx, auth.PipelineState{
		StepIdx:     sess.Ctx.StepIdx(),
		PassedSteps: sess.Ctx.PassedSteps(),
	})
	switch result.Result {
	case auth.StepPass:
		p.authSessions.Delete(req.Token)
		portalUser, err := service.ResolvePortalUser(username, result.GroupName, user, true)
		if err != nil {
			portalError(w, err.Error())
			return
		}
		p.locks.Success(username, r.RemoteAddr)
		resp := p.portalIssueLoginResponse(w, r, portalUser, "首次登录修改密码成功")
		if resp.Code != 0 {
			portalError(w, resp.Msg)
			return
		}
		portalOK(w, resp.Data)
	case auth.StepPending:
		sess.Ctx.SetStepIdx(result.State.StepIdx)
		sess.Ctx.SetPassedSteps(result.State.PassedSteps)
		p.authSessions.Save(req.Token, sess)
		portalOK(w, portalChallengeResponse(req.Token, result, sess.Ctx).Data)
	case auth.StepFail:
		if result.Err != nil {
			base.Warn("用户门户强制改密后续认证失败:", result.Err)
		}
		p.locks.Fail(username, r.RemoteAddr)
		if base.GetCfg().DisplayError && result.Err != nil {
			portalError(w, result.Err.Error())
			return
		}
		portalError(w, "修改密码失败")
	}
}

func (p *PortalService) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Email == "" {
		portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
		return
	}

	limiterKey := host + ":" + req.Username
	now := time.Now()
	p.forgotLimiter.mu.Lock()
	if t, ok := p.forgotLimiter.next[limiterKey]; ok && now.Before(t) {
		p.forgotLimiter.mu.Unlock()
		portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
		return
	}
	p.forgotLimiter.next[limiterKey] = now.Add(60 * time.Second)
	for k, t := range p.forgotLimiter.next {
		if now.After(t) {
			delete(p.forgotLimiter.next, k)
		}
	}
	p.forgotLimiter.mu.Unlock()

	user := &dbdata.User{}
	err = dbdata.Users.GetByUsername(req.Username, user)
	if err != nil || user.Status != 1 {
		portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
		return
	}
	if user.Type != "" && user.Type != "local" {
		portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
		return
	}
	if user.Email != req.Email {
		portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
		return
	}

	token, err := service.IssueResetToken(user)
	if err != nil {
		base.Error("用户门户生成重置token失败:", err)
		portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
		return
	}

	resetURL := getServerAddr(r) + "/ui/#/portal?reset_token=" + url.QueryEscape(token)
	htmlBody := fmt.Sprintf(`<html><body>
<h2>RemLink 密码重置</h2>
<p>您好 %s，</p>
<p>请点击下方链接重置您的密码（15 分钟内有效）：</p>
<p><a href="%s">%s</a></p>
	<p>如果您未发起此请求，请忽略此邮件。</p>
	</body></html>`, html.EscapeString(user.Username), html.EscapeString(resetURL), html.EscapeString(resetURL))

	smtpCfg := &dbdata.SettingSmtp{}
	if err := dbdata.Settings.Get(smtpCfg); err != nil || smtpCfg.Host == "" {
		base.Warn("用户门户SMTP未配置，跳过发送重置邮件:", user.Username)
	} else {
		base.SafeGo(func() {
			if err := notify.SendEmail(notify.Message{
				Subject: "RemLink 密码重置",
				To:      user.Email,
				Body:    htmlBody,
			}); err != nil {
				base.Error("用户门户发送重置邮件失败:", user.Username, err)
			}
		})
	}

	portalOK(w, map[string]string{"message": "如果账号匹配，重置邮件已发送"})
}

func (p *PortalService) VerifyResetPassword(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		portalOK(w, map[string]any{"valid": false})
		return
	}

	claims, err := service.ParseResetClaims(token)
	if err != nil {
		portalOK(w, map[string]any{"valid": false})
		return
	}
	if p.isTokenUsed(claims.JTI) {
		portalOK(w, map[string]any{"valid": false})
		return
	}

	user, err := service.LocalPasswordUser(claims.UserID)
	if err != nil {
		portalOK(w, map[string]any{"valid": false})
		return
	}
	portalOK(w, map[string]any{
		"valid":    true,
		"username": user.Username,
	})
}

func (p *PortalService) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal {
		http.NotFound(w, r)
		return
	}

	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		portalError(w, "参数错误")
		return
	}
	if req.Token == "" || req.NewPassword == "" {
		portalError(w, "参数不能为空")
		return
	}
	if err := utils.CheckPasswordPolicy(req.NewPassword); err != nil {
		portalError(w, err.Error())
		return
	}

	claims, err := service.ParseResetClaims(req.Token)
	if err != nil {
		portalError(w, err.Error())
		return
	}

	user, err := service.LocalPasswordUser(claims.UserID)
	if err != nil {
		if errors.Is(err, service.ErrExternalUser) {
			portalError(w, err.Error())
		} else {
			portalError(w, "用户不存在或已停用")
		}
		return
	}

	hashed, err := utils.PasswordHash(req.NewPassword)
	if err != nil {
		base.Error("用户门户重置密码哈希失败:", err)
		portalError(w, "重置密码失败")
		return
	}

	if p.checkAndMarkToken(claims.JTI) {
		portalError(w, "重置链接已失效，请重新申请")
		return
	}

	n, err := service.CommitPassword(user.Username, hashed)
	if err != nil {
		base.Error("用户门户重置密码失败:", err)
		portalError(w, "重置密码失败")
		return
	}
	if n > 0 {
		base.Info("用户重置密码，已吊销", n, "个有效会话:", user.Username)
	}

	portalOK(w, map[string]string{"message": "密码重置成功，请使用新密码登录"})
}
