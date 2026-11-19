package dbdata

import (
	"net/url"
	"strconv"

	"xorm.io/xorm"
)

// 访问审计的存储入口
type AccessAuditStore struct{ *table[AccessAudit] }

// 访问审计与管理日志的存储入口实例，由 SetDB 注入持久化实现
var Audits = &AccessAuditStore{}

func (st *AccessAuditStore) GetAuditSession(values url.Values) *xorm.Session {
	session := st.engine().Where("1=1")
	if v := values.Get("search[username]"); v != "" {
		fuzzy := "%" + v + "%"
		session.And("username IN (SELECT username FROM "+QuoteIdent("user")+" WHERE username LIKE ? OR nickname LIKE ?)", fuzzy, fuzzy)
	}
	if v := values.Get("search[group_name]"); v != "" {
		session.And("group_name = ?", v)
	}
	if v := values.Get("search[src]"); v != "" {
		session.And("src = ?", v)
	}
	if v := values.Get("search[dst]"); v != "" {
		session.And("dst = ?", v)
	}
	if v := values.Get("search[src_port]"); v != "" {
		session.And("src_port = ?", v)
	}
	if v := values.Get("search[dst_port]"); v != "" {
		session.And("dst_port = ?", v)
	}
	if v := values.Get("search[access_proto]"); v != "" {
		session.And("access_proto = ?", v)
	}
	if dates, ok := values["search[date][]"]; ok && len(dates) == 2 && dates[0] != "" {
		session.And("created_at BETWEEN ? AND ?", dates[0], dates[1])
	} else if d0 := values.Get("search[date][0]"); d0 != "" {
		d1 := values.Get("search[date][1]")
		session.And("created_at BETWEEN ? AND ?", d0, d1)
	}
	if v := values.Get("search[info]"); v != "" {
		session.And("info LIKE ?", "%"+v+"%")
	}
	sort, _ := strconv.Atoi(values.Get("search[sort]"))
	switch sort {
	case 0:
		// 默认倒序：新记录在前，无需翻到最后一页
		session.OrderBy("id desc")
	case 2:
		session.OrderBy("id asc")
	default:
		// sort==1 兼容旧前端：倒序
		session.OrderBy("id desc")
	}
	return session
}
func (st *AccessAuditStore) ClearAccessAudit(ts string) (int64, error) {
	affected, err := st.engine().Where("created_at < ?", ts).Delete(&AccessAudit{})
	return affected, err
}
