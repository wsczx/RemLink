package handler

import (
	"github.com/wsczx/remlink/admin"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/cron"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/sessdata"
	"github.com/wsczx/remlink/webvpn"
)

// 初始化并启动所有服务组件：数据库、定时任务、认证会话、IP转发、管理后台等
func Start() {
	dbdata.Start()
	sessdata.Start()
	if err := cron.Start(); err != nil {
		base.Error("启动定时任务失败:", err)
	}

	auth.GetLockManager().Init()  // 初始化防爆破定时器和IP白名单
	AuthSessionManager.Start()    // 启动认证会话管理器
	sessdata.CleanupAllNatRules() // 清理所有防火墙残留规则
	webvpn.GetManager().Start()   // 启动 WebVPN 子系统（吊销阈值加载 + 审计批处理）

	// 开启服务器转发
	err := sysctlSet("net.ipv4.ip_forward", "1")
	if err != nil {
		base.Warn(err)
	}

	val, err := sysctlGet("net.ipv4.ip_forward")
	if err != nil || val != "1" {
		base.Fatal("请执行 sysctl -w net.ipv4.ip_forward=1 开启IP转发")
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
		if err != nil || val != "1" {
			base.Fatal("请执行 sysctl -w net.ipv6.conf.all.forwarding=1 开启IPv6转发")
		}
	}

	switch base.GetCfg().LinkMode {
	case base.LinkModeTUN:
		checkTun()
	case base.LinkModeTAP:
		checkTap()
	case base.LinkModeMacvtap:
		checkMacvtap()
	default:
		base.Fatal("LinkMode 配置错误")
	}

	// 注册系统日志 WebSocket 实时推送回调
	base.BroadcastSyslogFunc = BroadcastSyslog
	// 注册系统日志 WebSocket Handler 到 admin 包
	admin.SyslogWSHandler = HandleSyslogWS
	// 启动系统日志 WebSocket Hub
	StartSyslogHub()
	// 启动审计日志批量写入
	go logAuditBatch()

	go admin.StartAdmin()
	go startTls()
	go startDtls()
}

func Stop() {
	cron.Stop()                  // 停止定时任务
	AuthSessionManager.Stop()    // 停止认证会话管理器
	auth.GetLockManager().Stop() // 停止防暴力破解清理协程
	webvpn.GetManager().Stop()   // 停止 WebVPN 子系统
	dbdata.Stop()                // 停止数据库
	destroyVtap()                // 销毁虚拟网卡
	// 停止 FakeDNS 管理器
	if sessdata.GlobalFakeDNSManager != nil {
		sessdata.GlobalFakeDNSManager.Stop()
	}
	// 清理全局NAT规则
	if sessdata.GlobalFirewall != nil {
		sessdata.GlobalFirewall.CleanupGlobal()
	}
}
