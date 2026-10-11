package orm

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
	"time"
)

// A 切片条件。第一个元素必须是运算符，后面是对应的值。
//
//	Where(H{
//	    "ctVal":  []any{">", 10},
//	    "instId": []any{"between", 1, 10},
//	    "status": []any{"in", 1, 2},
//	})

type H = map[string]any

func Op[T int | float64 | string](op string, args ...T) []any {
	key := strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(op)), " "))
	switch key {
	case ">", "<", "<=", ">=", "=", "!=", "<>", "LIKE", "NOT LIKE":
		if len(args) != 1 {
			panic(fmt.Sprintf("orm: operator %s requires 1 value", key))
		}
		return []any{key, args[0]}
	case "IS":
		if len(args) != 1 {
			panic("orm: operator IS requires 1 value")
		}
		flag := strings.ToUpper(strings.Join(strings.Fields(fmt.Sprint(args[0])), " "))
		switch flag {
		case "NULL":
			return []any{"IS NULL"}
		case "NOT NULL":
			return []any{"IS NOT NULL"}
		default:
			panic(fmt.Sprintf("orm: unsupported IS operator %v", args[0]))
		}
	case "IS NULL", "IS NOT NULL":
		if len(args) != 0 {
			panic(fmt.Sprintf("orm: operator %s requires 0 values", key))
		}
		return []any{key}
	case "IN", "NOT IN":
		if len(args) == 0 {
			panic(fmt.Sprintf("orm: operator %s requires at least 1 value", key))
		}
		out := make([]any, 1, 1+len(args))
		out[0] = key
		for _, v := range args {
			out = append(out, v)
		}
		return out
	case "BETWEEN", "NOT BETWEEN":
		if len(args) != 2 {
			panic(fmt.Sprintf("orm: operator %s requires 2 values", key))
		}
		return []any{key, args[0], args[1]}
	default:
		panic(fmt.Sprintf("orm: unsupported operator %q", op))
	}
}

// 比较条件。Where(H{...}) 里普通值表示相等，这些结构体用来指定运算符。
//
//	Where(H{
//	    "id":     2,
//	    "instId": Eq{Value: "BTCUSDT"},
//	    "age":    Gt{Value: 18},
//	})
type (
	Eq         struct{ Value any }    // =
	Ne         struct{ Value any }    // !=
	Gt         struct{ Value any }    // >
	Gte        struct{ Value any }    // >=
	Lt         struct{ Value any }    // <
	Lte        struct{ Value any }    // <=
	Like       struct{ Value any }    // LIKE
	NotLike    struct{ Value any }    // NOT LIKE
	In         struct{ Value []any }  // IN
	Between    struct{ Min, Max any } // BETWEEN min AND max
	NotBetween struct{ Min, Max any } // NOT BETWEEN min AND max
	Is         struct{ Value any }    // IS NULL，Value 必须为 nil
)

// WhereFunc Laravel 风格 Where 回调
//
//	Where(func(q *Builder) {
//	    q.Where("status", 1).OrWhere("vip", 1)
//	})
type WhereFunc func(q *Builder)

type whereItem struct {
	and      bool // true=AND, false=OR
	column   string
	operator string
	value    any
	raw      string // 原始 SQL 片段，如 "age > ?"
	rawArgs  []any
	in       bool
	between  bool
	notBet   bool
	nested   []whereItem // 回调分组条件
}

type selectColumn struct {
	expr string
	raw  bool // true 时不自动加反引号
}

type joinItem struct {
	joinType string // INNER / LEFT / RIGHT
	table    string
	first    string
	operator string
	second   string
	raw      string // JoinRaw 原始片段
	rawArgs  []any
}

// Builder Laravel 风格查询构造器
type Builder struct {
	db      *sql.DB
	table   string
	ctx     context.Context
	selects []selectColumn
	joins   []joinItem
	wheres  []whereItem
	orders  []string
	limitN  int
	offsetN int
	debug   bool // 默认 false；为 true 时打印 SQL
}

func NewBuilder(db *sql.DB, table string) *Builder {
	return &Builder{
		db:     db,
		table:  table,
		ctx:    context.Background(),
		limitN: -1,
		debug:  false,
	}
}

// WithContext 设置上下文
func (b *Builder) WithContext(ctx context.Context) *Builder {
	b.ctx = ctx
	return b
}

