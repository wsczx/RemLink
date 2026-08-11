// 将 auth.PipelineResult 的挑战信息收敛为与渲染格式无关的ChallengeView
// 再由各端序列化为 XML / WebAuth-JSON / Portal-JSON。
// 本文件只负责"挑战如何渲染"，不负责流程分发与锁定计数
//
// 分层关系（与 AuthFlow 的边界）：
//   - authflow.go 的 Flow 负责"按 StepPass/Fail/Pending 分发 + 统一锁定计数 +
//     存挑战断点"，分发到各端的 OnChallenge 回调；
//   - 本文件是各端 OnChallenge 回调内部共用的序列化工具：BuildChallengeView
//     构造一次与格式无关的挑战模型，ToXML/ToWebAuthJSON/ToPortalJSON 是三种
//     序列化出口。
// 挑战字段来源唯一，各端渲染差异（XML 标签 / JSON 字段名 / 状态码）收敛到各自的 ToXxx 方法

package handler

import (
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/dbdata"
)

// 规范化挑战状态码
// forcePwdCode 由调用方传入；WebAuth 与门户的 JSON 契约均使用 "change_pwd"
// （两端前端都按 change_pwd 切到改密表单）。注意：原生 XML 管道用的是另一套
// 模板常量 "force_pwd"（见 link_auth_tpl.go 的 tpl_request_force_pwd），与本函数无关
func challengeStatus(t auth.ChallengeType, forcePwdCode string) string {
	switch t {
	case auth.ChallengeOTP:
		return "otp"
	case auth.ChallengeSMS:
		return "sms"
	case auth.ChallengeRADIUS:
		return "radius"
	case auth.ChallengeForcePwd:
		return forcePwdCode
	case auth.ChallengeSSO:
		return "sso"
	default:
		return "credentials"
	}
}

// 所有字段都只在 BuildChallengeView 内构造一次，三端序列化时直接使用
type ChallengeView struct {
	Type        auth.ChallengeType
	Message     string // 统一文案（RADIUS challenge_msg / OTP 错误 / ForcePwd 提示）
	PhoneMasked string // SMS 脱敏手机号，仅 SMS 挑战有值
	Group       string
	SsoType     string // 仅 SSO 挑战有值
}

// 由管道结果构造挑战视图
// retry 表示挑战码错误（如 OTP 码不对），用于生成对应错误文案
func BuildChallengeView(result *auth.PipelineResult, ctx *auth.Context, retry bool) *ChallengeView {
	ch := result.Challenge
	if ch == nil {
		// NopChallenger（如 ldap/radius 缺凭据）→ 凭据输入界面
		return &ChallengeView{
			Type:    auth.ChallengeType(""),
			Group:   result.GroupName,
			Message: "请输入登录凭据",
		}
	}

	v := &ChallengeView{
		Type:    ch.Type,
		Group:   result.GroupName,
		SsoType: ssoTypeOf(ch),
	}

	switch ch.Type {
	case auth.ChallengeOTP:
		if retry {
			v.Message = "OTP 动态码错误，请重新输入"
		}
	case auth.ChallengeRADIUS:
		msg := ""
		if ctx != nil && ctx.RADIUS != nil {
			msg = ctx.RADIUS.ChallengeMsg
		}
		if retry {
			if msg == "" {
				v.Message = "验证失败，请重新输入二次验证码"
			}
		} else if msg == "" {
			v.Message = "请输入二次验证码"
		} else {
			v.Message = msg
		}
	case auth.ChallengeSMS:
		if ctx != nil && ctx.SMS != nil && ctx.SMS.Phone != "" {
			v.PhoneMasked = maskPhone(ctx.SMS.Phone)
		}
	case auth.ChallengeForcePwd:
		v.Message = "请设置新密码以继续登录"
	}
	return v
}

func ssoTypeOf(ch *auth.ChallengeInfo) string {
	if ch == nil {
		return ""
	}
	if t, ok := ch.Data["sso_type"].(string); ok {
		return t
	}
	return ""
}

// 脱敏手机号：前 3 + **** + 后 4。
func maskPhone(phone string) string {
	if len(phone) > 4 {
		return phone[:3] + "****" + phone[len(phone)-4:]
	}
	return phone
}

// ToXML 将挑战视图序列化为原生管道 XML 渲染数据
func (v *ChallengeView) ToXML() RequestData {
	data := RequestData{Group: v.Group, Groups: dbdata.GetGroupNamesNormal()}
	switch v.Type {
	case auth.ChallengeOTP:
		data.Error = v.Message
	case auth.ChallengeRADIUS:
		data.Error = v.Message
	case auth.ChallengeSSO:
		data.SsoType = v.SsoType
	case auth.ChallengeForcePwd:
		// 强制改密仅需 Group/Groups，ServerAddr 由调用方按 r 注入，无需额外字段
	}
	return data
}

// 序列化为 WebAuth 前端契约（status / hint / message / phone_masked）文案来源唯一
func (v *ChallengeView) ToWebAuthJSON() map[string]any {
	body := map[string]any{
		"status":  challengeStatus(v.Type, "change_pwd"),
		"message": v.Message,
	}
	if v.Message != "" {
		if v.Type == auth.ChallengeRADIUS {
			body["challenge_msg"] = v.Message
		} else {
			body["hint"] = v.Message
		}
	}
	if v.PhoneMasked != "" {
		body["phone_masked"] = v.PhoneMasked
	}
	return body
}

// 序列化为门户前端契约（status / message / session_id / token）
// ForcePwd 以 session_id 充当 token，复用前端既有 change_pwd 分支
func (v *ChallengeView) ToPortalJSON(sessionID string) map[string]any {
	body := map[string]any{
		"status":     challengeStatus(v.Type, "change_pwd"),
		"message":    v.Message,
		"session_id": sessionID,
	}
	if v.PhoneMasked != "" {
		body["phone_masked"] = v.PhoneMasked
	}
	if v.Type == auth.ChallengeForcePwd {
		body["token"] = sessionID
	}
	return body
}
