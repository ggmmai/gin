# orm

面向 MySQL 的 Go 查询构造器。链式写条件，参数用 `?` 绑定。查询结果可以扫进 `map[string]any`，也可以扫进结构体。

模块名是 `orm`，在本仓库里直接 `import "orm"`。

## 连接

```go
db, err := (&orm.OrmConfig{
    Host:     "127.0.0.1",
    Port:     3306,
    Username: "root",
    Password: "secret",
    Database: "app",
    Prefix:   "wy_",   // 表前缀，可空
    Debug:    false,   // 见下方「调试」
}).OrmOpen()
```

`OrmOpen` 会 `Ping`，成功后把连接池写到 `orm.OrmPool`，表前缀写到 `orm.OrmPrefix`。之后用 `orm.D("user")` 查询。

没填的字段使用这些默认值：

| 字段 | 默认 |
| --- | --- |
| `Host` | `127.0.0.1` |
| `Port` | `3306`（小于等于 0 或大于 65535 时同样回落到 3306） |
| `MaxOpenConns` | `20` |
| `MaxIdleConns` | `20` |
| `ConnMaxLifetime` | `1h` |
| `ConnMaxIdleTime` | `1h` |

`MaxIdleConns` 只是空闲连接上限。池子不会为了凑满这个数去新建连接。空闲连接来自用完归还的连接，闲置超过 `ConnMaxIdleTime` 后关闭。

实际 DSN 固定为：

```text
user:pass@tcp(host:port)/database?charset=utf8mb4&parseTime=true&loc=Local
```

`Charset`、`Collation` 字段目前不会写进 DSN。`parseTime=true` 让 `DATE`、`DATETIME`、`TIMESTAMP` 扫成 `time.Time`。

已有 `*sql.DB` 时可以自己放进默认池：

```go
orm.SetOrmPool(db, "wy_")
pool, prefix := orm.GetOrmPool()
```

`D` 在 `OrmPool == nil` 时 panic，提示先调用 `OrmOpen`。

也可以不走默认池，直接构造：

```go
b := orm.NewBuilder(db, "user", false)
```

`NewBuilder` 的第三个参数当前没有使用，构造器上的 SQL 打印默认关闭。要看 SQL，调用 `.Debug()`。

## 表名与前缀

```go
orm.D("user")            // FROM `wy_user`          Prefix = "wy_"
orm.D("user as u")       // FROM `wy_user` AS `u`
orm.D("user u")          // 同上，空格别名也会规范成 AS
orm.D("other_db.user")   // 名字里有 . 时不加前缀
```

`Join`、`LeftJoin`、`RightJoin` 的表名同样加前缀。带 `.` 的表名保持原样。`JoinRaw` 的片段原样拼进 SQL，不加前缀，也不加反引号。

标识符会用反引号包起来，内部的 `` ` `` 会被去掉。`u.id` 写成 `` `u`.`id` ``，`u.*` 写成 `` `u`.* ``。

## 读

下面的方法都在当前构造器上执行 `SELECT`。

### Get / Find

```go
rows, err := orm.D("user").
    Where("status", 1).
    OrderBy("id", "desc").
    Limit(20).
    Offset(40).
    Get()

row, err := orm.D("user").Where("id", 1).Find()
row, err = orm.D("user").Where("id", 1).First() // Find 的别名
```

`Get` 返回 `[]map[string]any`。`Find` / `First` 返回一行；没有行时是 `nil, nil`。

`Find` 和 `First` 会复制一份构造器再查，SQL 固定 `LIMIT 1`。原来写过的 `Limit` 不会被改掉，`OrderBy` 和 `Offset` 仍会带上。

只写了 `Offset`、没写 `Limit` 时，查询会变成 `LIMIT 18446744073709551615 OFFSET n`，这是 MySQL 只偏移、不截断条数的写法。

### Value

取一列的一个值，转成数字或字符串：

```go
name, err := orm.D("user").Where("id", 1).Value[string]("name")
age, err := orm.D("user").Where("id", 1).Value[int]("age")
id, err := orm.D("user").Where("id", 1).Value[int64]("u.id")
```

允许的类型是 `string` 以及有符号、无符号整数和浮点数，包含基于这些类型定义的新类型。返回的是指针。

- 没有这一行：`nil` 和 `sql.ErrNoRows`
- 这一列是 `NULL`：该类型的零值，错误为 `nil`
- 放不进目标类型（溢出、无法解析）时返回错误

`Value` 会清掉已有的 `Select`，只查这一列，并 `LIMIT 1`。这一步改的是当前构造器，不只是临时副本。

### Count / Exists

```go
n, err := orm.D("user").Where("status", 1).Count()
ok, err := orm.D("user").Where("email", "a@b.c").Exists()
```

`Count` 生成 `SELECT COUNT(*)`，不使用 `Select`、`OrderBy`、`Limit`。`Join` 和 `Where` 会保留。

`Exists` 生成 `SELECT 1 ... LIMIT 1`，找到一行就停。已写的 `Limit`、`OrderBy` 不参与这次判断，构造器上的值保持不变。没有行时返回 `false, nil`。

### 扫进结构体

```go
type Coin struct {
    ID     int32          `orm:"id"`
    InstID string         `orm:"instId"`
    Name   string         // 没有标签时用字段名 Name
    Title  sql.NullString `orm:"title"`
    Skip   int            `orm:"-"`
}

