package cron

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

type Cron struct {
	scheduler gocron.Scheduler
}

func New() (*Cron, error) {
	s, err := gocron.NewScheduler(gocron.WithLocation(time.Local))
	if err != nil {
		return nil, err
	}
	return &Cron{scheduler: s}, nil
}

// 注册全部定时任务并启动调度器
func (c *Cron) Start() error {
	jobs := []struct {
		name, spec string
		task       any
	}{
		{"ClearAudit", "0 * * * *", ClearAudit},
		{"ClearStatsInfo", "0 * * * *", ClearStatsInfo},
		{"ClearUserActLog", "0 * * * *", ClearUserActLog},
		{"ClearAdminOpLog", "0 * * * *", ClearAdminOpLog},
		{"CheckAndRenewCert", "0 0 * * *", dbdata.Certs.CheckAndRenewCert},
		{"SyncLdapUsers", "0 0 * * *", dbdata.Groups.SyncLdapUsers},
		{"SyncWXworkUsers", "0 0 * * *", dbdata.Groups.SyncWXworkUsers},
		{"SyncFeishuUsers", "0 0 * * *", dbdata.Groups.SyncFeishuUsers},
		{"SyncDingtalkUsers", "0 0 * * *", dbdata.Groups.SyncDingtalkUsers},
	}
	for _, j := range jobs {
		if _, err := c.scheduler.NewJob(gocron.CronJob(j.spec, false), gocron.NewTask(j.task)); err != nil {
			return fmt.Errorf("注册定时任务 %s 失败: %w", j.name, err)
		}
	}

	c.scheduler.Start()
	return nil
}

// 关闭定时任务调度器
func (c *Cron) Stop() {
	if err := c.scheduler.Shutdown(); err != nil {
		base.Error("关闭定时任务调度器失败:", err)
	}
}
