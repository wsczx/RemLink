package cron

import (
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

const siLifeDay = 30

// 清理图表数据
func ClearStatsInfo() {
	_, timesUp := isClearTime()
	if !timesUp {
		return
	}
	ts := getTimeAgo(siLifeDay)
	for _, item := range dbdata.StatsInfoIns.Actions {
		affected, err := dbdata.StatsInfoIns.ClearStatsInfo(item, ts)
		base.Info("Cron ClearStatsInfo  "+item+": ", affected, err)
	}
}

// 判断是否到达清理时间
func isClearTime() (int, bool) {
	dataLog, err := dbdata.SettingGetAuditLog()
	if err != nil {
		base.Error("Cron SettingGetLog: ", err)
		return -1, false
	}
	currentTime := time.Now().Format("15:04")
	if dataLog.ClearTime != currentTime {
		return -1, false
	}
	return dataLog.LifeDay, true
}

// 根据保存天数计算清理日期
func getTimeAgo(days int) string {
	var timeS string
	ts := time.Now().AddDate(0, 0, -days)
	tsZero := time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, time.Local)
	timeS = tsZero.Format(dbdata.LayoutTimeFormat)
	return timeS
}
