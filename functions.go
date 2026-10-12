package gin

import (
	"net/http"
	"path"
	"sync"

	"github.com/gin-gonic/gin/orm"
)

type Runtimes struct {
	Pt         int
	Module     string
	Controller string
	Action     string
	HomeAdmin  string
	RootPath   string
}

type response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

func (c *Context) Result(code int, msg string, value ...any) {
	var data any
	if len(value) == 0 {
		data = nil
	} else {
		data = value[0]
	}

	c.JSON(http.StatusOK, &response{
		code,
		msg,
		data,
	})
}

func (c *Context) Ok(msg string, value ...any) {
	var data any
	if len(value) == 0 {
		data = nil
	} else {
		data = value[0]
	}

	c.JSON(http.StatusOK, &response{
		0,
		msg,
		data,
	})
}

func (c *Context) Success(msg string, value ...any) {
	var data any
	if len(value) == 0 {
		data = nil
	} else {
		data = value[0]
	}

	c.JSON(http.StatusOK, &response{
		0,
		msg,
		data,
	})
}

func (c *Context) Warn(msg string, value ...any) {
	var data any
	if len(value) == 0 {
		data = nil
	} else {
		data = value[0]
	}

	c.JSON(http.StatusOK, &response{
		-1,
		msg,
		data,
	})
}

func (c *Context) Fail(msg string, value ...any) {
	var data any
	if len(value) == 0 {
		data = nil
	} else {
		data = value[0]
	}

	c.JSON(http.StatusOK, &response{
		1,
		msg,
		data,
	})
}

func StyleUrl(c *Context) string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "image")
}

func ImgUrl(c *Context) string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "image")
}

func AudioUrl(c *Context) string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "audio")
}

func VideoUrl(c *Context) string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "video")
}

func FileUrl(c *Context) string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "file")
}

func PosterUrl(c *Context) string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "poster")
}

func QrcoderUrl(c *Context) string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "qrcoder")
}

func ImgPath(c *Context) string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "image")
}

func AudioPath(c *Context) string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "audio")
}

func VideoPath(c *Context) string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "video")
}

func FilePath(c *Context) string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "file")
}

func PosterPath(c *Context) string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "poster")
}

func QrcoderPath(c *Context) string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "qrcoder")
}

func CertPath(c *Context) string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "cert")
}

type OrmBuilder = orm.OrmBuilder

func D(table string) *OrmBuilder {
	return orm.D(table)
}

func (c *Context) D(table string) *OrmBuilder {
	return orm.D(c.Runtimes.Module + "_" + table)
}

// c.Set / c.Get 的键放在 map[any]any 里，比较时类型和值都要相同。
// entityKey("config:get") 和字符串 "config:get" 不是同一个键，业务里的 c.Set("config:get", ...) 不会覆盖这里的回调。
type entityKey string

// EntityCache 返回实体缓存中间件。sync.Map 在注册时创建一次，所有请求共用。
// 每次请求再生成带上当前 Context 的回调，查询走 c.D。同一份返回值挂到多个路由组时共用同一张缓存。
func EntityCache[T any](table string) HandlerFunc {
	cache := sync.Map{}
	getKey := entityKey(table + ":get")
	delKey := entityKey(table + ":del")
	return func(c *Context) {
		c.Set(getKey, func(id int) *T {
			if value, ok := cache.Load(id); ok {
				return value.(*T)
			}
			row, err := c.D(table).Where("id", id).FindInto[T]()
			if err != nil {
				return nil
			}
			cache.Store(id, row)
			return row
		})
		c.Set(delKey, func(id int) {
			cache.Delete(id)
		})
		c.Next()
	}
}

func entityGet[T any](c *Context, table string, id int) *T {
	value, ok := c.Get(entityKey(table + ":get"))
	if !ok {
		panic("未注册实体缓存中间件: " + table)
	}
	get, ok := value.(func(int) *T)
	if !ok {
		panic("实体缓存类型不匹配: " + table)
	}
	return get(id)
}

func entityDelete(c *Context, table string, id int) {
	value, ok := c.Get(entityKey(table + ":del"))
	if !ok {
		panic("未注册实体缓存中间件: " + table)
	}
	del, ok := value.(func(int))
	if !ok {
		panic("实体缓存类型不匹配: " + table)
	}
	del(id)
}

func (c *Context) GetConfig[T any](ids ...int) *T {
	id := c.Pt
	if len(ids) > 0 {
		id = ids[0]
	}
	return entityGet[T](c, "config", id)
}

func (c *Context) DeleteConfig(ids ...int) {
	id := c.Pt
	if len(ids) > 0 {
		id = ids[0]
	}
	entityDelete(c, "config", id)
}

func (c *Context) GetUser[T any](id int) *T {
	return entityGet[T](c, "user", id)
}

func (c *Context) DeleteUser(id int) {
	entityDelete(c, "user", id)
}

func (c *Context) GetAdministrator[T any](id int) *T {
	return entityGet[T](c, "admin", id)
}

func (c *Context) DeleteAdministrator(id int) {
	entityDelete(c, "admin", id)
}
