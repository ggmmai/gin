package gin

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin/orm"
)

type Runtimes struct {
	Pt             int
	Module         string
	Controller     string
	Action         string
	HomeAdmin      string
	Theme          string
	IsGet          bool
	IsPost         bool
	Template       string
	AppDefault     string
	UrlMode        string
	RootPath       string
	DatabasePrefix string
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

func (c *Context) Table(table string) string {
	return c.Runtimes.DatabasePrefix + c.Runtimes.Module + "_" + table
}

func (c *Context) HomeUrl(path string, params ...map[string]any) string {
	query := make(map[string]any, 10)
	if len(params) > 0 {
		for k, v := range params[0] {
			query[k] = v
		}
	}
	query["mm"] = "home"
	return c.Url(path, query)
}

func (c *Context) AdminUrl(path string, params ...map[string]any) string {
	query := make(map[string]any, 10)
	if len(params) > 0 {
		for k, v := range params[0] {
			query[k] = v
		}
	}
	query["mm"] = "admin"
	return c.Url(path, query)
}

func (c *Context) Url(path string, params ...map[string]any) string {
	pt := ""
	module := ""
	homeAdmin := ""
	controller := ""
	action := ""
	querys := url.Values{}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch len(parts) {
	case 1:
		controller = parts[0]
	case 2:
		controller = parts[0]
		action = parts[1]
	case 3:
		module = parts[0]
		controller = parts[1]
		action = parts[2]
	case 4:
		module = parts[0]
		homeAdmin = parts[1]
		controller = parts[2]
		action = parts[3]
	}

	// 如果 module 不是默认模块，则添加 module 参数
	if module == "" && c.Runtimes.Module != c.Runtimes.AppDefault {
		module = c.Runtimes.Module
	}

	// 如果 homeAdmin 不是 home，则添加 homeAdmin 参数
	if homeAdmin == "" && c.Runtimes.HomeAdmin != "home" {
		homeAdmin = c.Runtimes.HomeAdmin
	}

	if len(params) > 0 {
		for k, v := range params[0] {
			value, ok := v.(string)
			if !ok {
				value = fmt.Sprintf("%v", v)
			}

			switch k {
			case "pt":
				pt = value
			case "m":
				module = value
			case "mm":
				homeAdmin = value
			case "cc":
				controller = value
			case "do":
				action = value
			default:
				querys.Set(k, value)
			}
		}
	}

	// 处理缺省值
	if pt == "" {
		pt = strconv.Itoa(c.Pt)
	}

	if module == "" {
		module = c.Runtimes.Module
	}

	if homeAdmin == "" {
		homeAdmin = c.Runtimes.HomeAdmin
	}

	if controller == "" {
		controller = "index"
	}

	if action == "" {
		action = "index"
	}

	if pt != "1" {
		querys.Add("pt", pt)
	}

	if c.Runtimes.UrlMode == "query" {
		if module != c.Runtimes.AppDefault {
			querys.Add("m", module)
		}
		if homeAdmin != "home" {
			querys.Add("mm", homeAdmin)
		}

		if controller != "index" && action != "index" {
			querys.Add("cc", controller)
			querys.Add("do", action)
		}

		if controller != "index" && action == "index" {
			querys.Add("cc", controller)
		}

		if len(querys) > 0 {
			return "/?" + querys.Encode()
		}
		return "/"
	} else {
		route := ""
		if module != c.Runtimes.AppDefault {
			route = "/" + module
		}

		mustHasAction := false
		if route == "" {
			if homeAdmin == "admin" {
				route += "/" + module + "/" + homeAdmin
				mustHasAction = true
			}
		} else if homeAdmin == "admin" {
			route += "/" + homeAdmin
			mustHasAction = true
		}

		// 必须要加上action，否则有歧义
		if mustHasAction || route != "" {
			route += "/" + controller + "/" + action
		} else {
			if controller == "index" && action == "index" {
				route = "/"
			} else if controller != "index" && action == "index" {
				route = "/" + controller
			} else {
				route = "/" + controller + "/" + action
			}
		}

		if len(querys) > 0 {
			return route + "?" + querys.Encode()
		}
		return route
	}
}

func (c *Context) StyleUrl() string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "image")
}

func (c *Context) ImageUrl() string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "image")
}

func (c *Context) AudioUrl() string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "audio")
}

func (c *Context) VideoUrl() string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "video")
}

func (c *Context) FileUrl() string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "file")
}

func (c *Context) PosterUrl() string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "poster")
}

func (c *Context) QrcoderUrl() string {
	host := c.Request.Host
	module := c.Runtimes.Module
	return path.Join(host, "uploads", module, "qrcoder")
}

func (c *Context) ImagePath() string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "image")
}

func (c *Context) AudioPath() string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "audio")
}

func (c *Context) VideoPath() string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "video")
}

func (c *Context) FilePath() string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "file")
}

func (c *Context) PosterPath() string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "poster")
}

func (c *Context) QrcoderPath() string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "qrcoder")
}

func (c *Context) CertPath() string {
	module := c.Runtimes.Module
	rootPath := c.Runtimes.RootPath
	return path.Join(rootPath, "uploads", module, "cert")
}

func (c *Context) View(obj ...any) {
	if len(obj) > 0 {
		c.HTML(200, c.Template(), obj[0])
	} else {
		c.HTML(200, c.Template(), nil)
	}
}

func (c *Context) Template(templates ...string) string {
	if len(templates) > 0 {
		return templates[0]
	} else if c.Runtimes.Template != "" {
		return c.Runtimes.Template
	} else {
		runtimes := c.Runtimes
		return fmt.Sprintf("%s/%s/view/%s/%s/%s.html", runtimes.Module, runtimes.HomeAdmin, runtimes.Theme, runtimes.Controller, runtimes.Action)
	}
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
