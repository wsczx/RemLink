// AuthFlow 是三端（原生 XML 管道 / WebAuth / 门户）共用的认证流程分发
//   - OnPass：认证通过（建 VPN 会话 / 签门户 JWT / WebVPN 免登）
//   - OnChallenge：需要二次挑战（渲染 XML / JSON）
//   - OnFail：认证失败（错误文案回落）
//
// 注意：本结构不持有协议状态机（由auth.Pipeline 的负责） Flow 仅负责
// 把已计算好的 PipelineResult 分发到回调，并在挑战路径统一维护锁定计数
//
// 新增认证方法时，只需在 auth 注册表登记一个新 step（实现 Authenticator/
// Challenger 接口），管道会自动编排，Flow 的锁定与分发逻辑无需改动。
//
// 锁定计数窗口约定（与历史行为一致）：
//   - 通过 / 首次进入挑战窗口：lockManager.Success 清零计数
//   - 挑战码错误（retry）：lockManager.Fail 累加
//   - 终态失败：lockManager.Fail 累加

package handler

import (
	"net/http"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
)

// 定义三端对管道终态的响应钩子
// 每个回调接收 *Flow（携带运行结果与请求上下文）
// 回调负责把结果写回 http.ResponseWriter —— Flow 本身不做任何渲染
type FlowCallbacks struct {
	// OnPass 认证完全通过。flow.Result 为 StepPass 结果（含 Username/GroupName）
	OnPass func(flow *Flow, w http.ResponseWriter, r *http.Request)
	// OnChallenge 需要二次挑战。flow.Result 为 StepPending 结果（含 Challenge/State）
	OnChallenge func(flow *Flow, w http.ResponseWriter, r *http.Request)
	// OnFail 认证失败。flow.Result.Err 携带错误（可能含 step 前缀）
	OnFail func(flow *Flow, w http.ResponseWriter, r *http.Request)
}

// 承载一次认证流程的运行结果与元信息，供回调使用
// Username 语义（跨端统一）：优先取管道结果的 Result.Username（续跑场景管道可能
// 重新解析出用户），否则回退到首认证请求里的 Ctx.Conn.Username。该值用于审计落库
// 与锁定计数，是「对谁加锁」的唯一权威来源，端点不得自行覆盖
type Flow struct {
	Result     *auth.PipelineResult
	Ctx        *auth.Context
	RemoteAddr string
	Username   string
	Callbacks  FlowCallbacks // Callbacks 由构造方注入
	Session    *AuthSession  // Session 可选注入。非 nil 时，savePendingState 在写回断点后自动持久化会话
}

// 首次执行认证管道（Authenticate），随后按结果分发
// 调用方需先完成：组解析所需的 Ctx.Conn.GroupName、凭据注入、锁定预检
func (f *Flow) Run(w http.ResponseWriter, r *http.Request) {
	// 含 otp 且首步非 local 时，Authenticate 内部已按需 LoadUserInfo
	// 其余情况管道第一个需要用户的 step 自行触发
	result := authsrv.Authenticate(f.Ctx)
	f.dispatch(w, r, result)
}

// 从挂起状态恢复管道（Resume），随后按结果分发
// state 为上次挑战保存的断点；hasChallengeResponse 表示本次带来了挑战响应码
func (f *Flow) Resume(w http.ResponseWriter, r *http.Request, state auth.PipelineState) {
	result := authsrv.Resume(f.Ctx, state)
	f.dispatch(w, r, result)
}

// 分发一个已计算好的管道结果，不重新执行管道
// 原生管道在 handlePipelineResult 中先执行 Authenticate/Resume 拿到 result
// 再调用 Dispatch 走统一的锁定计数 + 回调分发，避免重复执行管道
func (f *Flow) Dispatch(w http.ResponseWriter, r *http.Request, result *auth.PipelineResult) {
	f.dispatch(w, r, result)
}

// 统一处理锁定计数并按终态调用对应回调
func (f *Flow) dispatch(w http.ResponseWriter, r *http.Request, result *auth.PipelineResult) {
	f.Result = result
	f.RemoteAddr = r.RemoteAddr

	switch result.Result {
	case auth.StepPass:
		lockManager.Success(f.Username, f.RemoteAddr)
		if f.Callbacks.OnPass != nil {
			f.Callbacks.OnPass(f, w, r)
		}

	case auth.StepFail:
		lockManager.Fail(f.Username, f.RemoteAddr)
		if f.Callbacks.OnFail != nil {
			f.Callbacks.OnFail(f, w, r)
		}

	case auth.StepPending:
		f.handlePendingLock(result)
		if f.Callbacks.OnChallenge != nil {
			f.Callbacks.OnChallenge(f, w, r)
		}

	default:
		lockManager.Fail(f.Username, f.RemoteAddr)
		base.Warn("认证管道返回未知结果:", result.Result)
		if f.Callbacks.OnFail != nil {
			f.Callbacks.OnFail(f, w, r)
		}
	}
}

// 挑战阶段的锁定计数窗口
// 首次进入挑战窗口重置计数；挑战码错误（retry）则计入失败
func (f *Flow) handlePendingLock(result *auth.PipelineResult) {
	if result.IsChallengeRetry() {
		lockManager.Fail(f.Username, f.RemoteAddr)
	} else {
		lockManager.Success(f.Username, f.RemoteAddr)
	}
}

// 把挑战断点保存回认证会话，供下次 Resume
// 统一写回 StepIdx/PassedSteps；若 Flow.Session 已注入且持有 SessionID
func (f *Flow) savePendingState() {
	if f.Result == nil || f.Result.Result != auth.StepPending || f.Result.State.StepIdx < 0 {
		return
	}
	f.Ctx.SetStepIdx(f.Result.State.StepIdx)
	f.Ctx.SetPassedSteps(f.Result.State.PassedSteps)
	if f.Session != nil && f.Session.SessionID != "" {
		AuthSessionManager.Save(f.Session.SessionID, f.Session)
	}
}
