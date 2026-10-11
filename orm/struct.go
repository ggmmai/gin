package orm

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unsafe"
)

// GetInto 把查询结果扫进结构体切片，不经过 map[string]any。
// 字段用 orm 标签对应结果列名；没有标签时用字段名。orm:"-" 跳过该字段。
// NULL 不能写入 int、string 这类非指针字段，会返回错误。
// 指针字段写成 nil。实现 sql.Scanner 的字段（如 sql.NullString）由该字段处理 NULL。
//
//	list, err := D("coin").Select("id,instId").Limit(10).GetInto[Coin]()
func (b *Builder) GetInto[T any]() ([]T, error) {
	meta, err := prepareStructQuery[T](b)
	if err != nil {
		return nil, err
	}
	rows, err := b.queryRows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStructRows[T](rows, b.selects, b.limitN, meta)
}

// FindInto 把第一行扫进结构体。没有行时返回 nil, nil。
// 查询固定 LIMIT 1，不改变原构造器上的 Limit。
//
//	row, err := D("coin").Where("id", 1).FindInto[Coin]()
func (b *Builder) FindInto[T any]() (*T, error) {
	if b == nil {
		return nil, fmt.Errorf("orm scan: nil builder")
	}
	meta, err := cachedStructMeta(reflect.TypeFor[T]())
	if err != nil {
		return nil, err
	}
	clone := b.singleRow()
	if clone.db == nil {
		return nil, fmt.Errorf("orm scan: nil database")
	}
	rows, err := clone.queryRows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStructOne[T](rows, clone.selects, meta)
}

func prepareStructQuery[T any](b *Builder) (*structMeta, error) {
	if b == nil {
		return nil, fmt.Errorf("orm scan: nil builder")
	}
	meta, err := cachedStructMeta(reflect.TypeFor[T]())
	if err != nil {
		return nil, err
	}
	if b.db == nil {
		return nil, fmt.Errorf("orm scan: nil database")
	}
	return meta, nil
}

type fieldPlan struct {
	index   []int
	offset  uintptr
	typ     reflect.Type
	scanner bool
}

type structMeta struct {
	fields map[string]fieldPlan
}

var structMetas sync.Map

func cachedStructMeta(t reflect.Type) (*structMeta, error) {
	if v, ok := structMetas.Load(t); ok {
		return v.(*structMeta), nil
	}
	meta, err := buildStructMeta(t)
	if err != nil {
		return nil, err
	}
	actual, _ := structMetas.LoadOrStore(t, meta)
	return actual.(*structMeta), nil
}

func buildStructMeta(t reflect.Type) (*structMeta, error) {
	if t.Kind() == reflect.Pointer {
		return nil, fmt.Errorf("orm scan: %s is a pointer, use the struct type", t)
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("orm scan: %s is not a struct", t)
	}
	fields := make(map[string]fieldPlan)
	walkStructFields(t, nil, 0, fields)
	if len(fields) == 0 {
		return nil, fmt.Errorf("orm scan: %s has no scannable fields", t)
	}
	return &structMeta{fields: fields}, nil
}

var scannerType = reflect.TypeFor[sql.Scanner]()

func walkStructFields(t reflect.Type, prefix []int, base uintptr, fields map[string]fieldPlan) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("orm")
		if tag == "-" {
			continue
		}
		offset := base + f.Offset
		index := append(append([]int{}, prefix...), i)
		if f.Anonymous && tag == "" && f.Type.Kind() == reflect.Struct {
			walkStructFields(f.Type, index, offset, fields)
			continue
		}
		if f.PkgPath != "" {
			continue
		}
		name := tag
		if comma := strings.IndexByte(name, ','); comma >= 0 {
			name = name[:comma]
		}
		name = strings.TrimSpace(name)
		if name == "" {
			name = f.Name
		}
		if _, exists := fields[name]; exists {
			continue
		}
		fields[name] = fieldPlan{
			index:   index,
			offset:  offset,
			typ:     f.Type,
			scanner: reflect.PointerTo(f.Type).Implements(scannerType),
		}
	}
}

// colBind 是结果列到字段的对应。普通字段和 sql.Scanner 字段都直接扫进字段地址。
type colBind struct {
	matched bool
	offset  uintptr
	typ     reflect.Type
}

func newColBinds(layout columnLayout, meta *structMeta) ([]colBind, []any, []any) {
	binds := make([]colBind, len(layout.keys))
	ptrs := make([]any, len(layout.keys))
	holes := make([]any, len(layout.keys))
	for i, name := range layout.keys {
		if plan, ok := meta.fields[name]; ok {
			binds[i] = colBind{matched: true, offset: plan.offset, typ: plan.typ}
		}
		ptrs[i] = &holes[i]
	}
	return binds, ptrs, holes
}

func bindStructFields(base unsafe.Pointer, binds []colBind, ptrs []any, holes []any) {
	for i := range binds {
		if !binds[i].matched {
			ptrs[i] = &holes[i]
			continue
		}
		ptrs[i] = reflect.NewAt(binds[i].typ, unsafe.Add(base, binds[i].offset)).Interface()
	}
}

func scanStructRows[T any](rows *sql.Rows, selects []selectColumn, limit int, meta *structMeta) ([]T, error) {
	layout, err := readColumnLayout(rows, selects)
	if err != nil {
		return nil, err
	}
	binds, ptrs, holes := newColBinds(layout, meta)
	result := make([]T, 0, rowCapacity(limit))
	for rows.Next() {
		var row T
		bindStructFields(unsafe.Pointer(&row), binds, ptrs, holes)
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func scanStructOne[T any](rows *sql.Rows, selects []selectColumn, meta *structMeta) (*T, error) {
	layout, err := readColumnLayout(rows, selects)
	if err != nil {
		return nil, err
	}
	binds, ptrs, holes := newColBinds(layout, meta)
	if !rows.Next() {
		return nil, rows.Err()
	}
	var row T
	bindStructFields(unsafe.Pointer(&row), binds, ptrs, holes)
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &row, nil
}
