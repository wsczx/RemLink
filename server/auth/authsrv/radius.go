package authsrv

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"layeh.com/radius"
	"layeh.com/radius/rfc2865"
)

func init() {
	auth.Registry.Register("radius", func() auth.Authenticator {
		return &RADIUSAuth{}
	})
}

type RADIUSAuth struct {
	auth.RADIUSConfig
}

func (a *RADIUSAuth) Name() string { return "radius" }

func (a *RADIUSAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	if ctx.Conn.Username == "" {
		return auth.StepPending, nil
	}

	// Access-Challenge 恢复时必须带回服务端下发的 State。
	var prevState []byte
	isResume := ctx.RADIUS != nil && ctx.RADIUS.State != nil
	if isResume {
		prevState = ctx.RADIUS.State
	}

	// Resume 时用二次验证码作为 User-Password (RADIUS Challenge 协议要求)
	password := ctx.Conn.Password
	if isResume {
		code := ""
		if ctx.RADIUS != nil {
			code = ctx.RADIUS.ChallengeCode
		}
		if code == "" {
			return auth.StepPending, nil // 等待用户输入验证码
		}
		password = code
	} else if len(password) < 1 {
		return auth.StepPending, nil
	}

	packet := radius.New(radius.CodeAccessRequest, []byte(a.Secret))

	if err := rfc2865.UserName_SetString(packet, ctx.Conn.Username); err != nil {
		return auth.StepFail, fmt.Errorf("RADIUS 设置用户名失败: %w", err)
	}
	if err := rfc2865.UserPassword_SetString(packet, password); err != nil {
		return auth.StepFail, fmt.Errorf("RADIUS 设置密码失败: %w", err)
	}

	if isResume && len(prevState) > 0 {
		if err := rfc2865.State_Set(packet, prevState); err != nil {
			return auth.StepFail, fmt.Errorf("RADIUS 设置 State 失败: %w", err)
		}
	}

	if a.NasIP != "" {
		nasIP := net.ParseIP(a.NasIP)
		if err := rfc2865.NASIPAddress_Set(packet, nasIP); err != nil {
			return auth.StepFail, fmt.Errorf("RADIUS 设置 NAS IP 失败: %w", err)
		}
	}

	if ctx.Conn.MacAddr != "" {
		base.Trace("RADIUS MAC:", ctx.Conn.MacAddr)
		if err := rfc2865.CallingStationID_AddString(packet, ctx.Conn.MacAddr); err != nil {
			return auth.StepFail, fmt.Errorf("RADIUS 设置 MAC 失败: %w", err)
		}
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	response, err := radius.Exchange(reqCtx, packet, a.Addr)
	if err != nil {
		return auth.StepFail, fmt.Errorf("RADIUS 服务器连接异常: %w", err)
	}

	if response.Code == radius.CodeAccessAccept {
		ctx.SetInfo("RADIUS 认证通过")
		return auth.StepPass, nil
	}

	if response.Code == radius.CodeAccessChallenge {
		state := rfc2865.State_Get(response)
		replyMsg := rfc2865.ReplyMessage_GetString(response)

		rs := ctx.GetRADIUS()
		rs.State = state
		rs.ChallengeMsg = replyMsg

		ctx.SetInfo("RADIUS 需要二次验证")
		return auth.StepPending, nil
	}

	return auth.StepFail, fmt.Errorf("RADIUS 用户名或密码错误")
}

func (a *RADIUSAuth) Challenge() *auth.ChallengeInfo {
	return &auth.ChallengeInfo{
		Type:     auth.ChallengeRADIUS,
		Template: "radius_challenge",
	}
}
