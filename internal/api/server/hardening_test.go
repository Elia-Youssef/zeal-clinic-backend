package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"clinic-api/internal/buildmode"

	"github.com/labstack/echo/v4"
)

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "same-origin",
		"X-Robots-Tag":           "noindex, nofollow",
	}
	check := func(what string, rec *httptest.ResponseRecorder) {
		t.Helper()
		for name, value := range want {
			if got := rec.Header().Get(name); got != value {
				t.Errorf("%s: %s = %q, want %q", what, name, got, value)
			}
		}
		if rec.Header().Get("Strict-Transport-Security") != "" {
			t.Errorf("%s carries HSTS; TLS termination is the proxy's job", what)
		}
	}
	for _, path := range []string{"/health", "/robots.txt", "/", "/api/patients", "/api/no-such-route", "/files/missing.pdf"} {
		check("GET "+path, doRequest(t, e, http.MethodGet, path, nil, ""))
	}
	// Answers given before the routes, such as a refused body, carry them too.
	req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 16<<20 + 1
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized POST: %d, want 413", rec.Code)
	}
	check("a refused body", rec)
}

func TestContentSecurityPolicyOnTheAppShellOnly(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	for path, want := range map[string]string{
		"/":                         appShellCSP,
		"/patients/some-id/details": appShellCSP,
		"/health":                   "",
		"/api/patients":             "",
	} {
		rec := doRequest(t, e, http.MethodGet, path, nil, "")
		if got := rec.Header().Get("Content-Security-Policy"); got != want {
			t.Errorf("GET %s: Content-Security-Policy = %q, want %q", path, got, want)
		}
	}
}

// The limit refuses a request by its declared length before reading it, so a
// small body with a large Content-Length stands in for a large upload. Only
// the cloud build receives the sync push and the restore upload, so only its
// matched routes skip the limit; other spellings of those paths match other
// routes and stay limited.
func TestBodyLimitSkipsTheLargeBodyRoutes(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	declared := func(method, path, token string, length int64) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = length
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	over := int64(16<<20 + 1)
	if rec := declared(http.MethodPost, "/api/rooms", tok, over); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("POST /api/rooms declaring %d bytes: %d, want 413", over, rec.Code)
	}
	if rec := declared(http.MethodPost, "/api/rooms", tok, 2); rec.Code == http.StatusRequestEntityTooLarge {
		t.Errorf("POST /api/rooms with a small body: 413")
	}
	for _, path := range []string{"/api/cloud-restore", "/api/sync/push"} {
		rec := declared(http.MethodPost, path, tok, over)
		if buildmode.Cloud && rec.Code == http.StatusRequestEntityTooLarge {
			t.Errorf("cloud: POST %s declaring %d bytes: 413, but the limit must not apply there", path, over)
		}
		if !buildmode.Cloud && rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("clinic: POST %s declaring %d bytes: %d, want 413 (the clinic receives no upload there)", path, over, rec.Code)
		}
	}
	for _, path := range []string{"/api/sync%2Fpush", "/api/sync%2fpush", "/api/%73ync/push", "/api/cloud%2Drestore", "/api/sync-foo"} {
		if rec := declared(http.MethodPost, path, tok, over); rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("POST %s declaring %d bytes: %d, want 413", path, over, rec.Code)
		}
	}
}

// The built server bounds both the headers of a request and the quiet time of
// a kept-alive connection.
func TestServerTimeouts(t *testing.T) {
	setupTestEnv(t)
	e := CreateServerWithOptions(Options{})
	if e.Server.ReadHeaderTimeout != readHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %s, want %s", e.Server.ReadHeaderTimeout, readHeaderTimeout)
	}
	if e.Server.IdleTimeout != idleTimeout {
		t.Errorf("IdleTimeout = %s, want %s", e.Server.IdleTimeout, idleTimeout)
	}
}

func TestCORSOnlyInDevForTheViteOrigins(t *testing.T) {
	setupTestEnv(t)
	preflight := func(e *echo.Echo, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodOptions, "/api/patients", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", http.MethodPut)
		req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	dev := CreateServerWithOptions(Options{DevCORS: true})
	for _, origin := range devOrigins {
		rec := preflight(dev, origin)
		if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Errorf("dev preflight from %s: %d, Access-Control-Allow-Origin %q", origin, rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
		}
		for header, want := range map[string]string{"Access-Control-Allow-Methods": "PATCH", "Access-Control-Allow-Headers": "Authorization"} {
			if got := rec.Header().Get(header); !strings.Contains(got, want) {
				t.Errorf("dev preflight from %s: %s = %q, want it to list %s", origin, header, got, want)
			}
		}
	}
	if rec := preflight(dev, "http://clinic.example"); rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("dev preflight from another origin was allowed: %v", rec.Header())
	}

	built := CreateServerWithOptions(Options{})
	if rec := preflight(built, devOrigins[0]); rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("the built server answers a preflight with CORS headers: %v", rec.Header())
	}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", devOrigins[0])
	rec := httptest.NewRecorder()
	built.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("the built server adds CORS headers to a plain GET: %v", rec.Header())
	}
}

// The login rate limit keys on the extractor's address: a forwarded address
// sent through a loopback peer is ignored by the clinic build (the eleventh
// attempt from one peer is refused) and honored by the cloud build (each
// forwarded address gets its own allowance, as behind a local reverse proxy).
func TestLoginRateLimitUsesTheClientIPExtractor(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	attempt := func(i int) int {
		body := asJSON(t, map[string]string{"username": "nobody", "password": "wrong-password"})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:4321"
		req.Header.Set(echo.HeaderXForwardedFor, "198.51.100."+strconv.Itoa(i))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 1; i <= 10; i++ {
		if code := attempt(i); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d, want 401", i, code)
		}
	}
	want := http.StatusTooManyRequests
	if buildmode.Cloud {
		want = http.StatusUnauthorized
	}
	if code := attempt(11); code != want {
		t.Errorf("attempt 11 on the cloud build %v: %d, want %d", buildmode.Cloud, code, want)
	}
}

func TestAuditLogRecordsTheExtractorAddress(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	body := asJSON(t, map[string]any{"name": "Audit Room", "type": "Consultation"})
	req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	req.RemoteAddr = "127.0.0.1:4321"
	req.Header.Set(echo.HeaderXForwardedFor, "198.51.100.7")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create room: %d %s", rec.Code, rec.Body.String())
	}
	want := "127.0.0.1"
	if buildmode.Cloud {
		want = "198.51.100.7"
	}
	if n := countTableRows(t, "audit_log", "entity_type = 'rooms' AND ip_address = ?", want); n != 1 {
		t.Errorf("audit rows for the room with ip_address %s: %d, want 1", want, n)
	}
}
