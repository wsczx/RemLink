package dbdata

import (
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/security"
)

func Start() {
	security.SetDir(".")
	if err := security.LoadKey(); err != nil {
		base.Fatal(err)
	}

	initDb()
	initData()
	if err := SettingLoadServerConfig(); err != nil {
		base.Fatal(err)
	}

	LoadWebVpnRevoke()

	SyncLdapUsers()
	SyncWXworkUsers()
	SyncFeishuUsers()
}

func Stop() error {
	return xdb.Close()
}
