package handler

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/wsczx/remlink/admin"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/sessdata"
	"github.com/wsczx/remlink/webvpn"
)

// 标记 handler 生命周期是否已启动
var started atomic.Bool

// 聚合本进程全部监听服务实例，停止时由 App 显式关闭
type Servers struct {
	Admin *http.Server // 管理后台 HTTPS 服务
	TLS   *http.Server // VPN 主 TLS 服务（CSTP/AnyConnect/WebVPN）
	DTLS  net.Listener // 服务监听（可选，按配置启用）
}

// handler 启动依赖，由 App 逐一构造注入
type Services struct {
	SyslogHub    *SyslogHub
	AccessAudit  *AccessAudit
	Sessions     *sessdata.SessionManager
	Locks        *auth.LockManager
	Webvpn       *webvpn.WebVPNManager
	AuthSessions *AuthSessionManager
	Tunnel       *Tunnel
	Tap          *TapLink
	DTLSLimiter  *DTLSHandshakeLimiter
	Portal       *PortalService
	LinkAuth     *LinkAuthService
	WebAuth      *WebAuthService
	SSO          *SSOHandler
}

// 优雅关闭全部监听服务，超时由 ctx 控制
func (s *Servers) Shutdown(ctx context.Context) {
	if s == nil {
		return
	}
	if s.Admin != nil {
		_ = s.Admin.Shutdown(ctx)
	}
	if s.TLS != nil {
		_ = s.TLS.Shutdown(ctx)
	}
	if s.DTLS != nil {
		_ = s.DTLS.Close()
	}
}

// 初始化并启动所有服务组件：定时任务、认证会话、IP转发、管理后台等
// 依赖由 App 构造并经 services 注入；前置条件不满足时返回 error
func Start(ctx context.Context, services Services) (*Servers, error) {
	if started.Swap(true) {
		return nil, errors.New("服务已启动")
	}
	auditInstance = services.AccessAudit
	var (
		subsystemsStarted bool
		adminSrv          *http.Server
		tlsSrv            *http.Server
		dtlsLn            net.Listener
	)
	// 已启动资源按逆序回滚；started 最后复位，避免回滚期间被重复进入
	fail := func(err error) (*Servers, error) {
		if dtlsLn != nil {
			_ = dtlsLn.Close()
		}
		if tlsSrv != nil {
			_ = tlsSrv.Close()
		}
		if adminSrv != nil {
			_ = adminSrv.Close()
		}
		if subsystemsStarted {
			services.AuthSessions.Stop()
			services.Locks.Stop()
			services.Webvpn.Stop()
		}
		started.Store(false)
		return nil, err
	}
	if err := sessdata.Start(ctx, services.Sessions); err != nil {
		return fail(err)
	}

	services.Locks.Init()                  // 初始化防爆破定时器和IP白名单
	services.AuthSessions.Start()          // 启动认证会话管理器
	services.Sessions.CleanupAllNatRules() // 清理所有防火墙残留规则
	services.Webvpn.Start()                // 启动 WebVPN 子系统（吊销阈值加载 + 审计批处理）
	subsystemsStarted = true

	// 开启服务器转发
	err := sysctlSet("net.ipv4.ip_forward", "1")
	if err != nil {
		base.Warn(err)
	}

	val, err := sysctlGet("net.ipv4.ip_forward")
	if err != nil {
		return fail(fmt.Errorf("读取 IP 转发状态失败: %w", err))
	}
	if val != "1" {
		return fail(errors.New("请执行 sysctl -w net.ipv4.ip_forward=1 开启IP转发"))
	}

	// 启用 IPv6 转发，并允许主网卡接收 RA
	if base.GetCfg().Ipv6CIDR != "" {
		// 确保 IPv6 在已有接口和新建接口上启用
		if err := sysctlSet("net.ipv6.conf.all.disable_ipv6", "0"); err != nil {
			base.Warn(err)
		}
		if err := sysctlSet("net.ipv6.conf.default.disable_ipv6", "0"); err != nil {
			base.Warn(err)
		}
		if err := sysctlSet("net.ipv6.conf."+base.GetCfg().MasterDev+".disable_ipv6", "0"); err != nil {
			base.Warn(err)
		}
		if err := sysctlSet("net.ipv6.conf."+base.GetCfg().MasterDev+".accept_ra", "2"); err != nil {
			base.Warn(err)
		}
		// 同时设置 default 和 all，确保新建及已有接口都能转发 IPv6
		if err := sysctlSet("net.ipv6.conf.default.forwarding", "1"); err != nil {
			base.Warn(err)
		}
		if err := sysctlSet("net.ipv6.conf.all.forwarding", "1"); err != nil {
			base.Warn(err)
		}
		val, err := sysctlGet("net.ipv6.conf.all.forwarding")
		if err != nil {
			return fail(fmt.Errorf("读取 IPv6 转发状态失败: %w", err))
		}
		if val != "1" {
			return fail(errors.New("请执行 sysctl -w net.ipv6.conf.all.forwarding=1 开启IPv6转发"))
		}
	}

	var linkErr error
	switch base.GetCfg().LinkMode {
	case base.LinkModeTUN:
		linkErr = testTun(services.Sessions.Firewall().Get())
	case base.LinkModeTAP:
		linkErr = services.Tap.checkAndSetGateway()
	case base.LinkModeMacvtap:
		linkErr = testMacvtap(services.Tap)
	default:
		linkErr = errors.New("LinkMode 配置错误")
	}
	if linkErr != nil {
		return fail(fmt.Errorf("数据链路检查失败: %w", linkErr))
	}

	// 系统日志 WebSocket Hub 与审计批量写入
	StartSyslogHub(ctx, services.SyslogHub)
	base.SafeGo(func() { logAuditBatch(ctx, services.AccessAudit) })

	adminSrv, err = admin.StartAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleSyslogWS(services.SyslogHub, w, r)
	}), &admin.Api{Sessions: services.Sessions, Locks: services.Locks})
	if err != nil {
		return fail(fmt.Errorf("管理后台启动失败: %w", err))
	}
	tlsSrv, err = startTls(services)
	if err != nil {
		return fail(err)
	}
	dtlsLn, err = startDTLS(ctx, services)
	if err != nil {
		return fail(err)
	}
	return &Servers{Admin: adminSrv, TLS: tlsSrv, DTLS: dtlsLn}, nil
}

func Stop(services Services) {
	if !started.Swap(false) {
		return
	}
	services.AuthSessions.Stop() // 停止认证会话管理器
	services.Locks.Stop()        // 停止防暴力破解清理协程
	services.Webvpn.Stop()       // 停止 WebVPN 子系统
	dbdata.Stop()                // 停止数据库
	destroyVtap()                // 销毁虚拟网卡
	services.Sessions.StopFakeDNS()
	// 清理全局NAT规则
	if fw := services.Sessions.Firewall().GetFirewallInstance(); fw != nil {
		fw.CleanupGlobal()
	}
}
