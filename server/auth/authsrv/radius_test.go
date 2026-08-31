package authsrv

import (
	"context"
	"net"
	"testing"

	"github.com/wsczx/remlink/auth"
	"layeh.com/radius"
	"layeh.com/radius/rfc2865"
)

var mockAddr string

func startMockRadius(t *testing.T, secret, mode string) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	mockAddr = conn.LocalAddr().String()

	handler := radius.HandlerFunc(func(w radius.ResponseWriter, r *radius.Request) {
		username := rfc2865.UserName_GetString(r.Packet)
		state := rfc2865.State_Get(r.Packet)

		resp := radius.New(radius.CodeAccessReject, []byte(secret))
		resp.Identifier = r.Packet.Identifier
		resp.Authenticator = r.Packet.Authenticator // 响应 Authenticator 基于请求计算

		switch mode {
		case "accept":
			resp.Code = radius.CodeAccessAccept

		case "reject":
			// Reject

		case "challenge":
			if len(state) > 0 {
				if username == "testuser" {
					resp.Code = radius.CodeAccessAccept
				}
			} else {
				// 首次：返回 Challenge
				if username == "testuser" {
					resp.Code = radius.CodeAccessChallenge
					rfc2865.State_Set(resp, []byte("test-state-12345678901234567890"))
					rfc2865.ReplyMessage_SetString(resp, "请输入二次验证码")
				}
			}
		}

		_ = w.Write(resp)
	})

	srv := &radius.PacketServer{
		SecretSource:       radius.StaticSecretSource([]byte(secret)),
		Handler:            handler,
		InsecureSkipVerify: true,
	}

	t.Cleanup(func() {
		_ = srv.Shutdown(context.Background())
		conn.Close()
	})

	go func() {
		_ = srv.Serve(conn)
	}()
}

func TestRADIUSAuth_Accept(t *testing.T) {
	secret := "shared-secret"
	startMockRadius(t, secret, "accept")

	a := &RADIUSAuth{}
	a.Secret = secret
	a.Addr = mockAddr

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username: "testuser",
			Password: "correctpass",
		},
	}

	result, err := a.Authenticate(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != auth.StepPass {
		t.Fatalf("expected StepPass, got %v", result)
	}
}

func TestRADIUSAuth_Reject(t *testing.T) {
	secret := "shared-secret"
	startMockRadius(t, secret, "reject")

	a := &RADIUSAuth{}
	a.Secret = secret
	a.Addr = mockAddr

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username: "testuser",
			Password: "wrongpass",
		},
	}

	result, err := a.Authenticate(ctx)
	if err == nil {
		t.Fatal("expected error for rejected auth")
	}
	if result != auth.StepFail {
		t.Fatalf("expected StepFail, got %v", result)
	}
}

func TestRADIUSAuth_Challenge_FirstCall(t *testing.T) {
	secret := "shared-secret"
	startMockRadius(t, secret, "challenge")

	a := &RADIUSAuth{}
	a.Secret = secret
	a.Addr = mockAddr

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username: "testuser",
			Password: "correctpass",
		},
	}

	result, err := a.Authenticate(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != auth.StepPending {
		t.Fatalf("expected StepPending, got %v", result)
	}

	if ctx.RADIUS == nil || len(ctx.RADIUS.State) == 0 {
		t.Fatal("radius_state not saved in RADIUS state")
	}
	msg := ""
	if ctx.RADIUS != nil {
		msg = ctx.RADIUS.ChallengeMsg
	}
	if msg == "" {
		t.Fatal("radius_challenge_msg not saved in RADIUS state")
	}
}

func TestRADIUSAuth_Challenge_Resume_CorrectCode(t *testing.T) {
	secret := "shared-secret"
	startMockRadius(t, secret, "challenge")

	a := &RADIUSAuth{}
	a.Secret = secret
	a.Addr = mockAddr

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username: "testuser",
			Password: "correctpass",
		},
		RADIUS: &auth.RADIUSState{
			State:         []byte("test-state-12345678901234567890"),
			ChallengeMsg:  "请输入二次验证码",
			ChallengeCode: "123456",
		},
	}

	result, err := a.Authenticate(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != auth.StepPass {
		t.Fatalf("expected StepPass, got %v", result)
	}
}

func TestRADIUSAuth_Challenge_Resume_WrongCode(t *testing.T) {
	secret := "shared-secret"
	// 用 reject 模拟 Radius 服务端拒绝错误验证码
	startMockRadius(t, secret, "reject")

	a := &RADIUSAuth{}
	a.Secret = secret
	a.Addr = mockAddr

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username: "testuser",
			Password: "correctpass",
		},
		RADIUS: &auth.RADIUSState{
			State:         []byte("test-state-12345678901234567890"),
			ChallengeMsg:  "请输入二次验证码",
			ChallengeCode: "000000",
		},
	}

	result, err := a.Authenticate(ctx)
	if err == nil {
		t.Fatal("expected error for wrong code")
	}
	if result != auth.StepFail {
		t.Fatalf("expected StepFail, got %v", result)
	}
}

func TestRADIUSAuth_Challenge_Resume_EmptyCode(t *testing.T) {
	secret := "shared-secret"
	startMockRadius(t, secret, "challenge")

	a := &RADIUSAuth{}
	a.Secret = secret
	a.Addr = mockAddr

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username: "testuser",
			Password: "correctpass",
		},
		RADIUS: &auth.RADIUSState{
			State:        []byte("test-state-12345678901234567890"),
			ChallengeMsg: "请输入二次验证码",
		},
	}

	result, err := a.Authenticate(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != auth.StepPending {
		t.Fatalf("expected StepPending for empty otp_code, got %v", result)
	}
}

func TestRADIUSAuth_EmptyUsername(t *testing.T) {
	a := &RADIUSAuth{}
	a.Secret = "secret"
	a.Addr = "127.0.0.1:0"

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username: "",
			Password: "pass",
		},
	}

	result, err := a.Authenticate(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != auth.StepPending {
		t.Fatalf("expected StepPending for empty username, got %v", result)
	}
}

func TestRADIUSAuth_EmptyPassword_NonResume(t *testing.T) {
	a := &RADIUSAuth{}
	a.Secret = "secret"
	a.Addr = "127.0.0.1:0"

	ctx := &auth.Context{
		Conn: auth.ConnInfo{
			Username: "testuser",
			Password: "",
		},
	}

	result, err := a.Authenticate(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != auth.StepPending {
		t.Fatalf("expected StepPending for empty password, got %v", result)
	}
}

func TestRADIUSAuth_ChallengeInfo(t *testing.T) {
	a := &RADIUSAuth{}
	info := a.Challenge()
	if info == nil {
		t.Fatal("expected non-nil ChallengeInfo")
	}
	if info.Type != auth.ChallengeRADIUS {
		t.Fatalf("expected ChallengeRADIUS, got %v", info.Type)
	}
}
