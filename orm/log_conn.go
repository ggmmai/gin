package orm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log"
	"runtime"
	"strconv"
	"sync"

	"github.com/go-sql-driver/mysql"
)

// LogWriter 是 SQL 错误的输出位置。为空时写到标准日志。
var LogWriter io.Writer

func logSQLError(err error, sqlStr string, args any) {
	if err == nil || errors.Is(err, sql.ErrNoRows) || errors.Is(err, driver.ErrSkip) {
		return
	}
	msg := fmt.Sprintf("[orm] sql error: %v | sql: %s | args: %v\n", err, sqlStr, args)
	if buf, ok := sqlErrorBufs.Load(currentGID()); ok {
		buf.(*sqlErrorBuf).add(msg)
		return
	}
	writeSQLError(msg)
}

// DelaySQLErrorLog 把当前请求里的 SQL 错误和 log 输出留到返回的函数被调用时再写出。
// 访问日志在请求结束后才写，先留住这些内容，就能排在访问日志后面。
func DelaySQLErrorLog() func() {
	gid := currentGID()
	buf := &sqlErrorBuf{}
	sqlErrorBufs.Store(gid, buf)
	return func() {
		sqlErrorBufs.Delete(gid)
		buf.flush()
	}
}

// CaptureLog 在请求缓冲还在时收下这段日志。访问日志写完、缓冲卸下后再输出。
func CaptureLog(p []byte) bool {
	buf, ok := sqlErrorBufs.Load(currentGID())
	if !ok {
		return false
	}
	buf.(*sqlErrorBuf).add(string(p))
	return true
}

var sqlErrorBufs sync.Map

type sqlErrorBuf struct {
	mu   sync.Mutex
	msgs []string
}

func (b *sqlErrorBuf) add(msg string) {
	b.mu.Lock()
	b.msgs = append(b.msgs, msg)
	b.mu.Unlock()
}

func (b *sqlErrorBuf) flush() {
	b.mu.Lock()
	msgs := b.msgs
	b.msgs = nil
	b.mu.Unlock()
	length := len(msgs)
	for index, msg := range msgs {
		writeSQLError(msg)
		if index == length-1 {
			// 这样做的目的是确保每个请求都可以换行隔开
			writeSQLError("\n")
		}
	}
}

func writeSQLError(msg string) {
	if LogWriter != nil {
		_, _ = io.WriteString(LogWriter, msg)
		return
	}
	log.Print(msg)
}

func currentGID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	line := buf[:n]
	line = line[len("goroutine "):]
	end := 0
	for end < len(line) && line[end] >= '0' && line[end] <= '9' {
		end++
	}
	id, _ := strconv.ParseUint(string(line[:end]), 10, 64)
	return id
}

// OpenMySQL 打开 MySQL，并在查询、执行失败时把 SQL 和参数写到日志。
func OpenMySQL(dsn string) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	base, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(logConnector{base}), nil
}

type logConnector struct {
	driver.Connector
}

func (c logConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return logConn{conn}, nil
}

type logConn struct {
	driver.Conn
}

func (c logConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
	logSQLError(err, query, namedArgs(args))
	return rows, err
}

func (c logConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	res, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
	logSQLError(err, query, namedArgs(args))
	return res, err
}

func (c logConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	stmt, err := c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, query)
	logSQLError(err, query, nil)
	return stmt, err
}

func (c logConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.Conn.Prepare(query)
	logSQLError(err, query, nil)
	return stmt, err
}

func (c logConn) Ping(ctx context.Context) error {
	return c.Conn.(driver.Pinger).Ping(ctx)
}

func (c logConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}

func (c logConn) CheckNamedValue(nv *driver.NamedValue) error {
	return c.Conn.(driver.NamedValueChecker).CheckNamedValue(nv)
}

func (c logConn) ResetSession(ctx context.Context) error {
	return c.Conn.(driver.SessionResetter).ResetSession(ctx)
}

func (c logConn) IsValid() bool {
	return c.Conn.(driver.Validator).IsValid()
}

func namedArgs(args []driver.NamedValue) []any {
	if len(args) == 0 {
		return nil
	}
	out := make([]any, len(args))
	for i, arg := range args {
		out[i] = arg.Value
	}
	return out
}