list, err := orm.D("coin").Select("id,instId").Limit(10).GetInto[Coin]()
one, err := orm.D("coin").Where("id", 1).FindInto[Coin]()
```

类型参数必须是结构体本身，不能是 `*Coin`。

- `orm` 标签对应结果列名。标签里逗号之前的部分才是列名。
- 没有标签时用字段名，区分大小写。
- `orm:"-"` 跳过。未导出字段跳过。
- 匿名嵌入的结构体、且自身没有 `orm` 标签时，会展开进去。同一个列名先出现的字段生效。
- 没有对应列的字段保持零值。结果里多出来的列会被忽略。
- `FindInto` 没有行时返回 `nil, nil`，并固定 `LIMIT 1`，不改原构造器的 `Limit`。
- `NULL` 不能写入 `int`、`string` 这类非指针字段，扫描会返回错误。指针字段写成 `nil`。实现了 `sql.Scanner` 的字段（如 `sql.NullString`）由该字段自己处理 `NULL`。

结构体字段布局会按类型缓存。

## 写

### Insert

```go
id, err := orm.D("user").Insert(orm.H{
    "name": "ann",
    "age":  20,
})

id, err = orm.D("user").Insert(
    orm.H{"name": "ann", "age": 20},
    orm.H{"name": "bob", "age": 21},
)
```

多行合成一条 `INSERT`，只往返一次。每一行的字段名必须和第一行完全一致。列名按字母序写入 SQL。返回第一条生成的自增 id（`LastInsertId`）。

表名带别名时，插入用去掉别名后的表：`D("user as u").Insert(...)` 写入 `` `user` ``。空数据、空列名、行列不一致都会返回错误。

### Update / Delete

```go
n, err := orm.D("user").Where("id", 1).Update(orm.H{"age": 10, "name": "ann"})
n, err = orm.D("user").Where("status", 0).OrderBy("id", "asc").Limit(1).Delete()
```

两者都必须带 `Where`。没有条件时直接返回错误，不会生成全表更新或全表删除。

`SET` 的列按字母序排列。`OrderBy` 和 `Limit` 会写进 `UPDATE` / `DELETE`。MySQL 这两类语句没有 `OFFSET`，调用过 `Offset` 会返回错误。返回影响行数。

### Increment / Decrement

```go
n, err := orm.D("user").Where("id", 1).Increment("age", 2)
n, err = orm.D("user").Where("id", 1).Decrement("score", 0.5)
```

步长必须传入，类型是整数或浮点数（含 `int8` 到 `int64`、`uint8` 到 `uint64`、`float32`、`float64`，以及基于它们的定义类型）。同样必须有 `Where`，可以带 `OrderBy` 和 `Limit`，不能带 `Offset`。

生成的语句形如 `` SET `age` = `age` + ? ``。

### Truncate

```go
err := orm.D("user").Truncate()
```

执行 `TRUNCATE TABLE`，重置自增。不看 `Where`，清空的是 `D` 指定的那张表。别名会被去掉。

## 条件

`Where` 追加 `AND`，`OrWhere` 追加 `OR`。第一个条件前面不会再加 `AND` / `OR`，所以第一条写成 `OrWhere` 时效果和 `Where` 相同。

运算符只接受白名单，不能把任意字符串拼进 SQL。比较运算符大小写和多余空格会被整理，例如 `" not  like "` 会变成 `NOT LIKE`。

### 两参数：相等

```go
orm.D("user").Where("id", 1)                 // `id` = ?
orm.D("user").Where("status", 1).Where("vip", 0)
```

### 三参数：运算符

```go
orm.D("user").Where("age", ">", 18)
orm.D("user").Where("name", "like", "%ann%")
orm.D("user").Where("name", "not like", "%bot%")
orm.D("user").Where("id", "!=", 1)           // 也可用 "<>"
orm.D("user").Where("id", "in", []any{1, 2, 3})
orm.D("user").Where("id", "not in", []any{4})
orm.D("user").Where("age", "between", []any{18, 30})
orm.D("user").Where("age", "not between", []any{0, 10})
orm.D("user").Where("deleted_at", "is", "null")
orm.D("user").Where("deleted_at", "is", "not null")
orm.D("user").Where("deleted_at", "is null", 1) // 显式 IS NULL 时忽略右侧的值
```

`in` / `not in` 的值必须是切片。`between` 也可以把两个端点直接写成第四、第五个参数，这是唯一接受四个参数的形式：

```go
orm.D("user").Where("age", "between", 18, 30)
orm.D("user").OrWhere("score", "not between", 0, 10)
```

`IS` 的标志只接受 `null` 和 `not null`（忽略大小写和多余空格）。

### nil 变成 IS NULL

```go
orm.D("user").Where("deleted_at", nil)         // `deleted_at` IS NULL
orm.D("user").Where("deleted_at", "!=", nil)   // IS NOT NULL
orm.D("user").Where("deleted_at", "<>", nil)   // IS NOT NULL
```

`>`、`<`、`LIKE` 等运算符配 `nil` 会 panic。

### WhereIn

```go
orm.D("user").WhereIn("id", []int{1, 2, 3})
orm.D("user").WhereIn("name", []string{"a", "b"})
```

元素类型与 `Value` 相同：数字或字符串，以及基于它们的定义类型。`nil` 或空切片会 panic。

### map 条件

`orm.H` 就是 `map[string]any`。一张 map 内部永远是 `AND`，列名按字母序输出，整组再用括号接到前面的条件上。

```go
orm.D("coin").Where(orm.H{
    "id":     2,                          // =
    "instId": orm.Eq{Value: "BTCUSDT"},   // =
    "age":    orm.Gt{Value: 18},          // >
    "score":  orm.Lte{Value: 100},        // <=
    "name":   orm.Like{Value: "%bit%"},   // LIKE
    "status": orm.In{Value: []any{1, 2}}, // IN
})
// WHERE (`age` > ? AND `id` = ? AND `instId` = ? AND `name` LIKE ? AND `score` <= ? AND `status` IN (?, ?))

