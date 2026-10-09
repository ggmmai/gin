package orm

import (
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Find 获取一条数据（类似 Laravel first / find）
// 返回 map[string]any；无数据时返回 nil, nil
// 查询固定 LIMIT 1。已经写过的 Limit 不会带到这次查询里，原构造器上的 Limit 保持不变。
func (b *Builder) Find() (map[string]any, error) {
	clone := b.singleRow()
	rows, err := clone.queryRows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOne(rows, clone.selects)
}

// First Find 的别名
func (b *Builder) First() (map[string]any, error) {
	return b.Find()
}

// singleRow 复制一份构造器，并把条数改成 1
func (b *Builder) singleRow() *Builder {
	clone := *b
	clone.limitN = 1
	return &clone
}

// Get 获取多条数据
func (b *Builder) Get() ([]map[string]any, error) {
	rows, err := b.queryRows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows, b.selects, b.limitN)
}

func (b *Builder) queryRows() (*sql.Rows, error) {
	sqlStr, args := b.buildSelectSQL()
	b.logSQL(sqlStr, args)
	rows, err := b.db.QueryContext(b.ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("orm get: %w\nsql=%s args=%v", err, sqlStr, args)
	}
	return rows, nil
}

// Value 获取单列单值，并转成数字或字符串。会清空已有 Select，只查这一列，并 LIMIT 1。
// 列名可以写成 id 或 c.id。手动写了 as 时，SQL 带上这个别名。
// 没有这条数据时返回 nil 和 sql.ErrNoRows。这一列是 NULL 时返回该类型的零值。
//
//	name, err := D("user").Where("id", 1).Value[string]("name")
//	age, err := D("user").Where("id", 1).Value[int]("age")
func (b *Builder) Value[T numberOrString](column string) (*T, error) {
	clone := b.Select(column).singleRow()
	rows, err := clone.queryRows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var raw any
	// 逗号拆成多列，或 u.* 展开成多列时，仍按结果键取出，避免 Scan 目标个数对不上。
	if len(clone.selects) != 1 || isSelectWildcard(clone.selects[0].expr) {
		var row map[string]any
		row, err = scanOne(rows, clone.selects)
		if err != nil {
			return nil, err
		}
		if row == nil {
			return nil, sql.ErrNoRows
		}
		raw = row[selectFieldKey(column)]
	} else {
		var found bool
		raw, found, err = scanValue(rows)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, sql.ErrNoRows
		}
	}
	v, err := castNumberOrString[T](raw)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func castNumberOrString[T numberOrString](v any) (T, error) {
	var zero T
	if v == nil {
		return zero, nil
	}
	dt := reflect.TypeFor[T]()
	rv := reflect.ValueOf(v)
	if rv.Type() == dt {
		return v.(T), nil
	}
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
		return castText[T](string(rv.Bytes()))
	}
	if dt.Kind() == reflect.String {
		text, ok := scalarText(rv)
		if !ok {
			return zero, fmt.Errorf("orm value: cannot store %T in %s", v, dt)
		}
		out := reflect.New(dt).Elem()
		out.SetString(text)
		return out.Interface().(T), nil
	}
	if rv.Kind() == reflect.String {
		return castText[T](rv.String())
	}
	out := reflect.New(dt).Elem()
	switch dt.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, ok := signedValue(rv)
		if !ok || out.OverflowInt(n) {
			return zero, fmt.Errorf("orm value: cannot store %T in %s", v, dt)
		}
		out.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, ok := unsignedValue(rv)
		if !ok || out.OverflowUint(n) {
			return zero, fmt.Errorf("orm value: cannot store %T in %s", v, dt)
		}
		out.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, ok := floatValue(rv)
		if !ok || out.OverflowFloat(n) {
			return zero, fmt.Errorf("orm value: cannot store %T in %s", v, dt)
		}
		out.SetFloat(n)
	default:
		return zero, fmt.Errorf("orm value: cannot store %T in %s", v, dt)
	}
	return out.Interface().(T), nil
}

