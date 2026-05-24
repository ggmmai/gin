package gin

import (
	"net/http"
)

type Runtimes struct {
	Pt         uint
	Module     string
	Controller string
	Action     string
	Home_admin string
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
