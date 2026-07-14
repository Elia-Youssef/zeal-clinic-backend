//go:build cloud

package sync

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRequireCriticalSync_CloudSessionReadiness(t *testing.T) {
	token, err := beginCriticalSession()
	if err != nil {
		t.Fatal(err)
	}
	defer endCriticalSession(token)

	call := func() int {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/invoice", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := RequireCriticalSync(func(c echo.Context) error {
			return c.NoContent(http.StatusNoContent)
		})(c); err != nil {
			t.Fatal(err)
		}
		return rec.Code
	}

	if got := call(); got != http.StatusServiceUnavailable {
		t.Fatalf("before ready: status = %d, want %d", got, http.StatusServiceUnavailable)
	}
	if !markCriticalSessionReady(token) {
		t.Fatal("current token did not open the gate")
	}
	if got := call(); got != http.StatusNoContent {
		t.Fatalf("after ready: status = %d, want %d", got, http.StatusNoContent)
	}
	endCriticalSession(token)
	if got := call(); got != http.StatusServiceUnavailable {
		t.Fatalf("after disconnect: status = %d, want %d", got, http.StatusServiceUnavailable)
	}
}
