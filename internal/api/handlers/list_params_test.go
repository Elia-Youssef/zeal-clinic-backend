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
	e := echo.New()
	target := "/?" + query
	if query == "" {
		target = "/"
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec)
}

func TestParseListParams_Empty(t *testing.T) {
	p := parseListParams(makeCtx(t, ""))
	if p.Offset != 0 || p.Limit != 0 || p.Filter != "" || p.Sort != "" || p.Order != "" {
		t.Errorf("got %+v, want zero values", p)
	}
}

func TestParseListParams_AllFields(t *testing.T) {
	p := parseListParams(makeCtx(t, "offset=20&limit=50&filter=abc&sort=name&order=desc"))
	if p.Offset != 20 || p.Limit != 50 || p.Filter != "abc" || p.Sort != "name" || p.Order != "desc" {
		t.Errorf("got %+v", p)
	}
}

func TestParseListParams_SortWithoutOrder(t *testing.T) {
	p := parseListParams(makeCtx(t, "sort=createdAt"))
	if p.Sort != "createdAt" || p.Order != "" {
		t.Errorf("got %+v", p)
	}
}

func TestParseListParams_OrderWithoutSort(t *testing.T) {
	p := parseListParams(makeCtx(t, "order=desc"))
	if p.Sort != "" || p.Order != "desc" {
		t.Errorf("got %+v", p)
	}
}

func TestParseListParams_NegativeOffsetDropped(t *testing.T) {
	// Negative offsets are explicitly rejected (n >= 0).
	p := parseListParams(makeCtx(t, "offset=-5&limit=10"))
	if p.Offset != 0 {
		t.Errorf("expected offset=0 (negative dropped), got %d", p.Offset)
	}
	if p.Limit != 10 {
		t.Errorf("expected limit=10, got %d", p.Limit)
	}
}

func TestParseListParams_ZeroOffsetIsKept(t *testing.T) {
	p := parseListParams(makeCtx(t, "offset=0&limit=10"))
	if p.Offset != 0 || p.Limit != 10 {
		t.Errorf("got %+v", p)
	}
}

func TestParseListParams_ZeroLimitDropped(t *testing.T) {
	// Zero limit is rejected (n > 0).
	p := parseListParams(makeCtx(t, "offset=5&limit=0"))
	if p.Limit != 0 {
		t.Errorf("zero limit must be dropped, got %d", p.Limit)
	}
	if p.Offset != 5 {
		t.Errorf("offset preserved, got %d", p.Offset)
	}
}

func TestParseListParams_NegativeLimitDropped(t *testing.T) {
	p := parseListParams(makeCtx(t, "offset=5&limit=-3"))
	if p.Limit != 0 {
		t.Errorf("negative limit must be dropped, got %d", p.Limit)
	}
}

func TestParseListParams_NonNumericIgnored(t *testing.T) {
	p := parseListParams(makeCtx(t, "offset=abc&limit=xyz&filter=ok"))
	if p.Offset != 0 || p.Limit != 0 {
		t.Errorf("non-numeric must be ignored, got %+v", p)
	}
	if p.Filter != "ok" {
		t.Errorf("filter should still be parsed, got %q", p.Filter)
	}
}

func TestParseListParams_FloatValuesIgnored(t *testing.T) {
	// strconv.Atoi rejects floats.
	p := parseListParams(makeCtx(t, "offset=1.5&limit=10.0"))
	if p.Offset != 0 || p.Limit != 0 {
		t.Errorf("floats must be rejected, got %+v", p)
	}
}

func TestParseListParams_EmptyValuesNotParsed(t *testing.T) {
	p := parseListParams(makeCtx(t, "offset=&limit=&filter=&sort=&order="))
	if p.Offset != 0 || p.Limit != 0 || p.Filter != "" || p.Sort != "" || p.Order != "" {
		t.Errorf("got %+v", p)
	}
}

func TestParseListParams_FilterIsRawNotTrimmed(t *testing.T) {
	// Whitespace-only filter is preserved verbatim; store-side FilterClause
	// turns it into a LIKE %  %.
	p := parseListParams(makeCtx(t, "filter=%20%20"))
	if p.Filter != "  " {
		t.Errorf("filter should preserve whitespace, got %q", p.Filter)
	}
}

func TestParseListParams_LargeNumbers(t *testing.T) {
	p := parseListParams(makeCtx(t, "offset=2147483647&limit=2147483647"))
	if p.Offset != 2147483647 || p.Limit != 2147483647 {
		t.Errorf("got %+v", p)
	}
}

func TestParseListParams_OverflowsAreRejected(t *testing.T) {
	// > int32 max but Atoi handles up to int range; on a 64-bit platform a
	// huge value still parses. We just verify it doesn't crash.
	p := parseListParams(makeCtx(t, "offset=99999999999999999999999&limit=99999999999999999999999"))
	if p.Offset != 0 || p.Limit != 0 {
		t.Errorf("strconv.Atoi overflow should leave defaults, got %+v", p)
	}
}

func TestParseListParams_DuplicateKeysUseFirst(t *testing.T) {
	// echo's QueryParam returns the first occurrence.
	p := parseListParams(makeCtx(t, "limit=5&limit=99"))
	if p.Limit != 5 {
		t.Errorf("expected first occurrence (5), got %d", p.Limit)
	}
}