// Debug 临时开启当前构造器的 SQL 打印
//
//	D("user").Debug().Where("id", 1).Find()
func (b *Builder) Debug() *Builder {
	b.debug = true
	return b
}

func (b *Builder) logSQL(sqlStr string, args []any) {
	if !b.debug {
		return
	}
	log.Printf("[orm debug] sql: %s | args: %v", sqlStr, args)
}

// Select 指定查询字段（自动用 ` 包裹），默认 *
// SQL 不会自动加字段别名。结果写入 map 时去掉表别名，Select("u.id") 的键是 id。
//
//	Select("id", "name")
//	Select("id,instId") // 也支持逗号分隔
//	Select("u.id", "c.email")
//	Select("u.id as user_id") // 只有手动写了别名，SQL 才带 AS
func (b *Builder) Select(columns ...string) *Builder {
	b.selects = nil
	for _, col := range columns {
		for _, part := range splitSelectFields(col) {
			b.selects = append(b.selects, selectColumn{expr: part, raw: false})
		}
	}
	return b
}

// SelectRaw 指定原始查询字段（不加反引号）
//
//		SelectRaw("ctVal")
//		SelectRaw("COUNT(*) AS total")
//	 没写 as、只是两个词，也会被当成别名。SelectRaw("DISTINCT u.id") 的结果键会变成 u.id  所以最好是这样用 SelectRaw("DISTINCT u.id as id")
//
// SelectRaw("u.id, c.email") 也不会按逗号拆开，第一列的键会对错  所以分开传递 SelectRaw("u.id").SelectRaw("c.email") 或者 SelectRaw("u.id, c.email")
func (b *Builder) SelectRaw(expressions ...string) *Builder {
	for _, expr := range expressions {
		expr = strings.TrimSpace(expr)
		if expr == "" {
			continue
		}
		b.selects = append(b.selects, selectColumn{expr: expr, raw: true})
	}
	return b
}

// Join INNER JOIN，用法类似 Laravel
//
//	Join("contacts", "users.id", "=", "contacts.user_id")
//	Join("contacts", "users.id", "contacts.user_id") // 默认 =
//	Join("contacts as c", "users.id", "=", "c.user_id")
func (b *Builder) Join(table string, args ...any) *Builder {
	return b.addJoin("INNER", table, args...)
}

// LeftJoin LEFT JOIN
func (b *Builder) LeftJoin(table string, args ...any) *Builder {
	return b.addJoin("LEFT", table, args...)
}

// RightJoin RIGHT JOIN
func (b *Builder) RightJoin(table string, args ...any) *Builder {
	return b.addJoin("RIGHT", table, args...)
}

// JoinRaw 原始 JOIN 片段
//
//	JoinRaw("LEFT JOIN contacts AS c ON users.id = c.user_id AND c.status = ?", 1)
func (b *Builder) JoinRaw(query string, args ...any) *Builder {
	query = strings.TrimSpace(query)
	if query == "" {
		panic("orm: JoinRaw query is empty")
	}
	b.joins = append(b.joins, joinItem{raw: query, rawArgs: args})
	return b
}

func (b *Builder) addJoin(joinType, table string, args ...any) *Builder {
	table = strings.TrimSpace(table)
	if table == "" {
		panic("orm: join table is required")
	}

	var first, operator, second string
	switch len(args) {
	case 2:
		var ok bool
		first, ok = args[0].(string)
		if !ok || strings.TrimSpace(first) == "" {
			panic("orm: join first column must be a non-empty string")
		}
		second, ok = args[1].(string)
		if !ok || strings.TrimSpace(second) == "" {
			panic("orm: join second column must be a non-empty string")
		}
		operator = "="
	case 3:
		var ok bool
		first, ok = args[0].(string)
		if !ok || strings.TrimSpace(first) == "" {
			panic("orm: join first column must be a non-empty string")
		}
		op, ok := args[1].(string)
		if !ok {
			panic("orm: join operator must be a string")
		}
		normalized, ok := normalizeOperator(op)
		if !ok || isNullOperator(normalized) {
			panic(fmt.Sprintf("orm: unsupported join operator %q", op))
		}
		second, ok = args[2].(string)
		if !ok || strings.TrimSpace(second) == "" {
			panic("orm: join second column must be a non-empty string")
		}
		operator = normalized
	default:
		panic("orm: Join requires 2 or 3 arguments after table")
	}

	b.joins = append(b.joins, joinItem{
		joinType: joinType,
		table:    table,
		first:    first,
		operator: operator,
		second:   second,
	})
	return b
}

