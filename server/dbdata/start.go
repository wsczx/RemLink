package dbdata

import (
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/security"
	"github.com/wsczx/remlink/pkg/utils"
)

// 初始化加密密钥与数据库
func Start() error {
	security.SetDir(".")
	if err := security.LoadKey(); err != nil {
		return err
	}
	if err := initDb(); err != nil {
		return err
	}
	if err := initData(); err != nil {
		return err
	}
	// 读取持久化配置；首次启动时以当前默认配置写库
	sc := &SettingServerConfig{}
	if err := Settings.Get(sc); err != nil {
		if !utils.CheckErrNotFound(err) {
			return err
		}
		sc.Config = *base.GetCfg()
		base.CompleteConfig(&sc.Config)
		if err := Settings.Save(sc); err != nil {
			return err
		}
	}
	base.LoadPersistedConfig(sc.Config)
	WebVpn.LoadRevoke()
	Groups.SyncLdapUsers()
	Groups.SyncWXworkUsers()
	Groups.SyncFeishuUsers()
	return nil
}

func Stop() error {
	return GetDB().Engine().Close()
}
