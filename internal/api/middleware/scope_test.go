package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// scopeRequest exercises RequireScope with a freshly constructed context whose
// "scopes" key is set to the provided value.
func scopeRequest(t *testing.T, scopes any, required string) (int, string) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if scopes != nil {
		c.Set("scopes", scopes)
	}
	called := false
	next := func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	}
	mw := RequireScope(required)(next)
	if err := mw(c); err != nil {
		t.Fatalf("middleware returned err: %v", err)
	}
	if rec.Code == http.StatusOK && !called {
		t.Errorf("200 returned but next was not called")
	}
	if rec.Code == http.StatusForbidden && called {
		t.Errorf("403 returned but next was called")
	}
	return rec.Code, rec.Body.String()
}

func TestRequireScope_NoScopesContext(t *testing.T) {
	code, body := scopeRequest(t, nil, "patients:read")
	if code != http.StatusForbidden {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(body, "no scopes found") {
		t.Errorf("body = %s", body)
	}
}

func TestRequireScope_WrongTypeInContext(t *testing.T) {
	// AuthMiddleware is supposed to set []string. Anything else must be rejected.
	cases := map[string]any{
		"string":  "patients:read",
		"map":     map[string]bool{"patients:read": true},
		"int":     42,
		"any nil": (*[]string)(nil),
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			code, body := scopeRequest(t, v, "patients:read")
			if code != http.StatusForbidden {
				t.Errorf("code = %d", code)
			}
			if !strings.Contains(body, "no scopes found") {
				t.Errorf("body = %s", body)
			}
		})
	}
}

func TestRequireScope_EmptyScopesSlice(t *testing.T) {
	code, body := scopeRequest(t, []string{}, "patients:read")
	if code != http.StatusForbidden {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(body, "no scopes found") {
		t.Errorf("body = %s", body)
	}
}

func TestRequireScope_MissingRequired(t *testing.T) {
	code, body := scopeRequest(t, []string{"patients:read", "users:read"}, "payments:write")
	if code != http.StatusForbidden {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(body, "missing required scope: payments:write") {
		t.Errorf("body = %s", body)
	}
}

func TestRequireScope_ExactMatch(t *testing.T) {
	code, _ := scopeRequest(t, []string{"patients:read"}, "patients:read")
	if code != http.StatusOK {
		t.Errorf("code = %d", code)
	}
}

func TestRequireScope_MatchAmongMany(t *testing.T) {
	scopes := []string{"a", "b", "c", "patients:write", "z"}
	code, _ := scopeRequest(t, scopes, "patients:write")
	if code != http.StatusOK {
		t.Errorf("code = %d", code)
	}
}

func TestRequireScope_CaseSensitive(t *testing.T) {
	code, body := scopeRequest(t, []string{"Patients:Read"}, "patients:read")
	if code != http.StatusForbidden {
		t.Errorf("scope match must be case-sensitive, got code = %d body = %s", code, body)
	}
}

func TestRequireScope_ResponseIsJSON(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("scopes", []string{"x"})
	mw := RequireScope("y")(func(c echo.Context) error { return nil })
	_ = mw(c)
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q want application/json", ct)
	}
}