// Where 添加 AND 条件
//
//	Where("id", 1)                    -> id = ?
//	Where("age", ">", 18)             -> age > ?
//	Where("name", "like", "%a%")
//	Where("id", "in", []any{1, 2})    -> id IN (?, ?)
//	Where("age", "between", 1, 10)    -> age BETWEEN ? AND ?
//	Where("age", "is", "null")        -> age IS NULL
//	Where("age", "is", "not null")    -> age IS NOT NULL
//	Where(H{"id": 2, "age": Gt{18}})
//	Where(func(q *Builder) {    -> ( ... )
//	    q.Where("a", 1).OrWhere("b", 2)
//	})
func (b *Builder) Where(args ...any) *Builder {
	return b.addWhere(true, args...)
}

// OrWhere 添加 OR 条件，同样支持回调分组
func (b *Builder) OrWhere(args ...any) *Builder {
	return b.addWhere(false, args...)
}

// WhereIn column IN (?)。values 不能是 nil 或空切片，元素只能是数字或字符串。
//
//	WhereIn("id", []int{1, 2})
//	WhereIn("name", []string{"a", "b"})
func (b *Builder) WhereIn[T numberOrString](column string, values []T) *Builder {
	if values == nil {
		panic("orm: WhereIn values must not be nil")
	}
	if len(values) == 0 {
		panic("orm: WhereIn values must not be empty")
	}
	args := make([]any, len(values))
	for i, v := range values {
		args[i] = v
	}
	b.wheres = append(b.wheres, whereItem{
		and:    true,
		column: column,
		in:     true,
		value:  args,
	})
	return b
}

// WhereRaw 原始条件，如 WhereRaw("age > ?", 18)
func (b *Builder) WhereRaw(query string, args ...any) *Builder {
	b.wheres = append(b.wheres, whereItem{
		and:     true,
		raw:     query,
		rawArgs: args,
	})
	return b
}

// OrderBy 排序。方向必须是 ASC 或 DESC，忽略大小写。
//
//	OrderBy("id", "asc")
//	OrderBy("id", "desc")
func (b *Builder) OrderBy(column, direction string) *Builder {
	dir := strings.ToUpper(strings.TrimSpace(direction))
	if dir != "ASC" && dir != "DESC" {
		panic(fmt.Sprintf("orm: OrderBy direction must be ASC or DESC, got %q", direction))
	}
	b.orders = append(b.orders, fmt.Sprintf("%s %s", quoteIdent(column), dir))
	return b
}

// Limit 限制条数
func (b *Builder) Limit(n int) *Builder {
	b.limitN = n
	return b
}

// Offset 偏移
func (b *Builder) Offset(n int) *Builder {
	b.offsetN = n
	return b
}

func (b *Builder) addWhere(and bool, args ...any) *Builder {
	// Where(func(q *Builder){ ... }) / OrWhere(func...)
	if len(args) == 1 {
		switch fn := args[0].(type) {
		case WhereFunc:
			return b.addWhereGroup(and, fn)
		case func(*Builder):
			return b.addWhereGroup(and, fn)
		case map[string]any:
			return b.addWhereMap(and, fn)
		default:
			panic("orm: Where callback must be func(*Builder) or map[string]any")
		}
	}

	switch len(args) {
	case 2:
		col, ok := args[0].(string)
		if !ok || strings.TrimSpace(col) == "" {
			panic("orm: Where column must be a non-empty string")
		}
		b.appendCond(and, col, args[1])
	case 3:
		col, op := whereColOp(args[0], args[1])
		normalized, ok := normalizeSliceOperator(op)
		if !ok {
			panic(fmt.Sprintf("orm: unsupported Where operator %q", op))
		}
		switch normalized {
		case "IN", "NOT IN":
			b.appendSliceCond(and, col, inValues(normalized, args[2]))
		case "BETWEEN", "NOT BETWEEN":
			b.appendSliceCond(and, col, betweenValues(normalized, args[2]))
		case "IS":
			b.wheres = append(b.wheres, newWhereItem(and, col, isFlagOperator(args[2]), nil))
		default:
			b.wheres = append(b.wheres, newWhereItem(and, col, normalized, args[2]))
		}
	case 4:
		col, op := whereColOp(args[0], args[1])
		normalized, ok := normalizeSliceOperator(op)
		if !ok || (normalized != "BETWEEN" && normalized != "NOT BETWEEN") {
			panic("orm: Where with 4 arguments only supports between and not between")
		}
		b.appendSliceCond(and, col, []any{normalized, args[2], args[3]})
	default:
		panic("orm: Where requires 1 (callback), 2 or 3 arguments, or 4 for between")
	}
	return b
}

