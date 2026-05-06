package server

import (
	"net/http"
	"testing"
)

func TestSmoke_Health(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/health", nil, "")
	if rec.Code != http.StatusOK {
		t.Errorf("/health code = %d", rec.Code)
	}
}

func TestSmoke_LoginAndVerify(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodGet, "/api/auth/verify", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("verify expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}
