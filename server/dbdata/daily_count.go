package dbdata

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"xorm.io/xorm"
)

// 单日计数
type DayCount struct {
	Date  string `json:"date"` // YYYY-MM-DD（服务器本地时区）
	Count int64  `json:"count"`
}

const (
	DailyCountDefaultDays = 14
	DailyCountMaxDays     = 92
	dailyTimeLayout       = "2006-01-02 15:04:05"
)

// 解析按天统计窗口：end=YYYY-MM-DD（默认今天，本地时区）、days（默认 14，上限 92）
// 返回窗口第一天 00:00 与天数
func ParseDailyWindow(values url.Values) (time.Time, int) {
	days, _ := strconv.Atoi(values.Get("days"))
	if days <= 0 {
		days = DailyCountDefaultDays
	}
	if days > DailyCountMaxDays {
		days = DailyCountMaxDays
	}
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if v := strings.TrimSpace(values.Get("end")); v != "" {
		if len(v) > 10 {
			v = v[:10]
		}
		if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
			end = t
		}
	}
	return end.AddDate(0, 0, -(days - 1)), days
}

// 按天统计 session 筛选结果的条数。
// 只用一条 SQL：WHERE 限定窗口区间，SELECT 中每天一个 SUM(CASE WHEN created_at 落在当天 THEN 1 ELSE 0 END)。
// 不依赖任何数据库特定的日期截断函数，sqlite/mysql/postgres/mssql 通用；时间边界与列表接口一致，按本地时区字符串比较。
// session 不能带 ORDER BY（聚合查询在 postgres/mssql 下会报错），调用方应传入仅含筛选条件的 session。
func DailyCounts(session *xorm.Session, bean any, start time.Time, days int) ([]DayCount, error) {
	if days <= 0 {
		return []DayCount{}, nil
	}
	cols := make([]string, days)
	result := make([]DayCount, days)
	for i := 0; i < days; i++ {
		d0 := start.AddDate(0, 0, i)
		d1 := start.AddDate(0, 0, i+1)
		// 边界值由服务端格式化生成（非用户输入），直接内联以兼容 Select 不支持参数绑定
		cols[i] = fmt.Sprintf("SUM(CASE WHEN created_at >= '%s' AND created_at < '%s' THEN 1 ELSE 0 END) AS d%d",
			d0.Format(dailyTimeLayout), d1.Format(dailyTimeLayout), i)
		result[i] = DayCount{Date: d0.Format("2006-01-02")}
	}
	end := start.AddDate(0, 0, days)
	session.And("created_at >= ? AND created_at < ?", start.Format(dailyTimeLayout), end.Format(dailyTimeLayout))

	rows, err := session.Table(bean).Select(strings.Join(cols, ", ")).QueryString()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return result, nil
	}
	row := rows[0]
	for i := range result {
		v := row["d"+strconv.Itoa(i)]
		if v == "" {
			v = row["D"+strconv.Itoa(i)]
		}
		if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			result[i].Count = int64(n)
		}
	}
	return result, nil
}

// 去掉列表接口中的日期筛选参数（按天统计的窗口由 end/days 决定）
func stripDateParams(values url.Values, keys ...string) url.Values {
	v := url.Values{}
	for k, vs := range values {
		v[k] = vs
	}
	for _, k := range keys {
		delete(v, k)
	}
	return v
}

// 用户行为日志按天统计（筛选参数同 act_log_list，忽略 sdate/edate）
func UserActLogDailyCounts(values url.Values) ([]DayCount, error) {
	start, days := ParseDailyWindow(values)
	session := applyUserActLogFilter(xdb.Where("1=1"), stripDateParams(values, "sdate", "edate"))
	return DailyCounts(session, &UserActLog{}, start, days)
}

// 访问审计按天统计（筛选参数同 audit/list 的 search[...]，忽略 search[date]）
func AuditDailyCounts(values url.Values) ([]DayCount, error) {
	start, days := ParseDailyWindow(values)
	session := applyAuditFilter(xdb.Where("1=1"),
		stripDateParams(values, "search[date][]", "search[date][0]", "search[date][1]"))
	return DailyCounts(session, &AccessAudit{}, start, days)
}

// 管理操作日志按天统计（筛选参数同 admin_op_log_list，忽略 sdate/edate）
func AdminOpLogDailyCounts(values url.Values) ([]DayCount, error) {
	start, days := ParseDailyWindow(values)
	session := applyAdminOpLogFilter(xdb.Where("1=1"), stripDateParams(values, "sdate", "edate"))
	return DailyCounts(session, &AdminOpLog{}, start, days)
}
