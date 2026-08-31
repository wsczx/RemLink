package cron

import (
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

var scheduler gocron.Scheduler

// 初始化并启动定时任务
func Start() error {
	s, err := gocron.NewScheduler(gocron.WithLocation(time.Local))
	if err != nil {
		return err
	}
	scheduler = s

	if _, err := s.NewJob(
		gocron.CronJob("0 * * * *", false),
		gocron.NewTask(ClearAudit),
	); err != nil {
		base.Error("注册定时任务失败 (ClearAudit):", err)
	}
	if _, err := s.NewJob(
		gocron.CronJob("0 * * * *", false),
		gocron.NewTask(ClearStatsInfo),
	); err != nil {
		base.Error("注册定时任务失败 (ClearStatsInfo):", err)
	}
	if _, err := s.NewJob(
		gocron.CronJob("0 * * * *", false),
		gocron.NewTask(ClearUserActLog),
	); err != nil {
		base.Error("注册定时任务失败 (ClearUserActLog):", err)
	}
	if _, err := s.NewJob(
		gocron.CronJob("0 * * * *", false),
		gocron.NewTask(ClearAdminOpLog),
	); err != nil {
		base.Error("注册定时任务失败 (ClearAdminOpLog):", err)
	}

	if _, err := s.NewJob(
		gocron.CronJob("0 0 * * *", false),
		gocron.NewTask(dbdata.CheckAndRenewCert),
	); err != nil {
		base.Error("注册定时任务失败 (CheckAndRenewCert):", err)
	}
	if _, err := s.NewJob(
		gocron.CronJob("0 0 * * *", false),
		gocron.NewTask(dbdata.SyncLdapUsers),
	); err != nil {
		base.Error("注册定时任务失败 (SyncLdapUsers):", err)
	}
	if _, err := s.NewJob(
		gocron.CronJob("0 0 * * *", false),
		gocron.NewTask(dbdata.SyncWXworkUsers),
	); err != nil {
		base.Error("注册定时任务失败 (SyncWXworkUsers):", err)
	}
	if _, err := s.NewJob(
		gocron.CronJob("0 0 * * *", false),
		gocron.NewTask(dbdata.SyncFeishuUsers),
	); err != nil {
		base.Error("注册定时任务失败 (SyncFeishuUsers):", err)
	}
	if _, err := s.NewJob(
		gocron.CronJob("0 0 * * *", false),
		gocron.NewTask(dbdata.SyncDingtalkUsers),
	); err != nil {
		base.Error("注册定时任务失败 (SyncDingtalkUsers):", err)
	}

	s.Start()
	return nil
}

// 关闭定时任务调度器
func Stop() {
	if scheduler != nil {
		if err := scheduler.Shutdown(); err != nil {
			base.Error("关闭定时任务调度器失败:", err)
		}
	}
}
