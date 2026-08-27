package dbdata

import (
	"net/url"
	"strings"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"xorm.io/xorm"
)

var (
	// 管理员操作日志异步写入池
	adminOpLogPool = utils.NewWorkerPool(1, 100)
)

// 异步写入管理员操作日志（安全审计）
// opType: 操作类型, opTarget: 操作目标, detail: 详情, clientIP: 操作者IP
func AdminLog(opType, opTarget, detail, clientIP string) {
	adminUser := substr(base.GetCfg().AdminUser, 0, 60)
	// 截断字段防止超长
	opType = substr(opType, 0, 60)
	opTarget = substr(opTarget, 0, 255)
	detail = substr(detail, 0, 512)
	clientIP = substr(clientIP, 0, 42)

	logEntry := AdminOpLog{
		AdminUser: adminUser,
		OpType:    opType,
		OpTarget:  opTarget,
		Detail:    detail,
		ClientIp:  clientIP,
	}

	adminOpLogPool.JobQueue <- func() {
		if err := Add(&logEntry); err != nil {
			base.Error("写入管理员操作日志失败:", err)
		}
	}
}

// 清除管理员操作日志
func ClearAdminOpLog(ts string) (int64, error) {
	affected, err := xdb.Where("created_at < ?", ts).Delete(&AdminOpLog{})
	return affected, err
}

// 操作类型中文名
var AdminOpLogTypes = []string{
	"用户管理",
	"用户组管理",
	"策略管理",
	"证书管理",
	"系统设置",
	"安全设置",
	"Provider管理",
	"IP映射管理",
	"WebVPN应用管理",
}

// 构建管理员操作日志查询 session
func GetAdminOpLogSession(values url.Values) *xorm.Session {
	session := xdb.Where("1=1")
	if v := values.Get("op_type"); v != "" {
		session.And("op_type = ?", v)
	}
	if v := values.Get("sdate"); v != "" {
		session.And("created_at >= ?", v+" 00:00:00")
	}
	if v := values.Get("edate"); v != "" {
		session.And("created_at <= ?", v+" 23:59:59")
	}
	if v := values.Get("keyword"); v != "" {
		kw := "%" + strings.TrimSpace(v) + "%"
		session.And("(op_target like ? or detail like ?)", kw, kw)
	}

	session.OrderBy("id desc")
	return session
}
