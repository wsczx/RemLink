package handler

import (
	"encoding/json"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/dbdata"
	"net/http"
)

// 重新发送短信验证码
func (s *WebAuthService) ResendSMS(w http.ResponseWriter, r *http.Request) {
	pending, state, ok := s.Pending(w, r)
	if !ok {
		return
	}

	phone := ""
	if pending.Ctx != nil && pending.Ctx.SMS != nil {
		phone = pending.Ctx.SMS.Phone
	}
	if phone == "" {
		s.WriteError(w, "未找到手机号信息")
		return
	}

	if _, err := authsrv.SendSmsCode(phone); err != nil {
		s.WriteError(w, err.Error())
		return
	}

	if pending.Ctx != nil && pending.Ctx.SMS != nil {
		pending.Ctx.SMS.Sent = false
	}
	s.sessions.Save(state, pending)

	s.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

// 处理 WebAuth 强制改密提交（POST /web-auth/change_password）
// 校验强度并更新密码、清除 ForcePwd 后续跑管道（forcepwd 步直接通过，继续后续 otp 等步骤）
func (s *WebAuthService) ChangePassword(w http.ResponseWriter, r *http.Request) {
	pending, state, ok := s.Pending(w, r)
	if !ok {
		return
	}
	if pending.Ctx == nil {
		s.sessions.Delete(state)
		s.WriteError(w, "认证会话已过期")
		return
	}

	var req struct {
		NewPassword string `json:"new_password"`
		Confirm     string `json:"new_password_confirm"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.WriteError(w, "参数错误")
		return
	}
	if req.NewPassword == "" || req.NewPassword != req.Confirm {
		s.WriteError(w, "两次输入的密码不一致")
		return
	}

	username := pending.Ctx.Conn.Username

	user := &dbdata.User{}
	if err := dbdata.Users.GetByUsername(username, user); err != nil || user.Status != 1 {
		s.WriteError(w, "用户不存在或已停用")
		return
	}
	if user.Type != "" && user.Type != "local" {
		s.WriteError(w, "外部认证用户请到对应身份源修改密码")
		return
	}
	if !user.ForcePwd {
		s.WriteError(w, "无需修改密码或会话已失效")
		return
	}

	if err := RunForcePasswordChange(pending.Ctx, username, req.NewPassword); err != nil {
		s.WriteError(w, err.Error())
		return
	}
	s.locks.Success(username, r.RemoteAddr)

	s.ResumeDispatch(w, r, state, pending, username)
}
