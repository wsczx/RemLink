package auth

import (
	"crypto/tls"
	"encoding/json"
	"strings"
	"time"
)

type StepResult int

const (
	StepPass    StepResult = iota // 认证通过，继续下一步
	StepPending                   // 需要客户端额外输入（如 OTP 码、SSO 重定向）
	StepFail                      // 认证失败，终止管道
)

func (s StepResult) String() string {
	switch s {
	case StepPass:
		return "Pass"
	case StepPending:
		return "Pending"
	case StepFail:
		return "Fail"
	default:
		return "Unknown"
	}
}

// 保存客户端连接信息
type ConnInfo struct {
	Username    string
	Nickname    string
	Password    string
	GroupName   string
	RemoteAddr  string
	UserAgent   string
	MacAddr     string
	DeviceID    string
	DeviceType  string
	PlatformVer string
	TLS         *tls.ConnectionState
}

// 保存认证所需的用户信息
type UserInfo struct {
	Username   string
	Nickname   string
	Type       string
	Groups     []string
	OtpSecret  string
	DisableOtp bool
	LimitTime  *time.Time
	Phone      string
	Email      string
	ForcePwd   bool // 强制改密
}

// 返回账号及昵称
func (u UserInfo) DisplayName() string {
	if u.Nickname == "" {
		return u.Username
	}
	return u.Username + "(" + u.Nickname + ")"
}

// 保存 OTP 步骤状态
type OTPState struct {
	Code string // OTP动态码
	Sent bool   // 是否已发送过 OTP（防重复发送）
}

// 保存短信验证状态
type SMSState struct {
	Phone string
	Code  string
	Sent  bool
}

// 保存 RADIUS 挑战状态
type RADIUSState struct {
	State         []byte // Access-Challenge 服务端下发的 State，Resume 时须带回
	ChallengeCode string // 二次验证码
	ChallengeMsg  string // 服务端下发的挑战提示
}

// 保存 SSO 认证状态
type SSOState struct {
	Type             string // "wxwork"|"feishu"
	From             string // 来源标记："portal"|"web_auth" 等
	ClientIP         string // 发起登录的客户端地址，回调时校验来源防重放
	Authenticated    bool   // SSO 回调已完成，身份+部门在回调阶段校验
	UserID           string // SSO 返回的用户 ID
	Code             string // OAuth code（管道内 SSO 流程）
	WebAuthCompleted bool   // WebAuth 流程已完成
	WebAuthUsername  string
	WebAuthNickname  string // WebAuth 认证阶段的显示昵称，顺流携带避免反查
	WebAuthGroup     string
	Redirect         string // 登录成功后回跳地址（如 WebVPN 子域名 URL），空则回门户首页
}

// 标识认证挑战类型
type ChallengeType string

const (
	ChallengeOTP      ChallengeType = "otp"       // OTP 动态码输入
	ChallengeSSO      ChallengeType = "sso"       // SSO 重定向
	ChallengeRADIUS   ChallengeType = "radius"    // RADIUS Access-Challenge（二次验证提示）
	ChallengeSMS      ChallengeType = "sms"       // 短信验证码输入
	ChallengeForcePwd ChallengeType = "force_pwd" // 强制改密：原生客户端弹极简改密页 / WebAuth 内联表单
)

// 保存认证器返回的挑战内容
type ChallengeInfo struct {
	Type     ChallengeType  // 挑战类型
	Template string         // XML/HTML 模板名称（handler 层使用）
	Data     map[string]any // 模板数据
}

// 是认证步骤的接口
type Authenticator interface {
	Name() string
	Authenticate(ctx *Context) (StepResult, error)
}

// 是支持交互式挑战的认证器接口
type Challenger interface {
	Authenticator
	Challenge() *ChallengeInfo
}

// 是不发起挑战的默认实现
type NopChallenger struct{}

func (NopChallenger) Challenge() *ChallengeInfo { return nil }

