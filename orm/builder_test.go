package orm

import (
	"strings"
	"testing"
)

func TestRowCapacity(t *testing.T) {
	if rowCapacity(1) != 1 || rowCapacity(10) != 10 || rowCapacity(100) != 100 || rowCapacity(scanRowCapLimit) != scanRowCapLimit {
		t.Fatal("limit within the cap should be the slice capacity")
	}
	if rowCapacity(scanRowCapLimit+1) != 0 || rowCapacity(0) != 0 || rowCapacity(-1) != 0 {
		t.Fatal("limit above the cap or unknown row count should not preallocate")
	}
}

func TestValueQuery(t *testing.T) {
	b := NewBuilder(nil, "user as u")
	b.Select("name").Limit(10)
	clone := b.Select("u.id as user_id").singleRow()
	sqlStr, _ := clone.buildSelectSQL()
	want := "SELECT `u`.`id` AS `user_id` FROM `user` AS `u` LIMIT 1"
	if sqlStr != want {
		t.Fatalf("sql=%s", sqlStr)
	}
	if b.limitN != 10 {
		t.Fatalf("original limit=%d", b.limitN)
	}
	if len(b.selects) != 1 || b.selects[0].expr != "u.id as user_id" {
		t.Fatalf("selects=%v", b.selects)
	}
}

func TestCastNumberOrString(t *testing.T) {
	type userID int64
	id, err := castNumberOrString[userID](int32(7))
	if err != nil || id != 7 {
		t.Fatalf("id=%v err=%v", id, err)
	}
	n, err := castNumberOrString[int]("18")
	if err != nil || n != 18 {
		t.Fatalf("n=%v err=%v", n, err)
	}
	s, err := castNumberOrString[string](int64(18))
	if err != nil || s != "18" {
		t.Fatalf("s=%q err=%v", s, err)
	}
	f, err := castNumberOrString[float64]("1.5")
	if err != nil || f != 1.5 {
		t.Fatalf("f=%v err=%v", f, err)
	}
	zero, err := castNumberOrString[int](nil)
	if err != nil || zero != 0 {
		t.Fatalf("zero=%v err=%v", zero, err)
	}
	if _, err := castNumberOrString[int8](int64(200)); err == nil {
		t.Fatal("expected overflow error")
	}
	if _, err := castNumberOrString[int]("abc"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestOrderByDirection(t *testing.T) {
	b := NewBuilder(nil, "user")
	b.OrderBy("id", "asc").OrderBy("name", "desc").OrderBy("age", "ASC")
	sqlStr, _ := b.buildSelectSQL()
	want := "ORDER BY `id` ASC, `name` DESC, `age` ASC"
	if !strings.Contains(sqlStr, want) {
		t.Fatalf("sql=%s", sqlStr)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for invalid direction")
			}
		}()
		NewBuilder(nil, "user").OrderBy("id", "descending")
	}()
}

func TestFindForcesLimitOne(t *testing.T) {
	b := NewBuilder(nil, "user")
	b.OrderBy("id", "desc").Limit(10).Offset(3)
	sqlStr, _ := b.singleRow().buildSelectSQL()
	if !strings.Contains(sqlStr, "ORDER BY `id` DESC LIMIT 1 OFFSET 3") {
		t.Fatalf("sql=%s", sqlStr)
	}
	if b.limitN != 10 {
		t.Fatalf("original limit=%d", b.limitN)
	}
}

func TestBuildSelectWhere(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	b.Where("id", 1).Where("age", ">", 18).OrderBy("id", "desc").Limit(10)

	sqlStr, args := b.buildSelectSQL()
	if !strings.Contains(sqlStr, "FROM `user`") {
		t.Fatalf("sql=%s", sqlStr)
	}
	if !strings.Contains(sqlStr, "`id` = ?") || !strings.Contains(sqlStr, "`age` > ?") {
		t.Fatalf("sql=%s", sqlStr)
	}
	if !strings.Contains(sqlStr, "ORDER BY `id` DESC") || !strings.Contains(sqlStr, "LIMIT 10") {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 2 || args[0] != 1 || args[1] != 18 {
		t.Fatalf("args=%v", args)
	}
}

func TestWhereOperatorWhitelist(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	b.Where("name", " like ", "%tom%")
	sqlStr, _ := b.buildSelectSQL()
	if !strings.Contains(sqlStr, "`name` LIKE ?") {
		t.Fatalf("sql=%s", sqlStr)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid operator")
		}
	}()
	b.Where("id", "; drop table", 1)
}