func castText[T numberOrString](s string) (T, error) {
	var zero T
	dt := reflect.TypeFor[T]()
	out := reflect.New(dt).Elem()
	switch dt.Kind() {
	case reflect.String:
		out.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, dt.Bits())
		if err != nil {
			return zero, fmt.Errorf("orm value: cannot store %q in %s", s, dt)
		}
		out.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, dt.Bits())
		if err != nil {
			return zero, fmt.Errorf("orm value: cannot store %q in %s", s, dt)
		}
		out.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(s, dt.Bits())
		if err != nil {
			return zero, fmt.Errorf("orm value: cannot store %q in %s", s, dt)
		}
		out.SetFloat(n)
	default:
		return zero, fmt.Errorf("orm value: cannot store %q in %s", s, dt)
	}
	return out.Interface().(T), nil
}

func scalarText(rv reflect.Value) (string, bool) {
	switch rv.Kind() {
	case reflect.String:
		return rv.String(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10), true
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'f', -1, 64), true
	default:
		return "", false
	}
}

func signedValue(rv reflect.Value) (int64, bool) {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u := rv.Uint()
		if u > math.MaxInt64 {
			return 0, false
		}
		return int64(u), true
	default:
		return 0, false
	}
}

func unsignedValue(rv reflect.Value) (uint64, bool) {
	switch rv.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := rv.Int()
		if n < 0 {
			return 0, false
		}
		return uint64(n), true
	default:
		return 0, false
	}
}

func floatValue(rv reflect.Value) (float64, bool) {
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		return rv.Float(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint()), true
	default:
		return 0, false
	}
}

// scanValue 读取第一行的第一列。没有行时 found 为 false。
func scanValue(rows *sql.Rows) (any, bool, error) {
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, false, err
	}
	kind := kindString
	if len(types) > 0 && types[0] != nil {
		kind = columnValueKind(types[0].DatabaseTypeName())
	}
	if !rows.Next() {
		return nil, false, rows.Err()
	}
	var v any
	if err := rows.Scan(&v); err != nil {
		return nil, false, err
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return normalizeValue(v, kind), true, nil
}

// columnResultKey 去掉表别名：c.id、db.coin.id 的结果键都是 id
func columnResultKey(column string) string {
	column = strings.TrimSpace(column)
	if i := strings.LastIndex(column, "."); i >= 0 {
		return strings.TrimSpace(column[i+1:])
	}
	return column
}

// selectFieldKey 是写入 map 的字段名。
// 手动指定了 as 时用这个别名；否则去掉表别名。
func selectFieldKey(expr string) string {
	if _, alias, ok := splitSelectAlias(expr); ok {
		return strings.Trim(strings.TrimSpace(alias), "`")
	}
	return columnResultKey(expr)
}

