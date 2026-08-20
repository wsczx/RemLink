// Flow 负责把认证管道结果分发给三端回调。

package handler

import (
	"net/http"
	"strings"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 三端对认证终态的响应回调
type FlowCallbacks struct {
	OnPass      func(flow *Flow, w http.ResponseWriter, r *http.Request) // 认证通过
	OnChallenge func(flow *Flow, w http.ResponseWriter, r *http.Request) // 需要二次挑战
	OnFail      func(flow *Flow, w http.ResponseWriter, r *http.Request) // 认证失败
}

// 承载一次认证流程的结果和请求元信息
type Flow struct {
	Result     *auth.PipelineResult
	Ctx        *auth.Context
	RemoteAddr string
	Username   string
	Source     string // 来源标识：native / webauth / portal，用于日志标注
	Callbacks  FlowCallbacks
	Session    *AuthSession // 非 nil 时，savePendingState 在写回断点后持久化会话
}

// 首次执行认证管道并按结果分发
func (f *Flow) Run(w http.ResponseWriter, r *http.Request) {
	result := authsrv.Authenticate(f.Ctx)
	f.dispatch(w, r, result)
}

// 统一记录认证失败日志，文案由 authFailMessage 生成，
func (f *Flow) RecordFail() {
	info := authFailMessage(f.Result.Err)
	if f.Source != "" {
		info = "[" + f.Source + "]" + info
	}
	recordFailAudit(f.Ctx.Conn, f.Username, f.RemoteAddr, info, f.Source == "门户")
}

// 供 RecordFail 与 WebAuth 锁定前置拦截点共用
func recordFailAudit(conn auth.ConnInfo, username, remoteAddr, info string, isPortal ...bool) {
	u := dbdata.UserActLog{
		Username:        username,
		RemoteAddr:      remoteAddr,
		Status:          dbdata.UserAuthFail,
		Info:            info,
		DeviceType:      conn.DeviceType,
		PlatformVersion: conn.PlatformVer,
	}
	if strings.Contains(info, "锁定") {
		u.IsLockedFail = true
	}
	dbdata.UserActLogIns.Add(u, conn.UserAgent, isPortal...)
}

// 从挂起状态恢复管道并按结果分发
func (f *Flow) Resume(w http.ResponseWriter, r *http.Request, state auth.PipelineState) {
	result := authsrv.Resume(f.Ctx, state)
	f.dispatch(w, r, result)
}

// 分发一个已算好的管道结果，不重新执行管道（供原生管道复用）
func (f *Flow) Dispatch(w http.ResponseWriter, r *http.Request, result *auth.PipelineResult) {
	f.dispatch(w, r, result)
}

// 统一处理锁定计数并按终态调用对应回调。
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

// 维护挑战阶段的锁定计数：首次进入重置，挑战码错误则累加
func (f *Flow) handlePendingLock(result *auth.PipelineResult) {
	if result.IsChallengeRetry() {
		lockManager.Fail(f.Username, f.RemoteAddr)
	} else {
		lockManager.Success(f.Username, f.RemoteAddr)
	}
}

// 把挑战断点写回认证会话，供下次 Resume
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
