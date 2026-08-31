package dbdata

import (
	"net/url"
	"strconv"

	"xorm.io/xorm"
)

func GetAuditSession(values url.Values) *xorm.Session {
	session := xdb.Where("1=1")
	if v := values.Get("search[username]"); v != "" {
		fuzzy := "%" + v + "%"
		session.And("username IN (SELECT username FROM user WHERE username LIKE ? OR nickname LIKE ?)", fuzzy, fuzzy)
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
	if v := values.Get("search[date][]"); v != "" {
		// 日期区间可能以 "date[0]"/"date[1]" 形式出现，下面单独处理
		_ = v
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

func ClearAccessAudit(ts string) (int64, error) {
	affected, err := xdb.Where("created_at < ?", ts).Delete(&AccessAudit{})
	return affected, err
}
