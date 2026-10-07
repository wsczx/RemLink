package dbdata

import (
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/geoip"
	"github.com/wsczx/remlink/pkg/security"
)

// GeoIP 数据库默认路径（与 configMetas 中 geoip_db 默认值一致，与 db_source 同在 conf 数据目录）
const DefaultGeoipDb = "./conf/geoip.mmdb"

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
	CloseDanglingWebVpnSessions()

	// GeoIP 国家库：文件不存在时静默关闭；路径取自配置，支持运行时修改与文件热替换
	geoip.Init(func() string {
		// 老版本持久化配置中无该字段时回退默认路径
		if p := base.GetCfg().GeoipDb; p != "" {
			return p
		}
		return DefaultGeoipDb
	})
	if st := geoip.GetStatus(); st.Enabled {
		base.Info("GeoIP 数据库已加载:", st.Path, st.DatabaseType, st.BuildTime.Format("2006-01-02"))
	} else if st.Error != "" {
		base.Warn("GeoIP 数据库加载失败:", st.Path, st.Error)
	}

	SyncLdapUsers()
	SyncWXworkUsers()
	SyncFeishuUsers()
}

func Stop() error {
	return xdb.Close()
}
