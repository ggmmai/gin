package gin

import (
	"fmt"
	"net/http"
	"strings"
)

func (c *Context) RewritePath() {
	module := strings.ToLower(c.DefaultQuery("m", "xiangqin"))
	controller := strings.ToLower(c.DefaultQuery("c", "index"))
	action := strings.ToLower(c.DefaultQuery("do", "index"))
	home_admin := strings.ToLower(c.DefaultQuery("mm", "home"))

	Path := c.Request.URL.Path
	path := fmt.Sprintf("/%v/%s/%v/%v", module, home_admin, controller, action)
	if Path == "/" || strings.HasPrefix(Path, "/pages") {
		c.Request.URL.Path = path
	}
}

func (c *Context) Pt() uint {
	pt := struct {
		Id uint
	}{}

	if err := c.ShouldBindQuery(&pt); err != nil || pt.Id == 0 {
		pt.Id = 1
	}

	return pt.Id
}

type response struct {
	Code int8   `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

func (c *Context) Result(code int8, msg string, data any) {
	c.JSON(http.StatusOK, response{
		code,
		msg,
		data,
	})
}

func (c *Context) Ok(msg string, data any) {
	c.JSON(http.StatusOK, &response{
		0,
		msg,
		data,
	})
}

func (c *Context) Success(msg string, data any) {
	c.JSON(http.StatusOK, &response{
		0,
		msg,
		data,
	})
}

func (c *Context) Warn(msg string, data any) {
	c.JSON(http.StatusOK, &response{
		-1,
		msg,
		data,
	})
}

func (c *Context) Fail(msg string, data any) {
	c.JSON(http.StatusOK, &response{
		1,
		msg,
		data,
	})
}
