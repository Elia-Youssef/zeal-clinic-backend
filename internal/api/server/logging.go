package server

import (
	"bytes"
	"strings"

	"github.com/labstack/echo/v4"
)

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
