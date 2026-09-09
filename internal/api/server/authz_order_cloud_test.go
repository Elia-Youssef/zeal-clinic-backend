//go:build cloud

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"clinic-api/internal/buildmode"
)

// Unknown paths under /api/sync ask for the sync secret, not a bearer token:
// a signed-in user gets the same 401 as an anonymous caller, and only the
// secret reveals that the path doesn't exist.
func TestAuthzOrder_UnknownSyncRoutesNeedTheSecretFirst(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	send := func(method, path, bearer, secret string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if secret != "" {
			req.Header.Set("X-Sync-Secret", secret)
			req.Header.Set("X-Sync-Version", buildmode.Version)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/sync"},
		{http.MethodGet, "/api/sync/no-such-route"},
		{http.MethodPost, "/api/sync/no-such-route"},
		{http.MethodDelete, "/api/sync/pull"},
	} {
		for _, bearer := range []string{"", tok} {
			rec := send(c.method, c.path, bearer, "")
			if got := classify(rec.Code, rec.Body.String()); got != outcomeNoAuth {
				t.Errorf("%s %s without the secret (bearer %t): %d %s, want 401", c.method, c.path, bearer != "", rec.Code, rec.Body.String())
			}
		}
		if rec := send(c.method, c.path, "", testSyncSecret); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s with the secret: %d %s, want 404", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
}
