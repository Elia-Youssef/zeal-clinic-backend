package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// The access log keeps the query string of ordinary requests and drops it
// under /api/sync/, where a caller might put a secret — and the target
// reaches the log line through the logger's own custom tag.
func TestRequestLoggerLogsTheTarget(t *testing.T) {
	var buf bytes.Buffer
	e := echo.New()
	e.Use(requestLogger(&buf))
	e.GET("/api/rooms", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	e.GET("/api/sync/pull", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })

	for _, target := range []string{"/api/rooms?filter=a&limit=5", "/api/sync/pull?since=3&sync_secret=abc"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", target, rec.Code, rec.Body.String())
		}
	}

	logged := buf.String()
	if !strings.Contains(logged, "GET /api/rooms?filter=a&limit=5\n") {
		t.Errorf("log lines %q miss the rooms target with its query", logged)
	}
	if !strings.Contains(logged, "GET /api/sync/pull\n") || strings.Contains(logged, "sync_secret") {
		t.Errorf("log lines %q must carry the sync path without its query", logged)
	}
}