// Count 统计数量（包含 Join）
func (b *Builder) Count() (int64, error) {
	fromSQL, args := b.buildFromClause()
	sqlStr := "SELECT COUNT(*) " + fromSQL
	whereSQL, whereArgs := b.buildWhere()
	if whereSQL != "" {
		sqlStr += " WHERE " + whereSQL
		args = append(args, whereArgs...)
	}
	b.logSQL(sqlStr, args)
	var n int64
	if err := b.db.QueryRowContext(b.ctx, sqlStr, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// Exists 是否存在。只查 SELECT 1 ... LIMIT 1，找到第一条就停。
// 已写的 Limit、OrderBy 不参与这次判断，原构造器保持不变。
func (b *Builder) Exists() (bool, error) {
	sqlStr, args := b.existsSQL()
	b.logSQL(sqlStr, args)
	var n int
	err := b.db.QueryRowContext(b.ctx, sqlStr, args...).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (b *Builder) existsSQL() (string, []any) {
	fromSQL, args := b.buildFromClause()
	sqlStr := "SELECT 1 " + fromSQL
	whereSQL, whereArgs := b.buildWhere()
	if whereSQL != "" {
		sqlStr += " WHERE " + whereSQL
		args = append(args, whereArgs...)
	}
	sqlStr += " LIMIT 1"
	return sqlStr, args
}

// Insert 插入一行或多行，返回第一条生成的自增 id。
// 多行合成一条 INSERT，只往返一次。各行的字段必须一致。
//
//	D("user").Insert(H{"name": "ann", "age": 20})
//	D("user").Insert(
//	    H{"name": "ann", "age": 20},
//	    H{"name": "bob", "age": 21},
//	)
//	D("user").Insert(rows...)
func (b *Builder) Insert(rows ...map[string]any) (int64, error) {
	sqlStr, args, err := b.insertSQL(rows)
	if err != nil {
		return 0, err
	}
	b.logSQL(sqlStr, args)
	res, err := b.db.ExecContext(b.ctx, sqlStr, args...)
	if err != nil {
		return 0, fmt.Errorf("orm insert: %w\nsql=%s", err, sqlStr)
	}
	return res.LastInsertId()
}

func (b *Builder) insertSQL(rows []map[string]any) (string, []any, error) {
	if len(rows) == 0 || len(rows[0]) == 0 {
		return "", nil, fmt.Errorf("orm insert: empty data")
	}
	cols := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		if strings.TrimSpace(k) == "" {
			return "", nil, fmt.Errorf("orm insert: empty column")
		}
		cols = append(cols, k)
	}
	sort.Strings(cols)

	quoted := make([]string, len(cols))
	marks := make([]string, len(cols))
	for i, col := range cols {
		quoted[i] = quoteIdent(col)
		marks[i] = "?"
	}
	group := "(" + strings.Join(marks, ", ") + ")"
	groups := make([]string, len(rows))
	args := make([]any, 0, len(rows)*len(cols))
	for i, row := range rows {
		if len(row) != len(cols) {
			return "", nil, fmt.Errorf("orm insert: row %d has %d columns, want %d", i, len(row), len(cols))
		}
		for _, col := range cols {
			v, ok := row[col]
			if !ok {
				return "", nil, fmt.Errorf("orm insert: row %d missing column %q", i, col)
			}
			args = append(args, v)
		}
		groups[i] = group
	}
	sqlStr := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		quoteIdent(baseTableName(b.table)),
		strings.Join(quoted, ", "),
		strings.Join(groups, ", "),
	)
	return sqlStr, args, nil
}

// Update 按当前 Where 条件更新，返回影响行数。会带上 OrderBy 和 Limit。
//
//	D("user").Where("id", 1).Update(map[string]any{"age": 10})
//	D("user").Where("status", 0).OrderBy("id", "asc").Limit(1).Update(map[string]any{"age": 10})
func (b *Builder) Update(data map[string]any) (int64, error) {
	sqlStr, args, err := b.updateSQL(data)
	if err != nil {
		return 0, err
	}
	b.logSQL(sqlStr, args)
	res, err := b.db.ExecContext(b.ctx, sqlStr, args...)
	if err != nil {
		return 0, fmt.Errorf("orm update: %w\nsql=%s", err, sqlStr)
	}
	return res.RowsAffected()
}

func (b *Builder) updateSQL(data map[string]any) (string, []any, error) {
	if len(data) == 0 {
		return "", nil, fmt.Errorf("orm update: empty data")
	}
	if len(b.wheres) == 0 {
		return "", nil, fmt.Errorf("orm update: where clause is required (refusing full-table update)")
	}

	cols := make([]string, 0, len(data))
	for k := range data {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	sets := make([]string, len(cols))
	args := make([]any, 0, len(cols))
	for i, k := range cols {
		sets[i] = quoteIdent(k) + " = ?"
		args = append(args, data[k])
	}
	whereSQL, whereArgs := b.buildWhere()
	args = append(args, whereArgs...)
	sqlStr := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s",
		quoteTableRef(b.table),
		strings.Join(sets, ", "),
		whereSQL,
	)
	tail, err := b.orderLimitSQL("update")
	if err != nil {
		return "", nil, err
	}
	return sqlStr + tail, args, nil
}

// Truncate 清空整张表（TRUNCATE TABLE），并重置自增 ID。
// 不看 Where 条件，始终清空 D() 指定的表。
//
//	D("user").Truncate()
func (b *Builder) Truncate() error {
	sqlStr, err := b.truncateSQL()
	if err != nil {
		return err
	}
	b.logSQL(sqlStr, nil)
	if _, err := b.db.ExecContext(b.ctx, sqlStr); err != nil {
		return fmt.Errorf("orm truncate: %w\nsql=%s", err, sqlStr)
	}
	return nil
}

func (b *Builder) truncateSQL() (string, error) {
	table := strings.TrimSpace(baseTableName(b.table))
	if table == "" {
		return "", fmt.Errorf("orm truncate: empty table")
	}
	return "TRUNCATE TABLE " + quoteIdent(table), nil
}

// Delete 按当前 Where 条件删除，返回影响行数。会带上 OrderBy 和 Limit。
//
//	D("user").Where("status", 0).OrderBy("id", "asc").Limit(1).Delete()
func (b *Builder) Delete() (int64, error) {
	sqlStr, args, err := b.deleteSQL()
	if err != nil {
		return 0, err
	}
	b.logSQL(sqlStr, args)
	res, err := b.db.ExecContext(b.ctx, sqlStr, args...)
	if err != nil {
		return 0, fmt.Errorf("orm delete: %w\nsql=%s", err, sqlStr)
	}
	return res.RowsAffected()
}

func (b *Builder) deleteSQL() (string, []any, error) {
	if len(b.wheres) == 0 {
		return "", nil, fmt.Errorf("orm delete: where clause is required (refusing full-table delete)")
	}
	whereSQL, args := b.buildWhere()
	sqlStr := fmt.Sprintf("DELETE FROM %s WHERE %s", quoteTableRef(b.table), whereSQL)
	tail, err := b.orderLimitSQL("delete")
	if err != nil {
		return "", nil, err
	}
	return sqlStr + tail, args, nil
}

// orderLimitSQL 拼上单表 UPDATE/DELETE 支持的 ORDER BY 和 LIMIT。
// MySQL 这两类语句没有 OFFSET。
func (b *Builder) orderLimitSQL(name string) (string, error) {
	if b.offsetN > 0 {
		return "", fmt.Errorf("orm %s: offset is not supported", name)
	}
	var sb strings.Builder
	if len(b.orders) > 0 {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(strings.Join(b.orders, ", "))
	}
	if b.limitN >= 0 {
		fmt.Fprintf(&sb, " LIMIT %d", b.limitN)
	}
	return sb.String(), nil
}

// number 是 Increment、Decrement 允许的步长。
type number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

type numberOrString interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64 | ~string
}

