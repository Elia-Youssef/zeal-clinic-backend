//go:build !cloud

package sync

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRequireCriticalSync_LocalAlwaysAllows(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/invoice", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := RequireCriticalSync(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})(c)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