func TestWhereMapConds(t *testing.T) {
	b := &Builder{table: "coin", limitN: -1}
	b.Where(H{
		"id":     2,
		"instId": Eq{"BTCUSDT"},
		"age":    Gt{18},
		"score":  Lte{100},
		"name":   Like{"%bit%"},
		"status": In{[]any{1, 2}},
	})
	sqlStr, args := b.buildSelectSQL()
	want := "WHERE (`age` > ? AND `id` = ? AND `instId` = ? AND `name` LIKE ? AND `score` <= ? AND `status` IN (?, ?))"
	if !strings.Contains(sqlStr, want) {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 7 || args[0] != 18 || args[1] != 2 || args[2] != "BTCUSDT" || args[3] != "%bit%" || args[4] != 100 || args[5] != 1 || args[6] != 2 {
		t.Fatalf("args=%v", args)
	}

	b2 := &Builder{table: "user", limitN: -1}
	b2.Where(H{"deleted_at": Ne{nil}, "vip": nil, "removed_at": Is{Value: nil}})
	sql2, args2 := b2.buildSelectSQL()
	if !strings.Contains(sql2, "`deleted_at` IS NOT NULL") || !strings.Contains(sql2, "`vip` IS NULL") || !strings.Contains(sql2, "`removed_at` IS NULL") {
		t.Fatalf("sql=%s", sql2)
	}
	if len(args2) != 0 {
		t.Fatalf("args=%v", args2)
	}
}

func TestWhereSliceCond(t *testing.T) {
	b := &Builder{table: "coin", limitN: -1}
	b.Where(H{
		"id":     2,
		"instId": []any{"between", 1, 10},
		"ctVal":  []any{">", 10},
		"status": []any{"in", 1, 2},
	})
	sqlStr, args := b.buildSelectSQL()
	want := "WHERE (`ctVal` > ? AND `id` = ? AND `instId` BETWEEN ? AND ? AND `status` IN (?, ?))"
	if !strings.Contains(sqlStr, want) {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 6 || args[0] != 10 || args[1] != 2 || args[2] != 1 || args[3] != 10 || args[4] != 1 || args[5] != 2 {
		t.Fatalf("args=%v", args)
	}
}

func TestCondSliceSkipsScalars(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	b.Where("id", "in", []int{1, 2}).Where("blob", []byte{0x01, 0x02})
	sqlStr, args := b.buildSelectSQL()
	if !strings.Contains(sqlStr, "`id` IN (?, ?)") || !strings.Contains(sqlStr, "`blob` = ?") {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 3 || args[0] != 1 || args[1] != 2 {
		t.Fatalf("args=%v", args)
	}
	raw, ok := args[2].([]byte)
	if !ok || len(raw) != 2 || raw[0] != 0x01 || raw[1] != 0x02 {
		t.Fatalf("args=%v", args)
	}
}

func TestWhereSliceCondRejectsBadOperator(t *testing.T) {
	cases := []any{
		[]any{"drop", 1},
		[]any{1, 2},
		[]any{},
		[]any{">"},
		[]any{">", 1, 2},
		[]any{"between", 1},
		[]any{"in"},
	}
	for _, cond := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected panic for %v", cond)
				}
			}()
			b := &Builder{table: "coin", limitN: -1}
			b.Where(H{"id": cond})
		}()
	}
}