// Increment 字段自增，返回影响行数。步长必须传入。
//
//	D("user").Where("id", 1).Increment("age", 2)
//	D("user").Where("id", 1).Increment("score", 1.5)
//	D("user").Where("id", 1).Increment("n", float32(0.5))
func (b *Builder) Increment[T number](column string, amount T) (int64, error) {
	return b.stepColumn("increment", "+", column, amount)
}

// Decrement 字段自减，返回影响行数。步长必须传入。
//
//	D("user").Where("id", 1).Decrement("age", 2)
//	D("user").Where("id", 1).Decrement("score", 0.5)
func (b *Builder) Decrement[T number](column string, amount T) (int64, error) {
	return b.stepColumn("decrement", "-", column, amount)
}

func (b *Builder) stepColumn[T number](name, op, column string, amount T) (int64, error) {
	sqlStr, args, err := b.stepSQL(name, op, column, amount)
	if err != nil {
		return 0, err
	}
	b.logSQL(sqlStr, args)
	res, err := b.db.ExecContext(b.ctx, sqlStr, args...)
	if err != nil {
		return 0, fmt.Errorf("orm %s: %w\nsql=%s", name, err, sqlStr)
	}
	return res.RowsAffected()
}

func (b *Builder) stepSQL(name, op, column string, amount any) (string, []any, error) {
	if len(b.wheres) == 0 {
		return "", nil, fmt.Errorf("orm %s: where clause is required", name)
	}
	whereSQL, args := b.buildWhere()
	sqlStr := fmt.Sprintf(
		"UPDATE %s SET %s = %s %s ? WHERE %s",
		quoteTableRef(b.table),
		quoteIdent(column),
		quoteIdent(column),
		op,
		whereSQL,
	)
	tail, err := b.orderLimitSQL(name)
	if err != nil {
		return "", nil, err
	}
	return sqlStr + tail, append([]any{amount}, args...), nil
}

// scanRowCapLimit 是按 Limit 预分配结果切片的上限。
// 再大就从空切片开始扩容，避免 Limit 很大时一次分配过多。
const scanRowCapLimit = 256

