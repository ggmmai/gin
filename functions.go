package gin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (c *Context) RewritePath() {
	module := strings.ToLower(c.DefaultQuery("m", "xiangqin"))
	controller := strings.ToLower(c.DefaultQuery("c", "index"))
	action := strings.ToLower(c.DefaultQuery("do", "index"))
	home_admin := strings.ToLower(c.DefaultQuery("mm", "home"))

	//判断是否为静态文件
	is_static := false
	static_files := []string{"/engine"}
	path := fmt.Sprintf("/%v/%s/%v/%v", module, home_admin, controller, action)
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		for _, static := range static_files {
			if strings.HasPrefix(c.Request.URL.Path, static) {
				is_static = true
			}
		}
	}

	if is_static == false && c.Request.URL.Path == "/" {
		c.Request.URL.Path = path
	}
}

func (c *Context) Pt() int {
	pt, err := strconv.Atoi(c.DefaultQuery("pt", "1"))
	if pt <= 0 || err == nil {
		pt = 1
	}
	return pt
}

func (c *Context) Result(code int8, msg string, data any) {
	c.JSON(200, H{
		"code": code,
		"msg":  msg,
		"data": data,
	})
}

func (c *Context) Ok(msg string, data any) {
	c.JSON(200, H{
		"code": 1,
		"msg":  msg,
		"data": data,
	})
}

func (c *Context) Wn(msg string, data any) {
	c.JSON(200, H{
		"code": -1,
		"msg":  msg,
		"data": data,
	})
}

func (c *Context) Er(msg string, data any) {
	c.JSON(200, H{
		"code": -1,
		"msg":  msg,
		"data": data,
	})
}
