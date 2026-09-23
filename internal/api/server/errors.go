package server

import (
	"errors"
	"net/http"
	"strings"

	"clinic-api/internal/api/httpx"

	"github.com/labstack/echo/v4"
)

// isAPIPath reports whether a request path belongs to the API tree: the /api
// group with its bare root.
func isAPIPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}

// apiErrorHandler answers an error that reaches it on an API path in the
// response envelope the dashboard reads, with the error's own status: unknown
// routes and methods, refused bodies, and the panics the recover middleware
// turns into errors. A route may also answer an error itself in its own body
// (the cloud's sync gate does), and a response already committed keeps its
// body. Every other path keeps echo's default answer.
func apiErrorHandler(e *echo.Echo) echo.HTTPErrorHandler {
	fallback := e.DefaultHTTPErrorHandler
	return func(err error, c echo.Context) {
		path := c.Request().URL.Path
		if !isAPIPath(path) {
			fallback(err, c)
			return
		}
		if c.Response().Committed {
			return
		}
		code := http.StatusInternalServerError
		message := http.StatusText(code)
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			code = httpErr.Code
			message = http.StatusText(code)
			if text, ok := httpErr.Message.(string); ok && text != "" {
				message = text
			}
		}
		var reply error
		if c.Request().Method == http.MethodHead {
			reply = c.NoContent(code)
		} else {
			reply = c.JSON(code, httpx.Response{Error: message})
		}
		if reply != nil {
			e.Logger.Error(reply)
		}
	}
}
