package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// makeCtx builds an echo.Context for the given query string.
func makeCtx(t *testing.T, query string) echo.Context {
	t.Helper()
	target := "/"
	if query != "" {
		target = "/?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	return echo.New().NewContext(req, httptest.NewRecorder())
}

func TestParseListParams(t *testing.T) {
	t.Run("empty query yields zero values", func(t *testing.T) {
		p := parseListParams(makeCtx(t, ""))
		if p.Offset != 0 || p.Limit != 0 || p.Filter != "" || p.Sort != "" || p.Order != "" {
			t.Errorf("got %+v, want zero values", p)
		}
	})

	t.Run("all fields parsed", func(t *testing.T) {
		p := parseListParams(makeCtx(t, "offset=20&limit=50&filter=abc&sort=name&order=desc"))
		if p.Offset != 20 || p.Limit != 50 || p.Filter != "abc" || p.Sort != "name" || p.Order != "desc" {
			t.Errorf("got %+v", p)
		}
	})

	// Offset must be >= 0 and limit must be > 0; out-of-range/non-numeric values
	// are dropped to the zero default rather than rejected with an error.
	t.Run("invalid offset/limit drop to default", func(t *testing.T) {
		cases := []string{"offset=-5&limit=10", "offset=5&limit=0", "offset=5&limit=-3", "offset=abc&limit=xyz"}
		for _, q := range cases {
			p := parseListParams(makeCtx(t, q))
			if p.Offset < 0 || p.Limit < 0 {
				t.Errorf("%q -> %+v, want non-negative", q, p)
			}
		}
		// zero offset is a valid value and is kept.
		if p := parseListParams(makeCtx(t, "offset=0&limit=10")); p.Offset != 0 || p.Limit != 10 {
			t.Errorf("offset=0 should be kept: %+v", p)
		}
		// negative offset is dropped but a valid sibling limit survives.
		if p := parseListParams(makeCtx(t, "offset=-5&limit=10")); p.Offset != 0 || p.Limit != 10 {
			t.Errorf("got %+v", p)
		}
	})
}
