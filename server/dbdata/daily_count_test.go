package dbdata

import (
	"net/url"
	"testing"
	"time"
)

func TestDailyCounts(t *testing.T) {
	preIpData(t)
	defer closeIpdata()

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	at := func(daysAgo int, h int) time.Time { return today.AddDate(0, 0, -daysAgo).Add(time.Duration(h) * time.Hour) }

	// 用户行为日志：今天 2 条（其中 1 条 status 不同），3 天前 1 条，20 天前 1 条（窗口外）
	acts := []UserActLog{
		{Username: "u1", Status: UserAuthSuccess, CreatedAt: at(0, 1)},
		{Username: "u1", Status: UserAuthFail, CreatedAt: at(0, 23)},
		{Username: "u1", Status: UserAuthSuccess, CreatedAt: at(3, 12)},
		{Username: "u1", Status: UserAuthSuccess, CreatedAt: at(20, 12)},
	}
	for i := range acts {
		if _, err := xdb.NoAutoTime().Insert(&acts[i]); err != nil {
			t.Fatal(err)
		}
	}

	v := url.Values{}
	v.Set("sdate", "2000-01-01") // 列表日期参数应被忽略
	v.Set("edate", "2000-01-01")
	days, err := UserActLogDailyCounts(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != DailyCountDefaultDays {
		t.Fatalf("len=%d", len(days))
	}
	if days[len(days)-1].Date != today.Format("2006-01-02") || days[len(days)-1].Count != 2 {
		t.Fatalf("today: %+v", days[len(days)-1])
	}
	if days[len(days)-4].Count != 1 {
		t.Fatalf("3 days ago: %+v", days[len(days)-4])
	}
	var total int64
	for _, d := range days {
		total += d.Count
	}
	if total != 3 {
		t.Fatalf("total=%d want 3", total)
	}

	// 复用列表筛选：status（列表参数为 值+1）
	v = url.Values{}
	v.Set("status", "1") // => status = 0 = UserAuthSuccess? 仅校验与列表接口一致
	list, _ := UserActLogIns.GetSession(v).And("created_at >= ?", today.AddDate(0, 0, -13).Format(dailyTimeLayout)).Count(&UserActLog{})
	days, err = UserActLogDailyCounts(v)
	if err != nil {
		t.Fatal(err)
	}
	total = 0
	for _, d := range days {
		total += d.Count
	}
	if total != list {
		t.Fatalf("filtered total=%d list=%d", total, list)
	}

	// end/days 参数
	v = url.Values{}
	v.Set("end", today.AddDate(0, 0, -3).Format("2006-01-02"))
	v.Set("days", "1")
	days, err = UserActLogDailyCounts(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || days[0].Count != 1 {
		t.Fatalf("end window: %+v", days)
	}

	// 访问审计
	audits := []AccessAudit{
		{Username: "u1", Src: "10.0.0.1", Dst: "10.1.1.1", DstPort: 443, CreatedAt: at(1, 5)},
		{Username: "u1", Src: "10.0.0.1", Dst: "10.1.1.2", DstPort: 80, CreatedAt: at(1, 6)},
	}
	for i := range audits {
		if err := Add(&audits[i]); err != nil {
			t.Fatal(err)
		}
	}
	v = url.Values{}
	v.Set("search[dst_port]", "443")
	days, err = AuditDailyCounts(v)
	if err != nil {
		t.Fatal(err)
	}
	if days[len(days)-2].Count != 1 {
		t.Fatalf("audit yesterday: %+v", days[len(days)-2])
	}

	// 管理操作日志
	op := AdminOpLog{AdminUser: "admin", OpType: "用户管理", OpTarget: "u1", CreatedAt: at(0, 2)}
	if _, err := xdb.NoAutoTime().Insert(&op); err != nil {
		t.Fatal(err)
	}
	v = url.Values{}
	v.Set("op_type", "用户管理")
	days, err = AdminOpLogDailyCounts(v)
	if err != nil {
		t.Fatal(err)
	}
	if days[len(days)-1].Count != 1 {
		t.Fatalf("op today: %+v", days[len(days)-1])
	}
	v.Set("op_type", "不存在")
	days, _ = AdminOpLogDailyCounts(v)
	for _, d := range days {
		if d.Count != 0 {
			t.Fatalf("unexpected %+v", d)
		}
	}
}