func whereColOp(column, operator any) (string, string) {
	col, ok := column.(string)
	if !ok || strings.TrimSpace(col) == "" {
		panic("orm: Where column must be a non-empty string")
	}
	op, ok := operator.(string)
	if !ok {
		panic("orm: Where operator must be a string")
	}
	return col, op
}

// inValues 把 Where("id", "in", []any{1, 2}) 转成切片条件
func inValues(op string, value any) []any {
	vals, ok := condSlice(value)
	if !ok {
		panic(fmt.Sprintf("orm: operator %s requires a slice", op))
	}
	out := make([]any, 0, 1+len(vals))
	out = append(out, op)
	out = append(out, vals...)
	return out
}

// betweenValues 接受长度为 2 的切片。两个端点请用 Where("age", "between", 1, 10)
func betweenValues(op string, value any) []any {
	vals, ok := condSlice(value)
	if !ok {
		panic(fmt.Sprintf("orm: operator %s requires 2 values", op))
	}
	out := make([]any, 0, 1+len(vals))
	out = append(out, op)
	out = append(out, vals...)
	return out
}

// 允许的比较运算符白名单（防止运算符被拼进 SQL 注入）。
// 大小写两种写法都放进来，常见运算符直接查表，不必先做字符串整理。
var allowedOperators = map[string]string{
	"=":           "=",
	"!=":          "!=",
	"<>":          "<>",
	">":           ">",
	"<":           "<",
	">=":          ">=",
	"<=":          "<=",
	"like":        "LIKE",
	"LIKE":        "LIKE",
	"not like":    "NOT LIKE",
	"NOT LIKE":    "NOT LIKE",
	"is null":     "IS NULL",
	"IS NULL":     "IS NULL",
	"is not null": "IS NOT NULL",
	"IS NOT NULL": "IS NOT NULL",
}

var sliceOperators = map[string]string{
	"is":          "IS",
	"IS":          "IS",
	"in":          "IN",
	"IN":          "IN",
	"not in":      "NOT IN",
	"NOT IN":      "NOT IN",
	"between":     "BETWEEN",
	"BETWEEN":     "BETWEEN",
	"not between": "NOT BETWEEN",
	"NOT BETWEEN": "NOT BETWEEN",
}

func canonicalizeOp(op string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(op)), " "))
}

func normalizeOperator(op string) (string, bool) {
	if normalized, ok := allowedOperators[op]; ok {
		return normalized, true
	}
	normalized, ok := allowedOperators[canonicalizeOp(op)]
	return normalized, ok
}

func newWhereItem(and bool, column, operator string, value any) whereItem {
	// nil 值：= / 默认 -> IS NULL；!= / <> -> IS NOT NULL
	if value == nil {
		switch operator {
		case "=", "":
			return whereItem{and: and, column: column, operator: "IS NULL"}
		case "!=", "<>":
			return whereItem{and: and, column: column, operator: "IS NOT NULL"}
		case "IS NULL", "IS NOT NULL":
			return whereItem{and: and, column: column, operator: operator}
		default:
			panic(fmt.Sprintf("orm: Where operator %q cannot be used with nil value", operator))
		}
	}
	if operator == "IS NULL" || operator == "IS NOT NULL" {
		// 显式 IS NULL / IS NOT NULL 时忽略右侧值
		return whereItem{and: and, column: column, operator: operator}
	}
	return whereItem{
		and:      and,
		column:   column,
		operator: operator,
		value:    value,
	}
}

// isFlagOperator 把 "null" / "not null" 变成 IS NULL / IS NOT NULL，不绑定参数
func isFlagOperator(value any) string {
	s, ok := value.(string)
	if !ok {
		panic(fmt.Sprintf("orm: unsupported IS operator %v", value))
	}
	flag := strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
	switch flag {
	case "NULL":
		return "IS NULL"
	case "NOT NULL":
		return "IS NOT NULL"
	default:
		panic(fmt.Sprintf("orm: unsupported IS operator %v", value))
	}
}

