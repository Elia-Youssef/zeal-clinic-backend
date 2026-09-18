package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// Every error under /api comes back in the envelope, so the dashboard has an
// Error text to show; paths outside /api keep echo's own answers.
func TestAPIErrorsUseTheEnvelope(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	e.GET("/api/error-test/panic", func(echo.Context) error { panic("a bug in a handler") })
	e.GET("/api/error-test/teapot", func(echo.Context) error { return echo.NewHTTPError(http.StatusTeapot, "short and stout") })

	for _, tc := range []struct {
		name, method, path, token string
		length                    int64
		code                      int
		message                   string
	}{
		{"an unknown route", http.MethodGet, "/api/no-such-route", tok, 0, http.StatusNotFound, "Not Found"},
		{"the bare prefix", http.MethodGet, "/api", tok, 0, http.StatusNotFound, "Not Found"},
		{"an unknown method on a known route", http.MethodPatch, "/api/patients", tok, 0, http.StatusNotFound, "Not Found"},
		{"a recovered panic", http.MethodGet, "/api/error-test/panic", "", 0, http.StatusInternalServerError, "Internal Server Error"},
		{"an error a handler returns", http.MethodGet, "/api/error-test/teapot", "", 0, http.StatusTeapot, "short and stout"},
		{"a body over the limit", http.MethodPost, "/api/rooms", tok, 16<<20 + 1, http.StatusRequestEntityTooLarge, "Request Entity Too Large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte("{}")))
			req.Header.Set("Content-Type", "application/json")
			if tc.length > 0 {
				req.ContentLength = tc.length
			}
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.code {
				t.Fatalf("%s %s: %d %s, want %d", tc.method, tc.path, rec.Code, rec.Body.String(), tc.code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("%s %s: Content-Type %q", tc.method, tc.path, ct)
			}
			var env struct {
				Error   string
				Success bool
				Data    json.RawMessage
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("%s %s: body %s is no envelope: %v", tc.method, tc.path, rec.Body.String(), err)
			}
			if env.Success || env.Error != tc.message || string(env.Data) != "null" {
				t.Errorf("%s %s: %s, want {Error: %q, Success: false, Data: null}", tc.method, tc.path, rec.Body.String(), tc.message)
			}
		})
	}

	// A HEAD gets the status without a body.
	rec := doRequest(t, e, http.MethodHead, "/api/no-such-route", nil, tok)
	if rec.Code != http.StatusNotFound || rec.Body.Len() != 0 {
		t.Errorf("HEAD /api/no-such-route: %d %q, want a bare 404", rec.Code, rec.Body.String())
	}

	// Outside /api, echo's own answer stays.
	rec = doRequest(t, e, http.MethodGet, "/files/missing.pdf", nil, tok)
	var plain struct{ Message string }
	if err := json.Unmarshal(rec.Body.Bytes(), &plain); rec.Code != http.StatusNotFound || err != nil || plain.Message != "Not Found" {
		t.Errorf("GET /files/missing.pdf: %d %q, want echo's 404", rec.Code, rec.Body.String())
	}
}