// 保存一次认证流程的上下文
type Context struct {
	Conn     ConnInfo
	UserInfo *UserInfo
	Identity string // 身份断言（证书 CN 等），供管道末尾一致性检查

	OTP    *OTPState
	SMS    *SMSState
	RADIUS *RADIUSState
	SSO    *SSOState

	PortalLogin bool // Portal 登录流程
	WebAuth     bool // WebAuth 浏览器认证流程

	info        string
	passedSteps []string
	stepIdx     int // 管道暂停步号（Resume 断点），由 handler 在 StepPending 时从 result.State.StepIdx 保存
}

func (c *Context) SetInfo(s string) {
	c.info = s
}

func (c *Context) AddPassedStep(name string) {
	c.passedSteps = append(c.passedSteps, name)
}

func (c *Context) LogInfo() string {
	// 多步认证优先展示完整流程。
	if len(c.passedSteps) > 1 {
		return buildInfoFromSteps(c.passedSteps)
	}
	if c.info != "" {
		return c.info
	}
	return buildInfoFromSteps(c.passedSteps)
}

// 认证步骤的展示名称。
var stepNameMap = map[string]string{
	"local":    "本地密码",
	"ldap":     "LDAP",
	"radius":   "RADIUS",
	"cert":     "证书",
	"otp":      "OTP",
	"sms":      "短信验证",
	"wxwork":   "企微",
	"feishu":   "飞书",
	"dingtalk": "钉钉",
	"admin":    "管理员认证",
}

func buildInfoFromSteps(steps []string) string {
	if len(steps) == 0 {
		return "认证成功"
	}
	names := make([]string, 0, len(steps))
	for _, s := range steps {
		if n, ok := stepNameMap[s]; ok {
			names = append(names, n)
		} else {
			names = append(names, s)
		}
	}
	return strings.Join(names, "+") + "认证通过"
}

func (c *Context) Username() string          { return c.Conn.Username }
func (c *Context) Password() string          { return c.Conn.Password }
func (c *Context) GroupName() string         { return c.Conn.GroupName }
func (c *Context) RemoteAddr() string        { return c.Conn.RemoteAddr }
func (c *Context) MacAddr() string           { return c.Conn.MacAddr }
func (c *Context) SetUserInfo(u *UserInfo)   { c.UserInfo = u }
func (c *Context) UserInfoLoaded() *UserInfo { return c.UserInfo }

func (c *Context) GetOTP() *OTPState {
	if c.OTP == nil {
		c.OTP = &OTPState{}
	}
	return c.OTP
}

func (c *Context) GetSMS() *SMSState {
	if c.SMS == nil {
		c.SMS = &SMSState{}
	}
	return c.SMS
}

func (c *Context) GetRADIUS() *RADIUSState {
	if c.RADIUS == nil {
		c.RADIUS = &RADIUSState{}
	}
	return c.RADIUS
}

func (c *Context) GetSSO() *SSOState {
	if c.SSO == nil {
		c.SSO = &SSOState{}
	}
	return c.SSO
}

// 回填已通过的步骤列表，供 Resume 从断点恢复。
func (c *Context) SetPassedSteps(steps []string) {
	c.passedSteps = steps
}

func (c *Context) PassedSteps() []string {
	return c.passedSteps
}

func (c *Context) SetStepIdx(idx int) {
	c.stepIdx = idx
}

func (c *Context) StepIdx() int {
	return c.stepIdx
}

// 管道暂停状态
type PipelineState struct {
	StepIdx     int
	PassedSteps []string
}

// 管道执行结果。
type PipelineResult struct {
	Result    StepResult
	Err       error
	Username  string
	GroupName string
	Info      string

	LimitTime *time.Time // 用户过期时间，由认证器加载到 ctx.UserInfo 后填入

	Challenge   *ChallengeInfo
	State       PipelineState
	PrevStepIdx int // 执行前的管道步号，首次认证为 -1。用于判断挑战是否原地踏步。
}

// IsChallengeRetry 判断管道是否在挑战步骤原地踏步（即挑战码错误，未推进到下一步）。
// PrevStepIdx < 0 表示首次认证
func (r *PipelineResult) IsChallengeRetry() bool {
	if r.PrevStepIdx < 0 {
		return false
	}
	return r.Result == StepPending && r.State.StepIdx == r.PrevStepIdx
}

// 认证服务需要的组基本信息。
type GroupInfo struct {
	Name        string
	AuthProfile json.RawMessage
}