func isNullOperator(op string) bool {
	return op == "IS NULL" || op == "IS NOT NULL"
}

func (b *Builder) addWhereMap(and bool, conds map[string]any) *Builder {
	if len(conds) == 0 {
		return b
	}
	// map 内部始终 AND，整组再按 Where/OrWhere 接到前面的条件上
	group := &Builder{table: b.table, limitN: -1}
	keys := make([]string, 0, len(conds))
	for k := range conds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, col := range keys {
		if strings.TrimSpace(col) == "" {
			panic("orm: Where column must be a non-empty string")
		}
		group.appendCond(true, col, conds[col])
	}
	if len(group.wheres) == 0 {
		return b
	}
	b.wheres = append(b.wheres, whereItem{
		and:    and,
		nested: group.wheres,
	})
	return b
}

func (b *Builder) appendCond(and bool, col string, value any) {
	if vals, ok := condSlice(value); ok {
		b.appendSliceCond(and, col, vals)
		return
	}
	op, val, in := whereCond(value)
	switch {
	case in:
		b.wheres = append(b.wheres, whereItem{
			and:    and,
			column: col,
			in:     true,
			value:  val,
		})
	case op == "BETWEEN" || op == "NOT BETWEEN":
		b.wheres = append(b.wheres, whereItem{
			and:     and,
			column:  col,
			between: true,
			notBet:  op == "NOT BETWEEN",
			value:   val,
		})
	default:
		b.wheres = append(b.wheres, newWhereItem(and, col, op, val))
	}
}

func (b *Builder) appendSliceCond(and bool, col string, vals []any) {
	if len(vals) == 0 {
		panic("orm: Where slice condition is empty")
	}
	opRaw, ok := vals[0].(string)
	if !ok {
		panic("orm: Where slice condition operator must be a string")
	}
	op, ok := normalizeSliceOperator(opRaw)
	if !ok {
		panic(fmt.Sprintf("orm: unsupported Where operator %q", opRaw))
	}
	args := vals[1:]
	switch op {
	case "IN", "NOT IN":
		if len(args) == 0 {
			panic(fmt.Sprintf("orm: operator %s requires at least 1 value", op))
		}
		b.wheres = append(b.wheres, whereItem{
			and:      and,
			column:   col,
			operator: op,
			in:       true,
			value:    args,
		})
	case "BETWEEN", "NOT BETWEEN":
		if len(args) != 2 {
			panic(fmt.Sprintf("orm: operator %s requires 2 values", op))
		}
		b.wheres = append(b.wheres, whereItem{
			and:     and,
			column:  col,
			between: true,
			notBet:  op == "NOT BETWEEN",
			value:   [2]any{args[0], args[1]},
		})
	case "IS":
		if len(args) != 1 {
			panic("orm: operator IS requires 1 value")
		}
		b.wheres = append(b.wheres, newWhereItem(and, col, isFlagOperator(args[0]), nil))
	case "IS NULL", "IS NOT NULL":
		if len(args) != 0 {
			panic(fmt.Sprintf("orm: operator %s requires 0 values", op))
		}
		b.wheres = append(b.wheres, newWhereItem(and, col, op, nil))
	default:
		if len(args) != 1 {
			panic(fmt.Sprintf("orm: operator %s requires 1 value", op))
		}
		b.wheres = append(b.wheres, newWhereItem(and, col, op, args[0]))
	}
}

// condSlice 把切片条件转成 []any。
// 数字、字符串、时间和比较结构体直接排除。[]byte 是二进制值，按相等条件处理。
// []any 直接复制。其余切片类型才用反射展开。
func condSlice(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		copy(out, v)
		return out, true
	case nil, bool, string, []byte, time.Time,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		Eq, Ne, Gt, Gte, Lt, Lte, Like, NotLike, In, Between, NotBetween, Is:
		return nil, false
	default:
		rv := reflect.ValueOf(v)
		if !rv.IsValid() || rv.Kind() != reflect.Slice {
			return nil, false
		}
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = rv.Index(i).Interface()
		}
		return out, true
	}
}

func normalizeSliceOperator(op string) (string, bool) {
	if normalized, ok := sliceOperators[op]; ok {
		return normalized, true
	}
	if normalized, ok := allowedOperators[op]; ok {
		return normalized, true
	}
	key := canonicalizeOp(op)
	if normalized, ok := sliceOperators[key]; ok {
		return normalized, true
	}
	normalized, ok := allowedOperators[key]
	return normalized, ok
}

