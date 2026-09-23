package server

import (
	"bytes"
	"io"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// requestLogger is the access-log middleware: one line per request, carrying
// the request target through logRequestTarget's custom tag. The lines go to
// out; the server logs to stdout and the tests capture the writer.
func requestLogger(out io.Writer) echo.MiddlewareFunc {
	return middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format:        "${time_rfc3339} | ${status} | ${latency_human} | ${method} ${custom}\n",
		CustomTagFunc: logRequestTarget,
		Output:        out,
	})
}

// logRequestTarget writes the request target for the access log: the full
// URI, except under /api/sync/, where only the path is logged so that a
// query string a peer might add never reaches the log.
func logRequestTarget(c echo.Context, buf *bytes.Buffer) (int, error) {
	req := c.Request()
	if strings.HasPrefix(req.URL.Path, "/api/sync/") {
		return buf.WriteString(req.URL.Path)
	}
	return buf.WriteString(req.RequestURI)
}
