// RemLink 是一个企业级远程办公vpn软件，可以支持多人同时在线使用。

//go:build !windows

package main

import (
	"embed"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/wsczx/remlink/admin"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/handler"
)

//go:embed ui
var uiData embed.FS

// 程序版本
var (
	appVer    string
	commitId  string
	buildDate string
)

func main() {
	admin.UiData = uiData
	base.APP_VER = appVer
	base.CommitId = commitId
	base.BuildDate = buildDate

	base.Start()

	if base.DisableAdminOtpFlag {
		handleDisableAdminOtp()
	}
	if base.ResetAdminPassFlag {
		handleResetAdminPassword()
	}
	if base.EnableFakeDNSFlag {
		handleEnableFakeDNS()
	}

	handler.Start()

	signalWatch()
}

func signalWatch() {
	base.Info("Server pid: ", os.Getpid())

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGALRM, syscall.SIGUSR2)
	for {
		sig := <-sigs
		base.Info("Get signal:", sig)
		switch sig {
		case syscall.SIGUSR2:
			// reload
			base.Info("Reload")
		default:
			// stop
			base.Info("Stop")
			handler.Stop()
			return
		}
	}
}

func handleDisableAdminOtp() {
	dbdata.Start()
	defer dbdata.Stop()

	cfg := base.GetCfg()
	if cfg.AdminOtp == "" {
		fmt.Fprintln(os.Stderr, "管理员两步验证当前未启用，无需禁用")
		os.Exit(0)
	}

	base.DisableAdminOtp()
	if err := dbdata.SettingSaveServerConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "保存配置到数据库失败: %v\n", err)
		os.Exit(1)
	}
	printBanner("管理员两步验证已强制禁用",
		"⚠ OTP 密钥已清除，请重启 remlink 服务",
		"⚠ 建议重新登录管理后台后立即重新绑定 OTP")
}

func handleResetAdminPassword() {
	dbdata.Start()
	defer dbdata.Stop()

	newPass := base.ResetAdminPassword()
	if newPass == "" {
		fmt.Fprintln(os.Stderr, "重置管理员密码失败")
		os.Exit(1)
	}
	if err := dbdata.SettingSaveServerConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "保存密码到数据库失败: %v\n", err)
		os.Exit(1)
	}

	cfg := base.GetCfg()
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "========================================\n")
	fmt.Fprintf(os.Stderr, "  管理员密码已重置\n")
	fmt.Fprintf(os.Stderr, "========================================\n")
	fmt.Fprintf(os.Stderr, "  用户名:      %s\n", cfg.AdminUser)
	fmt.Fprintf(os.Stderr, "  新密码:      %s\n", newPass)
	fmt.Fprintf(os.Stderr, "  管理后台:    https://<服务器IP>%s\n", cfg.AdminAddr)
	fmt.Fprintf(os.Stderr, "========================================\n")
	fmt.Fprintf(os.Stderr, "  ⚠ 请重启 remlink 服务使新密码生效后，立即登录修改密码\n")
	fmt.Fprintf(os.Stderr, "========================================\n\n")
	os.Exit(0)
}

func handleEnableFakeDNS() {
	dbdata.Start()
	defer dbdata.Stop()

	base.EnableFakeDNS()
	if err := dbdata.SettingSaveServerConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "保存配置到数据库失败: %v\n", err)
		os.Exit(1)
	}
	printBanner("FakeDNS 可见性已开启",
		"当前状态:    显示",
		"⚠ 请重启 remlink 服务使新配置生效")
}

func printBanner(head string, lines ...string) {
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "========================================\n")
	fmt.Fprintf(os.Stderr, "  %s\n", head)
	fmt.Fprintf(os.Stderr, "========================================\n")
	for _, l := range lines {
		fmt.Fprintf(os.Stderr, "  %s\n", l)
	}
	fmt.Fprintf(os.Stderr, "========================================\n\n")
	os.Exit(0)
}