func whereCond(value any) (op string, val any, in bool) {
	switch v := value.(type) {
	case Eq:
		return "=", v.Value, false
	case Ne:
		return "!=", v.Value, false
	case Gt:
		return ">", v.Value, false
	case Gte:
		return ">=", v.Value, false
	case Lt:
		return "<", v.Value, false
	case Lte:
		return "<=", v.Value, false
	case Like:
		return "LIKE", v.Value, false
	case NotLike:
		return "NOT LIKE", v.Value, false
	case In:
		return "", v.Value, true
	case Between:
		return "BETWEEN", [2]any{v.Min, v.Max}, false
	case NotBetween:
		return "NOT BETWEEN", [2]any{v.Min, v.Max}, false
	case Is:
		if v.Value != nil {
			panic("orm: Is only accepts nil")
		}
		return "IS NULL", nil, false
	default:
		return "=", value, false
	}
}

func (b *Builder) addWhereGroup(and bool, fn WhereFunc) *Builder {
	nested := &Builder{
		table:  b.table,
		limitN: -1,
	}
	fn(nested)
	if len(nested.wheres) == 0 {
		return b
	}
	b.wheres = append(b.wheres, whereItem{
		and:    and,
		nested: nested.wheres,
	})
	return b
}

func (b *Builder) buildSelectSQL() (string, []any) {
	cols := "*"
	if len(b.selects) > 0 {
		parts := make([]string, 0, len(b.selects))
		for _, c := range b.selects {
			if c.raw {
				parts = append(parts, c.expr)
			} else {
				parts = append(parts, quoteSelectColumn(c.expr))
			}
		}
		cols = strings.Join(parts, ", ")
	}

	var sb strings.Builder
	sb.WriteString("SELECT ")
	sb.WriteString(cols)
	sb.WriteByte(' ')

	fromSQL, args := b.buildFromClause()
	sb.WriteString(fromSQL)

	whereSQL, whereArgs := b.buildWhere()
	if whereSQL != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(whereSQL)
		args = append(args, whereArgs...)
	}
	if len(b.orders) > 0 {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(strings.Join(b.orders, ", "))
	}
	if b.limitN >= 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", b.limitN))
		if b.offsetN > 0 {
			sb.WriteString(fmt.Sprintf(" OFFSET %d", b.offsetN))
		}
	} else if b.offsetN > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT 18446744073709551615 OFFSET %d", b.offsetN))
	}
	return sb.String(), args
}

func (b *Builder) buildFromClause() (string, []any) {
	var sb strings.Builder
	sb.WriteString("FROM ")
	sb.WriteString(quoteTableRef(b.table))

	args := make([]any, 0)
	for _, j := range b.joins {
		if j.raw != "" {
			sb.WriteByte(' ')
			sb.WriteString(j.raw)
			args = append(args, j.rawArgs...)
			continue
		}
		sb.WriteByte(' ')
		sb.WriteString(j.joinType)
		sb.WriteString(" JOIN ")
		sb.WriteString(quoteTableRef(j.table))
		sb.WriteString(" ON ")
		sb.WriteString(quoteIdent(j.first))
		sb.WriteByte(' ')
		sb.WriteString(j.operator)
		sb.WriteByte(' ')
		sb.WriteString(quoteIdent(j.second))
	}
	return sb.String(), args
}

func (b *Builder) buildWhere() (string, []any) {
	return buildWhereItems(b.wheres)
}

func buildWhereItems(wheres []whereItem) (string, []any) {
	if len(wheres) == 0 {
		return "", nil
	}
	var sb strings.Builder
	var args []any
	if !writeWhereItems(&sb, &args, wheres) {
		return "", nil
	}
	return sb.String(), args
}

func writeWhereSep(sb *strings.Builder, started, and bool) {
	if !started {
		return
	}
	if and {
		sb.WriteString(" AND ")
	} else {
		sb.WriteString(" OR ")
	}
}

