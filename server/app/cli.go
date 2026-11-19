package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

func (a *App) RunMaintenance() (handled bool, message string, err error) {
	if !base.DisableAdminOtpFlag && !base.ResetAdminPassFlag && !base.EnableFakeDNSFlag {
		return false, "", nil
	}
	if err := a.OpenStore(); err != nil {
		return true, "", fmt.Errorf("数据库初始化失败: %w", err)
	}
	defer dbdata.Stop()

	switch {
	case base.DisableAdminOtpFlag:
		message, err = a.disableAdminOtp()
	case base.ResetAdminPassFlag:
		message, err = a.resetAdminPassword()
	default:
		message, err = a.enableFakeDNS()
	}
	return true, message, err
}

func (a *App) disableAdminOtp() (string, error) {
	if base.GetCfg().AdminOtp == "" {
		return "管理员两步验证当前未启用，无需禁用\n", nil
	}
	base.DisableAdminOtp()
	if err := dbdata.Settings.Save(&dbdata.SettingServerConfig{Config: *base.GetCfg()}); err != nil {
		return "", fmt.Errorf("保存配置到数据库失败: %w", err)
	}
	return banner("管理员两步验证已强制禁用",
		"⚠ OTP 密钥已清除，请重启 remlink 服务",
		"⚠ 建议重新登录管理后台后立即重新绑定 OTP"), nil
}

func (a *App) resetAdminPassword() (string, error) {
	newPass := base.ResetAdminPassword()
	if newPass == "" {
		return "", errors.New("重置管理员密码失败")
	}
	if err := dbdata.Settings.Save(&dbdata.SettingServerConfig{Config: *base.GetCfg()}); err != nil {
		return "", fmt.Errorf("保存密码到数据库失败: %w", err)
	}
	cfg := base.GetCfg()
	var b strings.Builder
	b.WriteString("\n========================================\n")
	b.WriteString("  管理员密码已重置\n")
	b.WriteString("========================================\n")
	fmt.Fprintf(&b, "  用户名:      %s\n", cfg.AdminUser)
	fmt.Fprintf(&b, "  新密码:      %s\n", newPass)
	fmt.Fprintf(&b, "  管理后台:    https://<服务器IP>%s\n", cfg.AdminAddr)
	b.WriteString("========================================\n")
	b.WriteString("  ⚠ 请重启 remlink 服务使新密码生效后，立即登录修改密码\n")
	b.WriteString("========================================\n\n")
	return b.String(), nil
}

func (a *App) enableFakeDNS() (string, error) {
	base.EnableFakeDNS()
	if err := dbdata.Settings.Save(&dbdata.SettingServerConfig{Config: *base.GetCfg()}); err != nil {
		return "", fmt.Errorf("保存配置到数据库失败: %w", err)
	}
	return banner("FakeDNS 可见性已开启",
		"当前状态:    显示",
		"⚠ 请重启 remlink 服务使新配置生效"), nil
}

func banner(head string, lines ...string) string {
	var b strings.Builder
	b.WriteString("\n========================================\n")
	fmt.Fprintf(&b, "  %s\n", head)
	b.WriteString("========================================\n")
	for _, l := range lines {
		fmt.Fprintf(&b, "  %s\n", l)
	}
	b.WriteString("========================================\n\n")
	return b.String()
}
