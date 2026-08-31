package cron

import (
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 清除访问审计日志
func ClearAudit() {
	lifeDay, timesUp := isClearTime()
	if !timesUp {
		return
	}
	// 当审计日志永久保存，则退出
	if lifeDay <= 0 {
		return
	}
	affected, err := dbdata.ClearAccessAudit(getTimeAgo(lifeDay))
	base.Info("Cron ClearAudit: ", affected, err)
}
