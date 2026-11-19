package handler

import (
	"encoding/json"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/notify"
	"net/http"
)

// 发送短信验证码
func (p *PortalService) SendSMS(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal || !notify.IsSmsConfigured() {
		portalError(w, "短信登录未启用")
		return
	}
	var req struct {
		Phone string `json:"phone"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<12)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Phone == "" {
		portalError(w, "请输入手机号")
		return
	}

	user := &dbdata.User{}
	if err := dbdata.Users.GetByPhone(req.Phone, user); err != nil || user.Status != 1 {
		portalError(w, "手机号未注册或账号异常")
		return
	}

	_, err := authsrv.SendSmsCode(req.Phone, r.RemoteAddr)
	if err != nil {
		base.Warn("Portal短信登录发送失败:", err)
		portalError(w, "验证码发送失败，请稍后重试")
		return
	}

	portalOK(w, map[string]any{
		"message":    "验证码已发送",
		"phone_tail": req.Phone[len(req.Phone)-4:],
	})
}

// 验证短信验证码并登录
func (p *PortalService) VerifySMS(w http.ResponseWriter, r *http.Request) {
	if !base.GetCfg().EnableUserPortal || !notify.IsSmsConfigured() {
		portalError(w, "短信登录未启用")
		return
	}
	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<12)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Phone == "" || req.Code == "" {
		portalError(w, "参数错误")
		return
	}

	if !p.locks.Check(req.Phone, r.RemoteAddr) {
		portalError(w, "验证过于频繁，请稍后重试")
		dbdata.UserActLogIns.Add(dbdata.UserActLog{RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "验证过于频繁，请稍后重试", IsLockedFail: true}, r.UserAgent(), true)
		return
	}

	_, err := authsrv.VerifySmsCode(req.Phone, req.Code)
	if err != nil {
		p.locks.Fail(req.Phone, r.RemoteAddr)
		portalError(w, err.Error())
		dbdata.UserActLogIns.Add(dbdata.UserActLog{RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "门户登录失败"}, r.UserAgent(), true)
		return
	}

	user := &dbdata.User{}
	if err := dbdata.Users.GetByPhone(req.Phone, user); err != nil || user.Status != 1 {
		portalError(w, "用户不存在或已禁用")
		dbdata.UserActLogIns.Add(dbdata.UserActLog{RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "门户登录失败"}, r.UserAgent(), true)
		return
	}

	p.locks.Success(req.Phone, r.RemoteAddr)
	resp := p.portalIssueLoginResponse(w, r, user, "短信验证码登录成功")
	if resp.Code != 0 {
		portalError(w, resp.Msg)
		dbdata.UserActLogIns.Add(dbdata.UserActLog{Username: user.Username, RemoteAddr: r.RemoteAddr, Status: dbdata.UserAuthFail, Info: "门户登录失败"}, r.UserAgent(), true)
		return
	}
	portalOK(w, resp.Data)
}