// rowScan 保存一次查询里每列固定的信息，以及可复用的扫描缓冲。
type rowScan struct {
	keys   []string
	kinds  []valueKind
	values []any
	ptrs   []any
}

// columnLayout 是一次查询里每列固定的结果键和类型。
type columnLayout struct {
	keys  []string
	kinds []valueKind
}

func readColumnLayout(rows *sql.Rows, selects []selectColumn) (columnLayout, error) {
	cols, err := rows.Columns()
	if err != nil {
		return columnLayout{}, err
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return columnLayout{}, err
	}
	nCols := len(cols)
	keys := make([]string, nCols)
	star := firstWildcardIndex(selects)
	for i, col := range cols {
		keys[i] = fieldKey(i, col, selects, star)
	}
	kinds := make([]valueKind, nCols)
	for i := range kinds {
		if i < len(types) && types[i] != nil {
			kinds[i] = columnValueKind(types[i].DatabaseTypeName())
		}
	}
	return columnLayout{keys: keys, kinds: kinds}, nil
}

// prepareRowScan 在读取行之前算好 map 键和列类型。这两样在同一列上全程不变。
func prepareRowScan(rows *sql.Rows, selects []selectColumn) (rowScan, error) {
	layout, err := readColumnLayout(rows, selects)
	if err != nil {
		return rowScan{}, err
	}
	nCols := len(layout.keys)
	values := make([]any, nCols)
	ptrs := make([]any, nCols)
	for i := range values {
		ptrs[i] = &values[i]
	}
	return rowScan{keys: layout.keys, kinds: layout.kinds, values: values, ptrs: ptrs}, nil
}

func (s rowScan) rowMap() map[string]any {
	row := make(map[string]any, len(s.keys))
	for i := range s.keys {
		row[s.keys[i]] = normalizeValue(s.values[i], s.kinds[i])
	}
	return row
}

func scanRows(rows *sql.Rows, selects []selectColumn, limit int) ([]map[string]any, error) {
	layout, err := prepareRowScan(rows, selects)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, rowCapacity(limit))
	for rows.Next() {
		if err := rows.Scan(layout.ptrs...); err != nil {
			return nil, err
		}
		result = append(result, layout.rowMap())
	}
	return result, rows.Err()
}

// scanOne 只读取第一行，不分配结果切片。没有行时返回 nil, nil。
func scanOne(rows *sql.Rows, selects []selectColumn) (map[string]any, error) {
	layout, err := prepareRowScan(rows, selects)
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		return nil, rows.Err()
	}
	if err := rows.Scan(layout.ptrs...); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return layout.rowMap(), nil
}

// rowCapacity 在 Limit 为 1 到 scanRowCapLimit 时返回这个条数，作为结果切片容量。
// 超过上限、为 0 或未设置时返回 0，结果切片随行数扩容。
func rowCapacity(limit int) int {
	if limit > 0 && limit <= scanRowCapLimit {
		return limit
	}
	return 0
}

// mappedFieldName 决定 map 的键。
// Select("u.id as user_id") 用 user_id；Select("u.id") 用 id。
// u.* 会展开成多列，星号以及它后面的列用驱动返回的列名。
// 没有对应 Select 时，按驱动返回的列名去掉表别名。
func mappedFieldName(i int, driverCol string, selects []selectColumn) string {
	return fieldKey(i, driverCol, selects, firstWildcardIndex(selects))
}

func fieldKey(i int, driverCol string, selects []selectColumn, star int) string {
	if star >= 0 && i >= star {
		return columnResultKey(driverCol)
	}
	if i < len(selects) {
		return selectResultKey(selects[i], driverCol)
	}
	return columnResultKey(driverCol)
}

func firstWildcardIndex(selects []selectColumn) int {
	for i, s := range selects {
		if isSelectWildcard(s.expr) {
			return i
		}
	}
	return -1
}

func selectResultKey(c selectColumn, driverCol string) string {
	if isSelectWildcard(c.expr) {
		return columnResultKey(driverCol)
	}
	if _, _, ok := splitSelectAlias(c.expr); ok || !c.raw {
		return selectFieldKey(c.expr)
	}
	return columnResultKey(driverCol)
}