func TestOp(t *testing.T) {
	b := &Builder{table: "coin", limitN: -1}
	b.Where(H{
		"id":    Op("IN", 1, 2, 3),
		"name":  Op("<>", "a"),
		"age":   Op("BETWEEN", 1, 10),
		"flag":  Op("NOT IN", 4, 5),
		"score": Op("NOT BETWEEN", 0, 1),
	})
	sqlStr, args := b.buildSelectSQL()
	want := "WHERE (`age` BETWEEN ? AND ? AND `flag` NOT IN (?, ?) AND `id` IN (?, ?, ?) AND `name` <> ? AND `score` NOT BETWEEN ? AND ?)"
	if !strings.Contains(sqlStr, want) {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 10 || args[0] != 1 || args[1] != 10 || args[2] != 4 || args[3] != 5 || args[4] != 1 || args[5] != 2 || args[6] != 3 || args[7] != "a" || args[8] != 0 || args[9] != 1 {
		t.Fatalf("args=%v", args)
	}
	if got := Op("IS", "null"); len(got) != 1 || got[0] != "IS NULL" {
		t.Fatalf("is null=%v", got)
	}
	if got := Op("IS", "not null"); len(got) != 1 || got[0] != "IS NOT NULL" {
		t.Fatalf("is not null=%v", got)
	}

	panics := []func(){
		func() { Op[int]("IN") },
		func() { Op("BETWEEN", 1) },
		func() { Op[string]("IS") },
		func() { Op(">", 1, 2) },
		func() { Op[int]("<>") },
	}
	for _, fn := range panics {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic")
				}
			}()
			fn()
		}()
	}
}