orm.D("user").Where("a", 1).OrWhere(orm.H{"b": 2, "c": 3})
// WHERE `a` = ? OR (`b` = ? AND `c` = ?)
```

比较结构体：

| 类型 | SQL |
| --- | --- |
| `Eq{Value}` | `=` |
| `Ne{Value}` | `!=`。`Value` 为 `nil` 时是 `IS NOT NULL` |
| `Gt{Value}` | `>` |
| `Gte{Value}` | `>=` |
| `Lt{Value}` | `<` |
| `Lte{Value}` | `<=` |
| `Like{Value}` | `LIKE` |
| `NotLike{Value}` | `NOT LIKE` |
| `In{Value: []any{...}}` | `IN` |
| `Between{Min, Max}` | `BETWEEN ? AND ?` |
| `NotBetween{Min, Max}` | `NOT BETWEEN ? AND ?` |
| `Is{Value: nil}` | `IS NULL`。`Value` 不是 `nil` 会 panic |

普通值（数字、字符串、`bool`、`time.Time`、`[]byte`）表示相等。`[]byte` 按一个二进制值绑定，不会被当成 `IN` 列表。map 里写 `nil` 表示 `IS NULL`。

### 切片条件与 Op

map 的值也可以是「运算符在前」的切片：

```go
orm.D("coin").Where(orm.H{
    "ctVal":  []any{">", 10},
    "instId": []any{"between", 1, 10},
    "status": []any{"in", 1, 2},
    "flag":   []any{"is", "null"},
})
```

`[]int`、`[]string` 这类切片同样能展开。第一个元素必须是运算符字符串。运算符非法、参数个数不对会 panic。

`Op` 用来生成这种切片。类型参数只能是 `int`、`float64` 或 `string`，同一次调用里的值类型要一致：

```go
orm.D("coin").Where(orm.H{
    "id":    orm.Op("in", 1, 2, 3),
    "name":  orm.Op("<>", "a"),
    "age":   orm.Op("between", 1, 10),
    "flag":  orm.Op("not in", 4, 5),
    "gone":  orm.Op("is", "null"),       // 得到 []any{"IS NULL"}，不再带值
})
```

支持的运算符：`>` `<` `<=` `>=` `=` `!=` `<>` `LIKE` `NOT LIKE`（1 个值），`IN` `NOT IN`（至少 1 个值），`BETWEEN` `NOT BETWEEN`（2 个值），`IS`（`"null"` 或 `"not null"`）。个数不对、运算符不认识会 panic。

### 分组

```go
orm.D("user").
    Where("deleted", 0).
    Where(func(q *orm.Builder) {
        q.Where("status", 1).OrWhere("vip", 1)
    }).
    OrWhere(func(q *orm.Builder) {
        q.Where("id", 9)
    })
