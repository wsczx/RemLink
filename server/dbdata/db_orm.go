package dbdata

import (
	"errors"
	"reflect"

	"xorm.io/xorm"
)

const PageSize = 10

var ErrNotFound = errors.New("ErrNotFound")

func Add(data any) error {
	_, err := xdb.InsertOne(data)
	return err
}

func AddBatch(data any) error {
	_, err := xdb.Insert(data)
	return err
}

func Update(fieldName string, value any, data any) error {
	_, err := xdb.Where(fieldName+"=?", value).Update(data)
	return err
}

func Del(data any) error {
	_, err := xdb.Delete(data)
	return err
}

func extract(data any, fieldName string) any {
	ref := reflect.ValueOf(data)
	r := &ref
	if r.Kind() == reflect.Pointer {
		e := r.Elem()
		r = &e
	}
	field := r.FieldByName(fieldName).Interface()
	return field
}

// 更新全部字段
func Set(data any) error {
	id := extract(data, "Id")
	_, err := xdb.ID(id).AllCols().Update(data)
	return err
}

func One(fieldName string, value any, data any) error {
	has, err := xdb.Where(fieldName+"=?", value).Get(data)
	if err != nil {
		return err
	}
	if !has {
		return ErrNotFound
	}

	return nil
}
func OneWhere(where string, data any, args ...any) error {
	has, err := xdb.Where(where, args...).Get(data)
	if err != nil {
		return err
	}
	if !has {
		return ErrNotFound
	}

	return nil
}

func CountAll(data any) int {
	n, _ := xdb.Count(data)
	return int(n)
}

func Find(data any, limit, page int) error {
	if limit <= 0 {
		// 默认按主键排序，确保结果一致性；limit<=0 视为查全部（无序分页参数兼容 -1 等）
		return xdb.OrderBy("id ASC").Find(data)
	}

	start := (page - 1) * limit
	// 按主键排序以确保分页结果一致性
	return xdb.OrderBy("id ASC").Limit(limit, start).Find(data)
}

func FindWhereCount(data any, where string, args ...any) int {
	n, _ := xdb.Where(where, args...).Count(data)
	return int(n)
}

func FindWhere(data any, limit int, page int, where string, args ...any) error {
	if limit == 0 {
		return xdb.Where(where, args...).OrderBy("id ASC").Find(data)
	}

	start := (page - 1) * limit
	return xdb.Where(where, args...).OrderBy("id ASC").Limit(limit, start).Find(data)
}

func FindAndCount(session *xorm.Session, data any, limit, page int) (int64, error) {
	if limit == 0 {
		return session.FindAndCount(data)
	}
	start := (page - 1) * limit
	totalCount, err := session.Limit(limit, start).FindAndCount(data)
	return totalCount, err
}

// 在不分页、不全量加载对象的前提下，对当前筛选条件做聚合统计
// 用 SQL 的 COUNT + CASE WHEN 在数据库侧完成统计，避免把全表用户载入内存
// 覆盖全部用户类型（wxwork/dingtalk/feishu）
type UserStats struct {
	Total    int
	Local    int
	Ldap     int
	External int
	Active   int
	Disable  int
}

func UserStatsWhere(where string, args ...any) (UserStats, error) {
	var s UserStats
	sql := `SELECT
		COUNT(*) AS total,
		SUM(CASE WHEN type = 'local' THEN 1 ELSE 0 END) AS local,
		SUM(CASE WHEN type = 'ldap' THEN 1 ELSE 0 END) AS ldap,
		SUM(CASE WHEN type IN ('external','wxwork','dingtalk','feishu') THEN 1 ELSE 0 END) AS external,
		SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END) AS active,
		SUM(CASE WHEN status != 1 THEN 1 ELSE 0 END) AS disable
	FROM user`
	if where != "" {
		sql += " WHERE " + where
	}
	if _, err := xdb.SQL(sql, args...).Get(&s); err != nil {
		return s, err
	}
	return s, nil
}