func TestOrWhereMapIsAndGroup(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	b.Where("a", 1).OrWhere(H{"b": 2, "c": 3})
	sqlStr, args := b.buildSelectSQL()
	want := "WHERE `a` = ? OR (`b` = ? AND `c` = ?)"
	if !strings.Contains(sqlStr, want) {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 3 || args[0] != 1 || args[1] != 2 || args[2] != 3 {
		t.Fatalf("args=%v", args)
	}
}

func TestWhereBetween(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	b.Where(H{
		"age":   Between{Min: 18, Max: 30},
		"score": NotBetween{Min: 0, Max: 10},
	}).Where("price", Between{Min: 1, Max: 9})

	sqlStr, args := b.buildSelectSQL()
	want := "WHERE (`age` BETWEEN ? AND ? AND `score` NOT BETWEEN ? AND ?) AND `price` BETWEEN ? AND ?"
	if !strings.Contains(sqlStr, want) {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 6 || args[0] != 18 || args[1] != 30 || args[2] != 0 || args[3] != 10 || args[4] != 1 || args[5] != 9 {
		t.Fatalf("args=%v", args)
	}
}

func TestNormalizeValue(t *testing.T) {
	norm := func(v any, dbType string) any {
		return normalizeValue(v, columnValueKind(dbType))
	}
	if got := norm([]byte("-12"), "TINYINT"); got != int8(-12) {
		t.Fatalf("tiny %#v", got)
	}
	if got := norm([]byte("-3"), "tinyint"); got != int8(-3) {
		t.Fatalf("tiny lower %#v", got)
	}
	if got := norm([]byte("255"), "UNSIGNED TINYINT"); got != uint8(255) {
		t.Fatalf("utiny %#v", got)
	}
	if got := norm([]byte("-300"), "SMALLINT"); got != int16(-300) {
		t.Fatalf("small %#v", got)
	}
	if got := norm([]byte("60000"), "UNSIGNED SMALLINT"); got != uint16(60000) {
		t.Fatalf("usmall %#v", got)
	}
	if got := norm([]byte("1"), "INT"); got != int32(1) {
		t.Fatalf("int %#v", got)
	}
	if got := norm([]byte("7"), " integer "); got != int32(7) {
		t.Fatalf("integer %#v", got)
	}
	if got := norm([]byte("4000000000"), "UNSIGNED INT"); got != uint32(4000000000) {
		t.Fatalf("uint %#v", got)
	}
	if got := norm([]byte("1"), "BIGINT"); got != int64(1) {
		t.Fatalf("bigint %#v", got)
	}
	if got := norm([]byte("1"), "UNSIGNED BIGINT"); got != uint64(1) {
		t.Fatalf("ubig %#v", got)
	}
	if got := norm([]byte("18446744073709551615"), "UNSIGNED BIGINT"); got != ^uint64(0) {
		t.Fatalf("big %#v", got)
	}
	if got := norm([]byte("2026"), "YEAR"); got != uint16(2026) {
		t.Fatalf("year %#v", got)
	}
	if got := norm([]byte("1.5"), "FLOAT"); got != float32(1.5) {
		t.Fatalf("float %#v", got)
	}
	if got := norm([]byte("1.5"), "DOUBLE"); got != float64(1.5) {
		t.Fatalf("double %#v", got)
	}
	if got := norm([]byte("10.50"), "DECIMAL"); got != "10.50" {
		t.Fatalf("decimal %#v", got)
	}
	if got := norm([]byte("Ann"), "VARCHAR"); got != "Ann" {
		t.Fatalf("text %#v", got)
	}
	if got := norm([]byte{0x01}, "BIT"); got != int64(1) {
		t.Fatalf("bit %#v", got)
	}
	raw := []byte{0xff, 0x00}
	if got, ok := norm(raw, "BLOB").([]byte); !ok || string(got) != string(raw) {
		t.Fatalf("blob %#v", got)
	}
	if norm(nil, "INT") != nil {
		t.Fatal("nil")
	}
}

func TestColumnResultKey(t *testing.T) {
	if got := columnResultKey("c.id"); got != "id" {
		t.Fatalf("got %s", got)
	}
	if got := columnResultKey("db.coin.id"); got != "id" {
		t.Fatalf("got %s", got)
	}
	if got := columnResultKey("instId"); got != "instId" {
		t.Fatalf("got %s", got)
	}
}

func TestMappedFieldName(t *testing.T) {
	selects := []selectColumn{
		{expr: "u.id"},
		{expr: "u.name as user_name"},
		{expr: "c.email AS mail"},
		{expr: "COUNT(*) AS total", raw: true},
	}
	if got := mappedFieldName(0, "id", selects); got != "id" {
		t.Fatalf("got %s", got)
	}
	if got := mappedFieldName(1, "name", selects); got != "user_name" {
		t.Fatalf("got %s", got)
	}
	if got := mappedFieldName(2, "email", selects); got != "mail" {
		t.Fatalf("got %s", got)
	}
	if got := mappedFieldName(3, "total", selects); got != "total" {
		t.Fatalf("got %s", got)
	}
	if got := mappedFieldName(4, "c.status", nil); got != "status" {
		t.Fatalf("got %s", got)
	}
	star := []selectColumn{{expr: "u.*"}}
	if got := mappedFieldName(0, "id", star); got != "id" {
		t.Fatalf("got %s", got)
	}
	if got := mappedFieldName(1, "name", star); got != "name" {
		t.Fatalf("got %s", got)
	}

	// u.* 展开后，后面的别名不能占到 name 那一列
	mixed := []selectColumn{
		{expr: "u.*"},
		{expr: "c.id as contact_id"},
	}
	if got := mappedFieldName(0, "id", mixed); got != "id" {
		t.Fatalf("got %s", got)
	}
	if got := mappedFieldName(1, "name", mixed); got != "name" {
		t.Fatalf("got %s", got)
	}
	if got := mappedFieldName(2, "contact_id", mixed); got != "contact_id" {
		t.Fatalf("got %s", got)
	}

	beforeStar := []selectColumn{
		{expr: "c.id as contact_id"},
		{expr: "u.*"},
	}
	if got := mappedFieldName(0, "contact_id", beforeStar); got != "contact_id" {
		t.Fatalf("got %s", got)
	}
	if got := mappedFieldName(1, "id", beforeStar); got != "id" {
		t.Fatalf("got %s", got)
	}
	if got := mappedFieldName(2, "name", beforeStar); got != "name" {
		t.Fatalf("got %s", got)
	}
}

func TestWhereNilIsNull(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	b.Where("deleted_at", nil).Where("name", "!=", nil)
	sqlStr, args := b.buildSelectSQL()
	if !strings.Contains(sqlStr, "`deleted_at` IS NULL") {
		t.Fatalf("sql=%s", sqlStr)
	}
	if !strings.Contains(sqlStr, "`name` IS NOT NULL") {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 0 {
		t.Fatalf("IS NULL should bind no args, got %v", args)
	}

	b2 := &Builder{table: "user", limitN: -1}
	b2.Where("deleted_at", "is null", 1) // 显式 IS NULL，忽略值
	sql2, args2 := b2.buildSelectSQL()
	if !strings.Contains(sql2, "`deleted_at` IS NULL") || len(args2) != 0 {
		t.Fatalf("sql=%s args=%v", sql2, args2)
	}
}

func TestNormalizeOperator(t *testing.T) {
	cases := map[string]string{
		"=": "=", "!=": "!=", "<>": "<>", ">": ">", "<": "<", ">=": ">=", "<=": "<=",
		"LIKE": "LIKE", " not  like ": "NOT LIKE",
		"is null": "IS NULL", "IS NOT NULL": "IS NOT NULL",
	}
	for in, want := range cases {
		got, ok := normalizeOperator(in)
		if !ok || got != want {
			t.Fatalf("normalizeOperator(%q)=%q ok=%v want %q", in, got, ok, want)
		}
	}
	if _, ok := normalizeOperator("between"); ok {
		t.Fatal("between should not be allowed as operator")
	}
}

func TestWhereInBetweenArgs(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	b.Where("id", "in", []any{1, 2}).OrWhere("age", "between", 1, 10)
	sqlStr, args := b.buildSelectSQL()
	want := "WHERE `id` IN (?, ?) OR `age` BETWEEN ? AND ?"
	if !strings.Contains(sqlStr, want) {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 4 || args[0] != 1 || args[1] != 2 || args[2] != 1 || args[3] != 10 {
		t.Fatalf("args=%v", args)
	}

	b2 := &Builder{table: "user", limitN: -1}
	b2.Where("id", "not in", []any{3}).Where("age", "not between", 1, 10)
	sql2, args2 := b2.buildSelectSQL()
	if !strings.Contains(sql2, "`id` NOT IN (?)") || !strings.Contains(sql2, "`age` NOT BETWEEN ? AND ?") {
		t.Fatalf("sql=%s", sql2)
	}
	if len(args2) != 3 || args2[0] != 3 || args2[1] != 1 || args2[2] != 10 {
		t.Fatalf("args=%v", args2)
	}

	b3 := &Builder{table: "user", limitN: -1}
	b3.Where("age", "between", []any{1, 10})
	sql3, _ := b3.buildSelectSQL()
	if !strings.Contains(sql3, "`age` BETWEEN ? AND ?") {
		t.Fatalf("sql=%s", sql3)
	}
}

func TestWhereIsNull(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	b.Where("age", "is", "null").OrWhere("name", "is", "not null")
	sqlStr, args := b.buildSelectSQL()
	want := "WHERE `age` IS NULL OR `name` IS NOT NULL"
	if !strings.Contains(sqlStr, want) {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 0 {
		t.Fatalf("args=%v", args)
	}

	b2 := &Builder{table: "user", limitN: -1}
	b2.Where("age", "IS", " Not  Null ")
	sql2, args2 := b2.buildSelectSQL()
	if !strings.Contains(sql2, "`age` IS NOT NULL") || len(args2) != 0 {
		t.Fatalf("sql=%s args=%v", sql2, args2)
	}
}

func TestWhereInAndOrWhere(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	b.WhereIn("id", []int{1, 2, 3}).OrWhere("status", 0)
	sqlStr, args := b.buildSelectSQL()
	if !strings.Contains(sqlStr, "`id` IN (?, ?, ?)") {
		t.Fatalf("sql=%s", sqlStr)
	}
	if !strings.Contains(sqlStr, "OR `status` = ?") {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 4 {
		t.Fatalf("args=%v", args)
	}

	b2 := &Builder{table: "user", limitN: -1}
	b2.WhereIn("name", []string{"a", "b"})
	sql2, args2 := b2.buildSelectSQL()
	if !strings.Contains(sql2, "`name` IN (?, ?)") {
		t.Fatalf("sql=%s", sql2)
	}
	if len(args2) != 2 || args2[0] != "a" || args2[1] != "b" {
		t.Fatalf("args=%v", args2)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for nil WhereIn")
			}
		}()
		var ids []int
		NewBuilder(nil, "user").WhereIn("id", ids)
	}()
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for empty WhereIn")
			}
		}()
		NewBuilder(nil, "user").WhereIn("id", []int{})
	}()
}

func TestStepColumnSQL(t *testing.T) {
	b := NewBuilder(nil, "user as u")
	b.Where("u.id", 1)

	sqlStr, args, err := b.stepSQL("increment", "+", "u.age", 1)
	if err != nil {
		t.Fatal(err)
	}
	want := "UPDATE `user` AS `u` SET `u`.`age` = `u`.`age` + ? WHERE `u`.`id` = ?"
	if sqlStr != want {
		t.Fatalf("got %s", sqlStr)
	}
	if len(args) != 2 || args[0] != 1 || args[1] != 1 {
		t.Fatalf("args=%v", args)
	}

	sqlStr, args, err = b.stepSQL("decrement", "-", "score", float64(0.5))
	if err != nil {
		t.Fatal(err)
	}
	want = "UPDATE `user` AS `u` SET `score` = `score` - ? WHERE `u`.`id` = ?"
	if sqlStr != want {
		t.Fatalf("got %s", sqlStr)
	}
	if len(args) != 2 || args[0] != 0.5 {
		t.Fatalf("args=%v", args)
	}

	requireStep(b, int(2))
	requireStep(b, int8(2))
	requireStep(b, int16(2))
	requireStep(b, int32(2))
	requireStep(b, int64(2))
	requireStep(b, uint(2))
	requireStep(b, uint8(2))
	requireStep(b, uint16(2))
	requireStep(b, uint32(2))
	requireStep(b, uint64(2))
	requireStep(b, float32(1.5))
	requireStep(b, float64(1.5))

	b2 := NewBuilder(nil, "user")
	if _, _, err := b2.stepSQL("increment", "+", "age", 1); err == nil {
		t.Fatal("expected where error")
	}
	if _, _, err := b2.stepSQL("decrement", "-", "age", 1); err == nil {
		t.Fatal("expected where error")
	}

	b.OrderBy("u.id", "desc").Limit(1)
	sqlStr, _, err = b.stepSQL("decrement", "-", "score", float64(0.5))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlStr, "ORDER BY `u`.`id` DESC LIMIT 1") {
		t.Fatalf("sql=%s", sqlStr)
	}
	b.Offset(5)
	if _, _, err := b.stepSQL("increment", "+", "age", 1); err == nil {
		t.Fatal("expected offset error")
	}
}

