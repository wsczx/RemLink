package sessdata

import "github.com/wsczx/remlink/base"

func Start() {
	if err := initIpPool(); err != nil {
		base.Fatal("IP池初始化失败:", err)
	}
	checkSession()
	saveStatsInfo()
}
