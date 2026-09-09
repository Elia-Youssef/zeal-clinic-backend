package server

import (
	"net/http"
	"strings"
	"testing"
)

// Under /api, sign-in is checked before a route is looked up: a path that
// doesn't exist, or a method a path doesn't have, answers 401 to anonymous
// callers and 404 only once signed in.
func TestAuthzOrder_UnknownAPIRoutesNeedSignInFirst(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api"},
		{http.MethodGet, "/api/no-such-route"},
		{http.MethodPost, "/api/no-such-route"},
		{http.MethodPut, "/api/no/such/route"},
		{http.MethodDelete, "/api/no-such-route/42"},
		{http.MethodPatch, "/api/allergies"},
		{http.MethodDelete, "/api/users/42"},
	} {
		rec := doRequest(t, e, c.method, c.path, nil, "")
		if got := classify(rec.Code, rec.Body.String()); got != outcomeNoAuth {
			t.Errorf("%s %s without a token: %d %s, want the sign-in 401", c.method, c.path, rec.Code, rec.Body.String())
		}
		rec = doRequest(t, e, c.method, c.path, nil, tok)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s signed in: %d %s, want 404", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
}

// Routes are matched on the raw path, so an encoded slash or letter never
// reaches a protected route by another spelling: /files and /api paths
// written that way fall through to the page handler, which serves only the
// dashboard build, or to the sign-in check.
func TestAuthzOrder_EncodedPathsDoNotBypassSignIn(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodGet, "/api/reports/revenue/pdf?from=2026-05-01&to=2026-05-31", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("revenue report: %d %s", rec.Code, rec.Body.String())
	}
	var data struct {
		URL string `json:"url"`
	}
	decodeEnvelope(t, rec.Body, &data)
	name := strings.TrimPrefix(data.URL, "/files/")
	if name == "" || name == data.URL {
		t.Fatalf("report url = %q", data.URL)
	}

	for _, path := range []string{"/files%2F" + name, "/files%2f" + name, "/%66iles/" + name} {
		rec := doRequest(t, e, http.MethodGet, path, nil, "")
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") ||
			strings.HasPrefix(rec.Body.String(), "%PDF-") {
			t.Errorf("GET %s without a token: %d %q, want the dashboard page", path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	for _, c := range []struct {
		path string
		code int
	}{
		{"/api%2Fusers", http.StatusNotFound},
		{"/api%2Fauth%2Fme", http.StatusNotFound},
		{"/api/%75sers", http.StatusUnauthorized},
		{"/api/users%2Fdropdown", http.StatusUnauthorized},
	} {
		rec := doRequest(t, e, http.MethodGet, c.path, nil, "")
		if rec.Code != c.code || strings.Contains(rec.Body.String(), `"Success":true`) {
			t.Errorf("GET %s without a token: %d %s, want %d", c.path, rec.Code, rec.Body.String(), c.code)
		}
	}
}
