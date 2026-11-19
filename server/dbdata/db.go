package dbdata

import (
	"reflect"

	"github.com/wsczx/remlink/pkg/utils"
	"xorm.io/xorm"
)

type DB struct {
	engine *xorm.Engine
}

func NewDB(engine *xorm.Engine) *DB { return &DB{engine: engine} }

// 返回底层 xorm 引擎，供封装未覆盖的复杂查询/事务/原生 SQL 使用
func (s *DB) Engine() *xorm.Engine { return s.engine }

func SetShowSQL(on bool) { GetDB().Engine().ShowSQL(on) }

func (s *DB) Insert(v any) error {
	_, err := s.engine.InsertOne(v)
	return err
}

func (s *DB) InsertBatch(v any) error {
	_, err := s.engine.Insert(v)
	return err
}

func (s *DB) Update(field string, value any, v any) error {
	_, err := s.engine.Where(field+"=?", value).Update(v)
	return err
}

func (s *DB) Delete(v any) error {
	_, err := s.engine.Delete(v)
	return err
}

func (s *DB) Save(v any) error {
	_, err := s.engine.ID(extract(v, "Id")).AllCols().Update(v)
	return err
}

func (s *DB) Get(field string, value any, dest any) error {
	return getOne(s.engine, field+"=?", dest, value)
}

func (s *DB) GetWhere(where string, dest any, args ...any) error {
	return getOne(s.engine, where, dest, args...)
}

func (s *DB) Find(dest any, limit, page int) error {
	if limit <= 0 {
		return s.engine.OrderBy("id ASC").Find(dest)
	}
	return s.engine.OrderBy("id ASC").Limit(limit, (page-1)*limit).Find(dest)
}

func (s *DB) FindWhere(dest any, limit, page int, where string, args ...any) error {
	if limit == 0 {
		return s.engine.Where(where, args...).OrderBy("id ASC").Find(dest)
	}
	return s.engine.Where(where, args...).OrderBy("id ASC").Limit(limit, (page-1)*limit).Find(dest)
}

func (s *DB) Count(v any) int {
	n, _ := s.engine.Count(v)
	return int(n)
}

func (s *DB) CountWhere(v any, where string, args ...any) int {
	n, _ := s.engine.Where(where, args...).Count(v)
	return int(n)
}

// 按条件查询单条记录
func getOne(engine *xorm.Engine, where string, dest any, args ...any) error {
	has, err := engine.Where(where, args...).Get(dest)
	if err != nil {
		return err
	}
	if !has {
		return utils.ErrNotFound
	}
	return nil
}

type table[T any] struct{}

func (t *table[T]) engine() *xorm.Engine {
	if s := GetDB(); s != nil {
		return s.engine
	}
	return nil
}

// 按字段值加载单条记录
func (t *table[T]) Get(field string, value any) (*T, error) {
	var v T
	if err := GetDB().Get(field, value, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// 按条件加载单条记录
func (t *table[T]) GetWhere(where string, args ...any) (*T, error) {
	var v T
	if err := GetDB().GetWhere(where, &v, args...); err != nil {
		return nil, err
	}
	return &v, nil
}

// 查询列表；limit<=0 表示不分页
func (t *table[T]) Find(limit, page int) ([]T, error) {
	var vs []T
	if err := GetDB().Find(&vs, limit, page); err != nil {
		return nil, err
	}
	return vs, nil
}

// 按条件分页查询列表
func (t *table[T]) FindWhere(limit, page int, where string, args ...any) ([]T, error) {
	var vs []T
	if err := GetDB().FindWhere(&vs, limit, page, where, args...); err != nil {
		return nil, err
	}
	return vs, nil
}

// 返回模型对应的记录总数
func (t *table[T]) Count() int { return GetDB().Count(new(T)) }

// 返回符合条件的记录数
func (t *table[T]) CountWhere(where string, args ...any) int {
	return GetDB().CountWhere(new(T), where, args...)
}

// 插入单条记录
func (t *table[T]) Insert(v *T) error { return GetDB().Insert(v) }

// 批量插入记录
func (t *table[T]) InsertBatch(v any) error { return GetDB().InsertBatch(v) }

// 更新记录的全部字段
func (t *table[T]) Save(v *T) error { return GetDB().Save(v) }

// 删除记录
func (t *table[T]) Delete(v *T) error { return GetDB().Delete(v) }

const PageSize = 10

func QuoteIdent(name string) string {
	return GetDB().Engine().Quote(name)
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

// 在指定会话内分页查询并返回总数（事务内使用）
func FindAndCount(session *xorm.Session, data any, limit, page int) (int64, error) {
	if limit == 0 {
		return session.FindAndCount(data)
	}
	start := (page - 1) * limit
	totalCount, err := session.Limit(limit, start).FindAndCount(data)
	return totalCount, err
}
