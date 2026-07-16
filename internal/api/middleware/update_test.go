package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func runUpdateGateRequest(t *testing.T, installing bool, path string) (*httptest.ResponseRecorder, bool) {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	called := false
	next := func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	}
	if err := updateGate(func() bool { return installing })(next)(c); err != nil {
		t.Fatal(err)
	}
	return rec, called
}

func TestUpdateGateBlocksAPIsDuringHealthCheck(t *testing.T) {
	rec, called := runUpdateGateRequest(t, true, "/api/patients")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if called {
		t.Fatal("blocked API request reached its handler")
	}
	if !strings.Contains(rec.Body.String(), "Update in progress") {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestUpdateGateKeepsHealthAndStatusAvailable(t *testing.T) {
	for _, path := range []string{"/health", "/api/update/status"} {
		t.Run(path, func(t *testing.T) {
			rec, called := runUpdateGateRequest(t, true, path)
			if rec.Code != http.StatusOK || !called {
				t.Fatalf("path %s: status=%d called=%v, want 200/true", path, rec.Code, called)
			}
		})
	}
}

func TestUpdateGateReopensAPIsAfterHealthCheck(t *testing.T) {
	rec, called := runUpdateGateRequest(t, false, "/api/patients")
	if rec.Code != http.StatusOK || !called {
		t.Fatalf("status=%d called=%v, want 200/true", rec.Code, called)
	}
}
