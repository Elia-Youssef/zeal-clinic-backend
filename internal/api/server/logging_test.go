package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// The access log keeps the query string of ordinary requests and drops it
// under /api/sync/, where a caller might put a secret.
func TestLogRequestTarget(t *testing.T) {
	cases := map[string]string{
		"/api/rooms?filter=a&limit=5":            "/api/rooms?filter=a&limit=5",
		"/api/sync/pull?since=3&sync_secret=abc": "/api/sync/pull",
		"/api/sync/status?sync_secret=abc":       "/api/sync/status",
		"/api/sync/events":                       "/api/sync/events",
		"/health":                                "/health",
	}
	e := echo.New()
	for target, want := range cases {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		c := e.NewContext(req, httptest.NewRecorder())
		var buf bytes.Buffer
		if _, err := logRequestTarget(c, &buf); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if got := buf.String(); got != want {
			t.Errorf("log target of %s = %q, want %q", target, got, want)
		}
	}
}