func TestInsertSQL(t *testing.T) {
	b := NewBuilder(nil, "user as u")
	sqlStr, args, err := b.insertSQL([]map[string]any{
		{"name": "ann", "age": 20},
		{"name": "bob", "age": 21},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "INSERT INTO `user` (`age`, `name`) VALUES (?, ?), (?, ?)"
	if sqlStr != want {
		t.Fatalf("got %s", sqlStr)
	}
	if len(args) != 4 || args[0] != 20 || args[1] != "ann" || args[2] != 21 || args[3] != "bob" {
		t.Fatalf("args=%v", args)
	}

	if _, _, err := b.insertSQL(nil); err == nil {
		t.Fatal("expected empty data error")
	}
	if _, _, err := b.insertSQL([]map[string]any{{"name": "ann"}, {"age": 1}}); err == nil {
		t.Fatal("expected column mismatch error")
	}
}

func TestExistsSQL(t *testing.T) {
	b := NewBuilder(nil, "user")
	b.Where("status", 0).OrderBy("id", "desc").Limit(10)
	sqlStr, args := b.existsSQL()
	want := "SELECT 1 FROM `user` WHERE `status` = ? LIMIT 1"
	if sqlStr != want {
		t.Fatalf("got %s", sqlStr)
	}
	if len(args) != 1 || args[0] != 0 {
		t.Fatalf("args=%v", args)
	}
	if b.limitN != 10 {
		t.Fatalf("original limit=%d", b.limitN)
	}

	b2 := NewBuilder(nil, "user as u")
	b2.LeftJoin("contact as c", "u.id", "=", "c.user_id").Where("c.id", 1)
	sql2, args2 := b2.existsSQL()
	want2 := "SELECT 1 FROM `user` AS `u` LEFT JOIN `contact` AS `c` ON `u`.`id` = `c`.`user_id` WHERE `c`.`id` = ? LIMIT 1"
	if sql2 != want2 {
		t.Fatalf("got %s", sql2)
	}
	if len(args2) != 1 || args2[0] != 1 {
		t.Fatalf("args=%v", args2)
	}
}

func TestDeleteOrderLimit(t *testing.T) {
	b := NewBuilder(nil, "user")
	b.Where("status", 0).OrderBy("id", "asc").Limit(2)
	sqlStr, args, err := b.deleteSQL()
	if err != nil {
		t.Fatal(err)
	}
	want := "DELETE FROM `user` WHERE `status` = ? ORDER BY `id` ASC LIMIT 2"
	if sqlStr != want {
		t.Fatalf("got %s", sqlStr)
	}
	if len(args) != 1 || args[0] != 0 {
		t.Fatalf("args=%v", args)
	}
	b.Offset(3)
	if _, _, err := b.deleteSQL(); err == nil {
		t.Fatal("expected offset error")
	}
}

func TestUpdateOrderLimit(t *testing.T) {
	b := NewBuilder(nil, "user")
	b.Where("status", 0).OrderBy("id", "desc").Limit(1)
	sqlStr, args, err := b.updateSQL(map[string]any{"age": 10})
	if err != nil {
		t.Fatal(err)
	}
	want := "UPDATE `user` SET `age` = ? WHERE `status` = ? ORDER BY `id` DESC LIMIT 1"
	if sqlStr != want {
		t.Fatalf("got %s", sqlStr)
	}
	if len(args) != 2 || args[0] != 10 || args[1] != 0 {
		t.Fatalf("args=%v", args)
	}

	sql2, args2, err := b.updateSQL(map[string]any{"name": "ann", "age": 10})
	if err != nil {
		t.Fatal(err)
	}
	want2 := "UPDATE `user` SET `age` = ?, `name` = ? WHERE `status` = ? ORDER BY `id` DESC LIMIT 1"
	if sql2 != want2 {
		t.Fatalf("got %s", sql2)
	}
	if len(args2) != 3 || args2[0] != 10 || args2[1] != "ann" || args2[2] != 0 {
		t.Fatalf("args=%v", args2)
	}

	b.Offset(2)
	if _, _, err := b.updateSQL(map[string]any{"age": 10}); err == nil {
		t.Fatal("expected offset error")
	}
}

func requireStep[T number](b *Builder, amount T) {
	var (
		_ func(string, T) (int64, error) = b.Increment
		_ func(string, T) (int64, error) = b.Decrement
	)
	_ = amount
}

func TestUpdateRequiresWhere(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	_, err := b.Update(map[string]any{"age": 10})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTruncateSQL(t *testing.T) {
	b := NewBuilder(nil, "user")
	sqlStr, err := b.truncateSQL()
	if err != nil {
		t.Fatal(err)
	}
	if sqlStr != "TRUNCATE TABLE `user`" {
		t.Fatalf("sql=%s", sqlStr)
	}

	b2 := NewBuilder(nil, "db.user as u")
	sql2, err := b2.truncateSQL()
	if err != nil {
		t.Fatal(err)
	}
	if sql2 != "TRUNCATE TABLE `db`.`user`" {
		t.Fatalf("sql=%s", sql2)
	}

	b3 := NewBuilder(nil, "   ")
	if _, err := b3.truncateSQL(); err == nil {
		t.Fatal("expected error for empty table")
	}
}

func TestDeleteRequiresWhere(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	_, err := b.Delete()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestQuoteIdent(t *testing.T) {
	if got := quoteIdent("user.name"); got != "`user`.`name`" {
		t.Fatalf("got %s", got)
	}
	if got := quoteIdent("us`er"); got != "`user`" {
		t.Fatalf("got %s", got)
	}
	if got := quoteIdent("u.*"); got != "`u`.*" {
		t.Fatalf("got %s", got)
	}
}

func TestQuoteSelectColumn(t *testing.T) {
	if got := quoteSelectColumn("u.id"); got != "`u`.`id`" {
		t.Fatalf("got %s", got)
	}
	if got := quoteSelectColumn("db.coin.id"); got != "`db`.`coin`.`id`" {
		t.Fatalf("got %s", got)
	}
	if got := quoteSelectColumn("u.id as user_id"); got != "`u`.`id` AS `user_id`" {
		t.Fatalf("got %s", got)
	}
	if got := quoteSelectColumn("id"); got != "`id`" {
		t.Fatalf("got %s", got)
	}
	if got := quoteSelectColumn("u.*"); got != "`u`.*" {
		t.Fatalf("got %s", got)
	}
	if got := quoteSelectColumn("db.user.*"); got != "`db`.`user`.*" {
		t.Fatalf("got %s", got)
	}
	if got := quoteIdent("*"); got != "*" {
		t.Fatalf("got %s", got)
	}
}

func TestWhereCallback(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	b.Where("deleted", 0).Where(func(q *Builder) {
		q.Where("status", 1).OrWhere("vip", 1)
	}).OrWhere(func(q *Builder) {
		q.Where("id", 9)
	})

	sqlStr, args := b.buildSelectSQL()
	want := "WHERE `deleted` = ? AND (`status` = ? OR `vip` = ?) OR (`id` = ?)"
	if !strings.Contains(sqlStr, want) {
		t.Fatalf("sql=%s\nwant contains %s", sqlStr, want)
	}
	if len(args) != 4 || args[0] != 0 || args[1] != 1 || args[2] != 1 || args[3] != 9 {
		t.Fatalf("args=%v", args)
	}
}

func TestTableAlias(t *testing.T) {
	b := NewBuilder(nil, "user as u")
	b.Select("u.id", "u.name").Where("u.status", 1)
	sqlStr, args := b.buildSelectSQL()
	want := "SELECT `u`.`id`, `u`.`name` FROM `user` AS `u` WHERE `u`.`status` = ?"
	if sqlStr != want {
		t.Fatalf("got %s\nwant %s", sqlStr, want)
	}
	if len(args) != 1 || args[0] != 1 {
		t.Fatalf("args=%v", args)
	}

	b2 := NewBuilder(nil, "user u")
	sql2, _ := b2.buildSelectSQL()
	if !strings.Contains(sql2, "FROM `user` AS `u`") {
		t.Fatalf("sql=%s", sql2)
	}

	if got := baseTableName("user as u"); got != "user" {
		t.Fatalf("baseTableName=%s", got)
	}
	if got := quoteTableRef("db.user as u"); got != "`db`.`user` AS `u`" {
		t.Fatalf("quoteTableRef=%s", got)
	}
}

func TestJoins(t *testing.T) {
	b := &Builder{table: "users", limitN: -1}
	b.Select("users.id", "c.email").
		Join("contacts as c", "users.id", "=", "c.user_id").
		LeftJoin("orders", "users.id", "orders.user_id").
		RightJoin("profiles p", "users.id", "=", "p.user_id").
		Where("users.status", 1)

	sqlStr, args := b.buildSelectSQL()
	wantParts := []string{
		"SELECT `users`.`id`, `c`.`email`",
		"FROM `users`",
		"INNER JOIN `contacts` AS `c` ON `users`.`id` = `c`.`user_id`",
		"LEFT JOIN `orders` ON `users`.`id` = `orders`.`user_id`",
		"RIGHT JOIN `profiles` AS `p` ON `users`.`id` = `p`.`user_id`",
		"WHERE `users`.`status` = ?",
	}
	for _, part := range wantParts {
		if !strings.Contains(sqlStr, part) {
			t.Fatalf("sql missing %q\nsql=%s", part, sqlStr)
		}
	}
	if len(args) != 1 || args[0] != 1 {
		t.Fatalf("args=%v", args)
	}
}

func TestJoinRaw(t *testing.T) {
	b := &Builder{table: "users", limitN: -1}
	b.JoinRaw("LEFT JOIN contacts AS c ON users.id = c.user_id AND c.status = ?", 1)
	sqlStr, args := b.buildSelectSQL()
	if !strings.Contains(sqlStr, "LEFT JOIN contacts AS c ON users.id = c.user_id AND c.status = ?") {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 1 || args[0] != 1 {
		t.Fatalf("args=%v", args)
	}
}

func TestSelectCommaAndSelectRaw(t *testing.T) {
	b := &Builder{table: "coin", limitN: 10}
	b.Select("id,instId").SelectRaw("ctVal").Limit(10)
	sqlStr, _ := b.buildSelectSQL()
	want := "SELECT `id`, `instId`, ctVal FROM `coin` LIMIT 10"
	if sqlStr != want {
		t.Fatalf("got %s\nwant %s", sqlStr, want)
	}
}

func TestDebugDefaultFalse(t *testing.T) {
	b := NewBuilder(nil, "user")
	if b.debug {
		t.Fatal("debug should default to false")
	}
	b.Debug()
	if !b.debug {
		t.Fatal("Debug() should enable debug")
	}
}

func TestNestedWhereCallback(t *testing.T) {
	b := &Builder{table: "user", limitN: -1}
	b.Where(func(q *Builder) {
		q.Where("a", 1).Where(func(q2 *Builder) {
			q2.Where("b", 2).OrWhere("c", 3)
		})
	})
	sqlStr, args := b.buildSelectSQL()
	want := "WHERE (`a` = ? AND (`b` = ? OR `c` = ?))"
	if !strings.Contains(sqlStr, want) {
		t.Fatalf("sql=%s", sqlStr)
	}
	if len(args) != 3 {
		t.Fatalf("args=%v", args)
	}
}