// WHERE `deleted` = ? AND (`status` = ? OR `vip` = ?) OR (`id` = ?)
```

回调可以套回调。回调里没有写出任何条件时，这一组会被丢掉。`WhereFunc` 就是 `func(*Builder)`。

### WhereRaw

原始片段会用括号包起来，参数仍用 `?`：

```go
orm.D("user").WhereRaw("age > ? AND score > ?", 18, 60)
// AND (age > ? AND score > ?)
```

`JoinRaw`、`WhereRaw`、`SelectRaw` 里的文字会原样进入 SQL。运算符白名单覆盖不到这些入口。

## 字段

不调用 `Select` 时是 `SELECT *`。

```go
orm.D("user").Select("id", "name")
orm.D("user").Select("id,instId")          // 逗号会拆开
orm.D("user as u").Select("u.id", "u.name")
orm.D("user").Select("u.id as user_id")    // 只有手写别名，SQL 才带 AS
```

`Select` 会清掉之前的字段列表。`SelectRaw` 是往列表后面追加，不加反引号，也不按逗号拆分：

```go
orm.D("coin").Select("id,instId").SelectRaw("COUNT(*) AS total")
// SELECT `id`, `instId`, COUNT(*) AS total
```

两个注意点：

- `SelectRaw("u.id, c.email")` 是一整段表达式，不会变成两列。要两列就分两次调用，或改用 `Select`。
- 表达式里没有 ` as `、但又恰好是两个词时，第二个词会被当成别名。`SelectRaw("DISTINCT u.id")` 的结果键会是 `u.id`。需要自己写别名：`SelectRaw("DISTINCT u.id as id")`。

### 结果 map 的键

| 写法 | SQL | map 键 |
| --- | --- | --- |
| `Select("u.id")` | `` `u`.`id` `` | `id` |
| `Select("u.id as user_id")` | `` `u`.`id` AS `user_id` `` | `user_id` |
| `Select("c.email AS mail")` | `` `c`.`email` AS `mail` `` | `mail` |
| `SelectRaw("COUNT(*) AS total")` | 原样 | `total` |
| `Select("u.*")` | `` `u`.* `` | 用驱动返回的列名，去掉表前缀 |
| 没写 `Select` | `*` | 驱动列名，去掉表前缀 |

`u.*` 会展开成多列。从星号这一列开始，后面的列都按驱动返回的列名取键，不能再靠后面的 `as` 去对位。别名列要写在星号前面。

结构体扫描用的是同一套列名。`orm:"user_id"` 对的是上面的 map 键，不是原始表达式。

## 连接

```go
orm.D("users").
    Select("users.id", "c.email").
    Join("contacts as c", "users.id", "=", "c.user_id").
    LeftJoin("orders", "users.id", "orders.user_id"). // 两个列名时运算符默认 =
    RightJoin("profiles p", "users.id", "=", "p.user_id").
    Where("users.status", 1)
