package orm

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// OrmConfig MySQL 连接配置
type OrmConfig struct {
	Host      string
	Port      int
	Username  string
	Password  string
	Database  string
	Prefix    string
	Charset   string
	Collation string
	// MaxOpenConns 最大打开连接数（含正在使用 + 空闲）。超出后新请求会等待；0 表示使用默认值 20
	MaxOpenConns int
	// MaxIdleConns 最大空闲连接数，池中保持待命、可复用的连接上限；过大占资源，过小频繁建连。0 表示默认 20
	//只表示空闲时最多留 20 条，池子不会为了凑满这个数去新建连接。
	// 空闲连接都是以前某次查询建出来、用完还回来的。程序如果一直只有 1 个请求在跑，池里就只有这 1 条，另外 19 个名额是空的。
	// 只有曾经同时用到过 20 条，空闲时才可能暂时留着 20 条，而且闲置满 1 小时后会被关掉。
	MaxIdleConns int
	// ConnMaxLifetime 单条连接最长存活时间，到点后关闭重建，避免被 MySQL wait_timeout 踢掉后变成坏连接。0 表示默认 1h
	ConnMaxLifetime time.Duration
	// ConnMaxIdleTime 单条连接在池里空闲的最长时间，超时后关闭，避免闲置太久被服务端断开。0 表示默认 1h
	ConnMaxIdleTime time.Duration
}

// Dsn 生成 MySQL 连接串
func (o *OrmConfig) getDsn() string {
	if o.Host == "" {
		o.Host = "127.0.0.1"
	}

	if o.Port <= 0 || o.Port > 65535 {
		o.Port = 3306
	}

	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local", // 如 user:pass@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=true&loc=Local
		o.Username, o.Password, o.Host, o.Port, o.Database,
	)
}

// OrmPool 是 D("table") 使用的默认连接池。OrmOpen 成功后写入。
var OrmPool *sql.DB
var OrmPrefix string

func (cfg *OrmConfig) OrmOpen() (*sql.DB, error) {
	db, err := sql.Open("mysql", cfg.getDsn())
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	} else {
		db.SetMaxOpenConns(20)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	} else {
		db.SetMaxIdleConns(20)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	} else {
		db.SetConnMaxLifetime(time.Hour)
	}
	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	} else {
		db.SetConnMaxIdleTime(time.Hour)
	}

	OrmPool = db
	OrmPrefix = cfg.Prefix
	return db, nil
}

func SetOrmPool(db *sql.DB, prefix string) {
	OrmPool = db
	OrmPrefix = prefix
}

func GetOrmPool() (*sql.DB, string) {
	return OrmPool, OrmPrefix
}

// D 使用默认连接创建查询构造器
// 支持表别名：D("user as u") / D("user u")
//
//	orm.D("user").Where("id", 1).Find()
//	orm.D("user as u").Where("u.id", 1).Find()
func D(table string) *Builder {
	if OrmPool == nil {
		panic("orm: default connection is not set, call orm.OrmOpen first")
	}

	if OrmPrefix != "" {
		table = OrmPrefix + table
	}
	return NewBuilder(OrmPool, table)
}
