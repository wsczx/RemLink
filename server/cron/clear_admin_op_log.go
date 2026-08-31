package cron

import (
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 清除管理员操作日志
func ClearAdminOpLog() {
	lifeDay, timesUp := isClearTime()
	if !timesUp {
		return
	}
	// 当审计日志永久保存时，则退出
	if lifeDay <= 0 {
		return
	}
	affected, err := dbdata.ClearAdminOpLog(getTimeAgo(lifeDay))
	base.Info("Cron ClearAdminOpLog: ", affected, err)
}