func isSelectWildcard(expr string) bool {
	col := expr
	if c, _, ok := splitSelectAlias(expr); ok {
		col = c
	}
	col = strings.TrimSpace(col)
	return col == "*" || strings.HasSuffix(col, ".*")
}

// valueKind 是列类型在扫描前归好的类别。行循环里按这个分支，不再处理类型名。
type valueKind uint8

const (
	kindString valueKind = iota
	kindBytes
	kindInt8
	kindInt16
	kindInt32
	kindInt64
	kindUint8
	kindUint16
	kindUint32
	kindUint64
	kindFloat32
	kindFloat64
	kindBit
)

func columnValueKind(dbType string) valueKind {
	switch strings.ToUpper(strings.TrimSpace(dbType)) {
	case "TINYINT":
		return kindInt8
	case "UNSIGNED TINYINT":
		return kindUint8
	case "SMALLINT":
		return kindInt16
	case "UNSIGNED SMALLINT", "YEAR":
		return kindUint16
	case "MEDIUMINT", "INT", "INTEGER":
		return kindInt32
	case "UNSIGNED MEDIUMINT", "UNSIGNED INT", "UNSIGNED INTEGER":
		return kindUint32
	case "BIGINT":
		return kindInt64
	case "UNSIGNED BIGINT":
		return kindUint64
	case "FLOAT":
		return kindFloat32
	case "DOUBLE":
		return kindFloat64
	case "BIT":
		return kindBit
	case "BINARY", "VARBINARY", "BLOB", "TINYBLOB", "MEDIUMBLOB", "LONGBLOB", "GEOMETRY", "VECTOR":
		return kindBytes
	default:
		return kindString
	}
}

// normalizeValue 按列类型转换驱动返回的 []byte。
// 整型按宽度对应 int8 到 int64，无符号对应 uint8 到 uint64。
// FLOAT 为 float32，DOUBLE 为 float64。DECIMAL 保留字符串以免丢失精度。
func normalizeValue(v any, kind valueKind) any {
	switch t := v.(type) {
	case nil:
		return nil
	case time.Time:
		return t
	case []byte:
		return convertBytes(t, kind)
	default:
		return t
	}
}

func convertBytes(b []byte, kind valueKind) any {
	switch kind {
	case kindInt8:
		return convertInt(b, 8)
	case kindUint8:
		return convertUint(b, 8)
	case kindInt16:
		return convertInt(b, 16)
	case kindUint16:
		return convertUint(b, 16)
	case kindInt32:
		return convertInt(b, 32)
	case kindUint32:
		return convertUint(b, 32)
	case kindInt64:
		return convertInt(b, 64)
	case kindUint64:
		return convertUint(b, 64)
	case kindFloat32:
		return convertFloat(b, 32)
	case kindFloat64:
		return convertFloat(b, 64)
	case kindBit:
		return convertBit(b)
	case kindBytes:
		return b
	default:
		// DECIMAL、VARCHAR、TEXT、JSON 等保持字符串
		return string(b)
	}
}

func convertInt(b []byte, bitSize int) any {
	n, err := strconv.ParseInt(string(b), 10, bitSize)
	if err != nil {
		return string(b)
	}
	switch bitSize {
	case 8:
		return int8(n)
	case 16:
		return int16(n)
	case 32:
		return int32(n)
	default:
		return n
	}
}

func convertUint(b []byte, bitSize int) any {
	n, err := strconv.ParseUint(string(b), 10, bitSize)
	if err != nil {
		return string(b)
	}
	switch bitSize {
	case 8:
		return uint8(n)
	case 16:
		return uint16(n)
	case 32:
		return uint32(n)
	default:
		return n
	}
}

func convertFloat(b []byte, bitSize int) any {
	f, err := strconv.ParseFloat(string(b), bitSize)
	if err != nil {
		return string(b)
	}
	if bitSize == 32 {
		return float32(f)
	}
	return f
}

func convertBit(b []byte) any {
	var n uint64
	for _, c := range b {
		n = (n << 8) | uint64(c)
	}
	if n > math.MaxInt64 {
		return strconv.FormatUint(n, 10)
	}
	return int64(n)
}