```

`Join` 是 `INNER JOIN`。运算符走同一份白名单，不能用 `IS NULL` 这种没有右侧列的写法。表名、列名都会加反引号并套用前缀。参数个数不是 2 或 3、列名为空、运算符不合法会 panic。

原样连接：

```go
orm.D("users").JoinRaw(
    "LEFT JOIN contacts AS c ON users.id = c.user_id AND c.status = ?",
    1,
)
```

## 结果里的 Go 类型

`Get` / `Find` 按列的数据库类型把驱动返回的字节转成具体类型。转不了就保留字符串。

| MySQL 类型 | Go 类型 |
| --- | --- |
| `TINYINT` | `int8` |
| `UNSIGNED TINYINT` | `uint8` |
| `SMALLINT` | `int16` |
| `UNSIGNED SMALLINT`、`YEAR` | `uint16` |
| `MEDIUMINT`、`INT`、`INTEGER` | `int32` |
| 上面三者的 `UNSIGNED` | `uint32` |
| `BIGINT` | `int64` |
| `UNSIGNED BIGINT` | `uint64` |
| `FLOAT` | `float32` |
| `DOUBLE` | `float64` |
| `BIT` | `int64`；超过 `MaxInt64` 时为十进制字符串 |
| `BINARY`、`VARBINARY`、各种 `BLOB`、`GEOMETRY`、`VECTOR` | `[]byte` |
| `DECIMAL`、`VARCHAR`、`TEXT`、`JSON` 以及其余类型 | `string` |
| `DATE` / `DATETIME` / `TIMESTAMP` | `time.Time` |
| `NULL` | `nil` |

`DECIMAL` 保持字符串，避免浮点丢掉小数。

## 上下文、调试、错误日志

```go
ctx, cancel := context.WithTimeout(context.Background(), time.Second)
defer cancel()

row, err := orm.D("user").WithContext(ctx).Debug().Where("id", 1).Find()
```

`.Debug()` 只打开当前这条构造器。打印格式：

```text
[orm debug] sql: SELECT * FROM `user` WHERE `id` = ? | args: [1]
```

`OrmConfig.Debug` 会写入 `orm.OrmDebug`，`GetOrmDebug` / `SetOrmDebug` 可以读写它。`NewBuilder` 目前固定把构造器的 `debug` 设为 `false`，所以只改配置不会打印 SQL，还需要 `.Debug()`。

`OrmOpen` 用的是普通 MySQL 驱动。查询失败时，`Get`、`Find`、`Insert`、`Update`、`Delete`、`Increment`、`Decrement`、`Truncate` 返回的错误里带有 SQL。`Count` 和 `Exists` 返回驱动原始错误。

如果希望连接在驱动层记录失败 SQL，用 `OpenMySQL` 打开，再放进默认池：

```go
db, err := orm.OpenMySQL(dsn)
orm.SetOrmPool(db, "")
```

失败时输出：

```text
[orm] sql error: ... | sql: ... | args: [...]
```

`sql.ErrNoRows` 不记。把 `orm.LogWriter` 设成某个 `io.Writer` 后，错误写到那里；为空时写到标准日志。

请求日志若在请求结束才写，可以先把 SQL 错误按当前 goroutine 攒住，访问日志写完再刷出：

```go
flush := orm.DelaySQLErrorLog()
defer flush()
```

同一次请求里要在同一条 goroutine 上调用返回的函数。

## 构造器怎么改自己

链式调用改的是同一个 `*Builder`，没有每次克隆。

- `Where`、`Join`、`OrderBy`、`SelectRaw` 会往上追加。
- `Select` 会替换整个字段列表。
- `Limit`、`Offset`、`WithContext`、`Debug` 会覆盖对应字段。
- `Find`、`FindInto`、`Exists` 查询时使用副本改 `LIMIT`，外部的 `Limit` 不变。
- `Value` 会改掉外部的 `Select`。

同一个构造器不要并发使用。一次请求里重新 `orm.D("user")` 会拿到新的构造器。

## 会 panic 的调用

这些在拼 SQL 时就会 panic，不会变成数据库错误：

- 还没 `OrmOpen` 就调用 `D`
- `Where` 参数个数不对、列名不是字符串、运算符不在白名单
- `WhereIn` 传入 `nil` 或空切片
- `Op` 的运算符或参数个数不合法
- `OrderBy` 的方向不是 `ASC` / `DESC`
- `Join` 的参数个数、列名或运算符不合法
- `JoinRaw` 传入空字符串
- `Is` 的值不是 `nil`

全表 `Update`、`Delete`、`Increment`、`Decrement`，以及它们带了 `Offset`，返回的是 `error`，不会 panic。