func writeWhereItems(sb *strings.Builder, args *[]any, wheres []whereItem) bool {
	started := false
	for _, w := range wheres {
		switch {
		case len(w.nested) > 0:
			nestedSQL, nestedArgs := buildWhereItems(w.nested)
			if nestedSQL == "" {
				continue
			}
			writeWhereSep(sb, started, w.and)
			sb.WriteByte('(')
			sb.WriteString(nestedSQL)
			sb.WriteByte(')')
			*args = append(*args, nestedArgs...)
		case w.raw != "":
			writeWhereSep(sb, started, w.and)
			sb.WriteByte('(')
			sb.WriteString(w.raw)
			sb.WriteByte(')')
			*args = append(*args, w.rawArgs...)
		case w.in:
			vals, ok := w.value.([]any)
			writeWhereSep(sb, started, w.and)
			if !ok || len(vals) == 0 {
				sb.WriteString("1 = 0")
				started = true
				continue
			}
			sb.WriteString(quoteIdent(w.column))
			sb.WriteByte(' ')
			if w.operator == "NOT IN" {
				sb.WriteString("NOT IN")
			} else {
				sb.WriteString("IN")
			}
			sb.WriteString(" (")
			for j, v := range vals {
				if j > 0 {
					sb.WriteString(", ")
				}
				sb.WriteByte('?')
				*args = append(*args, v)
			}
			sb.WriteByte(')')
		case w.between:
			pair, ok := w.value.([2]any)
			writeWhereSep(sb, started, w.and)
			if !ok {
				sb.WriteString("1 = 0")
				started = true
				continue
			}
			sb.WriteString(quoteIdent(w.column))
			sb.WriteByte(' ')
			if w.notBet {
				sb.WriteString("NOT BETWEEN")
			} else {
				sb.WriteString("BETWEEN")
			}
			sb.WriteString(" ? AND ?")
			*args = append(*args, pair[0], pair[1])
		default:
			op := w.operator
			if op == "" {
				op = "="
			}
			writeWhereSep(sb, started, w.and)
			sb.WriteString(quoteIdent(w.column))
			sb.WriteByte(' ')
			sb.WriteString(op)
			if !isNullOperator(op) {
				sb.WriteString(" ?")
				*args = append(*args, w.value)
			}
		}
		started = true
	}
	return started
}

// quoteSelectColumn 生成 SELECT 列表中的一列。
// u.id 写成 `u`.`id`，不自动加 AS。手动写了别名才保留：u.id as user_id。
func quoteSelectColumn(expr string) string {
	expr = strings.TrimSpace(expr)
	col, alias, hasAlias := splitSelectAlias(expr)
	quoted := quoteIdent(col)
	if hasAlias {
		return quoted + " AS " + quoteIdent(alias)
	}
	return quoted
}

// splitSelectAlias 拆出 "u.id as user_id" 或 "u.id user_id"
func splitSelectAlias(expr string) (col, alias string, ok bool) {
	lower := strings.ToLower(expr)
	if i := strings.LastIndex(lower, " as "); i >= 0 {
		col = strings.TrimSpace(expr[:i])
		alias = strings.TrimSpace(expr[i+4:])
		return col, alias, col != "" && alias != ""
	}
	fields := strings.Fields(expr)
	if len(fields) == 2 {
		return fields[0], fields[1], true
	}
	return expr, "", false
}

func quoteIdent(name string) string {
	name = strings.TrimSpace(name)
	if name == "*" {
		return "*"
	}
	// 允许 table.column。* 是通配符，不加反引号：u.* → `u`.*
	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		for i, p := range parts {
			parts[i] = quoteIdentPart(strings.TrimSpace(p))
		}
		return strings.Join(parts, ".")
	}
	return quoteIdentPart(name)
}

func quoteIdentPart(name string) string {
	if name == "*" {
		return "*"
	}
	if strings.Contains(name, "`") {
		name = strings.ReplaceAll(name, "`", "")
	}
	return "`" + name + "`"
}

// parseTableRef 解析表名与别名：users / users u / users as u
func parseTableRef(name string) (table, alias string) {
	name = strings.TrimSpace(name)
	fields := strings.Fields(name)
	switch {
	case len(fields) == 3 && strings.EqualFold(fields[1], "as"):
		return fields[0], fields[2]
	case len(fields) == 2:
		return fields[0], fields[1]
	default:
		return name, ""
	}
}

// baseTableName 返回不含别名的真实表名（Insert 用）
func baseTableName(name string) string {
	table, _ := parseTableRef(name)
	return table
}

// quoteTableRef 支持表名别名：users / users u / users as u
func quoteTableRef(name string) string {
	table, alias := parseTableRef(name)
	if alias == "" {
		return quoteIdent(table)
	}
	return quoteIdent(table) + " AS " + quoteIdent(alias)
}

func splitSelectFields(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
