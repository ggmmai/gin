package gin

import (
	"net/http"
	"path"

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

func (c *Context) D(table string) *orm.Builder {
	if orm.OrmPool == nil {
		panic("orm: default connection is not set, call orm.OrmOpen first")
	}

	if orm.OrmPrefix != "" {
		table = orm.OrmPrefix + table
	}
	return orm.NewBuilder(orm.OrmPool, table)
}
