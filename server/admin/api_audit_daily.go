package admin

import (
	"net/http"
	"net/url"

	"github.com/wsczx/remlink/dbdata"
)

// 按天统计接口公共参数（GET query / form）：
//   - end：窗口最后一天 YYYY-MM-DD（默认今天，服务器本地时区）
//   - days：天数（默认 14，上限 92）
//   - 其余筛选参数与对应列表接口一致；列表的日期参数（sdate/edate、search[date]）被忽略
//
// 返回 {days:[{date:'YYYY-MM-DD',count}]}，按日期升序，无数据的日期 count=0

func respDaily(w http.ResponseWriter, r *http.Request, fn func(url.Values) ([]dbdata.DayCount, error)) {
	_ = r.ParseForm()
	days, err := fn(r.Form)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	RespSucess(w, map[string]any{"days": days})
}

// 用户行为日志按天计数（筛选参数同 act_log_list：username/status/os/client）
func UserActLogDaily(w http.ResponseWriter, r *http.Request) {
	respDaily(w, r, dbdata.UserActLogDailyCounts)
}

// 访问审计按天计数（筛选参数同 audit/list：search[username]/search[group_name]/search[src]/search[dst]/search[src_port]/search[dst_port]/search[access_proto]/search[info]）
func SetAuditDaily(w http.ResponseWriter, r *http.Request) {
	respDaily(w, r, dbdata.AuditDailyCounts)
}

// 管理操作日志按天计数（筛选参数同 admin_op_log_list：op_type/keyword）
func AdminOpLogDaily(w http.ResponseWriter, r *http.Request) {
	respDaily(w, r, dbdata.AdminOpLogDailyCounts)
}
