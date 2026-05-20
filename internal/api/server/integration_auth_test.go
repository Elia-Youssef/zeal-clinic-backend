package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"clinic-api/internal/auth"
	"clinic-api/internal/database/store"
)

// /api/auth/login

func TestLogin_FirstLoginCapturesPassword(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)

	// Default admin starts with empty password_hash. First login should capture.
	tok := loginAdmin(t, e, "secret-1")
	if tok == "" {
		t.Fatal("expected token")
	}

	// Reload user: password_hash should now be non-empty.
	var u store.User
	if err := u.GetByUsername("admin"); err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash == "" {
		t.Errorf("password_hash still empty after first login")
	}

	// Second login with the SAME password must succeed.
	// NB: must wait 1s because JWT iat/exp are second-precision, so two logins in
	// the same second produce identical token strings and hit the
	// tokens.token UNIQUE constraint.
	time.Sleep(1100 * time.Millisecond)
	rec := doRequest(t, e, http.MethodPost, "/api/auth/login",
		asJSON(t, map[string]string{"username": "admin", "password": "secret-1"}), "")
	if rec.Code != http.StatusOK {
		t.Errorf("repeat login expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Wrong password now rejected.
	rec = doRequest(t, e, http.MethodPost, "/api/auth/login",
		asJSON(t, map[string]string{"username": "admin", "password": "wrong"}), "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong pw expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogin_MissingFields(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodPost, "/api/auth/login",
		asJSON(t, map[string]string{"username": ""}), "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing fields, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodPost, "/api/auth/login",
		asJSON(t, map[string]string{"username": "ghost", "password": "x"}), "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestLogin_InactiveUserReturns403(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)

	// Disable the admin user directly.
	if _, err := store.DB.Exec(`UPDATE users SET is_active = 0 WHERE username = 'admin'`); err != nil {
		t.Fatal(err)
	}
	rec := doRequest(t, e, http.MethodPost, "/api/auth/login",
		asJSON(t, map[string]string{"username": "admin", "password": "x"}), "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogin_BadJSON(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	r.Header.Set("Content-Type", "application/json")
	r.Body = http.NoBody
	rec := httptest.NewRecorder()
	// Empty body: c.Bind should still produce an error in practice, but echo
	// often tolerates an empty JSON body and produces zero values. The
	// downstream IsValid catches it and returns 401.
	e.ServeHTTP(rec, r)
	if rec.Code < 400 {
		t.Errorf("expected error code, got %d", rec.Code)
	}
}

func TestLogin_TokenExpiresAtAndPersisted(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := loginAdmin(t, e, "pw")

	// Token row should exist in the DB.
	var got store.Token
	if err := got.GetByValue(tok); err != nil {
		t.Fatalf("token not persisted: %v", err)
	}
	exp, err := got.ExpiresAt.Time()
	if err != nil {
		t.Fatal(err)
	}
	if !exp.After(time.Now()) {
		t.Errorf("expires_at is not in the future: %v", exp)
	}
}

// /api/auth/verify

func TestVerify_ProtectedRequiresAuth(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/api/auth/verify", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", rec.Code)
	}
}

func TestVerify_AcceptsBearerToken(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodGet, "/api/auth/verify", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestVerify_AcceptsAccessTokenQueryParam(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodGet, "/api/auth/verify?access_token="+tok, nil, "")
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 via query token, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestVerify_RejectsBadJWT(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/api/auth/verify", nil, "not-a-jwt")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestVerify_RejectsTamperedJWT(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	// Flip a character in the signature segment.
	tampered := tok[:len(tok)-3] + "AAA"
	rec := doRequest(t, e, http.MethodGet, "/api/auth/verify", nil, tampered)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for tampered token, got %d", rec.Code)
	}
}

func TestVerify_RejectsTokenNotInDB(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Delete the row from the tokens table.
	if _, err := store.DB.Exec(`DELETE FROM tokens WHERE token = ?`, tok); err != nil {
		t.Fatal(err)
	}
	rec := doRequest(t, e, http.MethodGet, "/api/auth/verify", nil, tok)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestVerify_RejectsExpiredTokenAndDeletesIt(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)

	// Force an expired token row in the DB (signature still valid; DB row says
	// expired). Use a JWT generated with an immediate expiry.
	var u store.User
	if err := u.GetByUsername("admin"); err != nil {
		t.Fatal(err)
	}

	tr, err := auth.GenerateToken(u, []string{"patients:read"})
	if err != nil {
		t.Fatal(err)
	}
	// Override expires_at to be in the past in the DB.
	if _, err := store.DB.Exec(`UPDATE tokens SET expires_at = ? WHERE token = ?`,
		"2000-01-01T00:00:00Z", tr.Token); err != nil {
		t.Fatal(err)
	}

	rec := doRequest(t, e, http.MethodGet, "/api/auth/verify", nil, tr.Token)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", rec.Code)
	}

	// The middleware should have deleted the expired row.
	var still store.Token
	if err := still.GetByValue(tr.Token); err == nil {
		t.Errorf("expired token row should be deleted by auth middleware, but still exists")
	}
}

func TestVerify_RejectsTokenForInactiveUser(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	if _, err := store.DB.Exec(`UPDATE users SET is_active = 0 WHERE username = 'admin'`); err != nil {
		t.Fatal(err)
	}
	rec := doRequest(t, e, http.MethodGet, "/api/auth/verify", nil, tok)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestVerify_NonBearerSchemeIgnored(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	r := httptest.NewRequest(http.MethodGet, "/api/auth/verify", nil)
	r.Header.Set("Authorization", "Token "+tok) // wrong scheme
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-Bearer scheme, got %d", rec.Code)
	}
}

// /api/auth/logout

func TestLogout_DeletesToken(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/auth/logout", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("logout expected 200, got %d", rec.Code)
	}

	// Token should be gone.
	var got store.Token
	if err := got.GetByValue(tok); err == nil {
		t.Errorf("expected token to be deleted, but still exists")
	}

	// Subsequent verify with the same token must 401.
	rec = doRequest(t, e, http.MethodGet, "/api/auth/verify", nil, tok)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("after logout expected 401, got %d", rec.Code)
	}
}

func TestLogout_WithoutTokenStillSucceeds(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodPost, "/api/auth/logout", nil, "")
	// Logout is public and best-effort: succeeds regardless.
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestLogin_RapidRepeatSucceeds verifies that two same-second logins succeed
// without colliding on tokens.token UNIQUE. A jti claim in the JWT ensures
// two consecutive logins produce distinct tokens.
func TestLogin_RapidRepeatSucceeds(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)

	// First login establishes the password.
	tok1 := loginAdmin(t, e, "rapid-pw")

	rec := doRequest(t, e, http.MethodPost, "/api/auth/login",
		asJSON(t, map[string]string{"username": "admin", "password": "rapid-pw"}), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on rapid repeat login, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			Token string `json:"token"`
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Token == "" || env.Data.Token == tok1 {
		t.Errorf("expected a distinct token on rapid repeat login, got token1=%q token2=%q", tok1, env.Data.Token)
	}
}

func TestLogin_BodyContainsRoleAndScopes(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodPost, "/api/auth/login",
		asJSON(t, map[string]string{"username": "admin", "password": "x"}), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			Role   string   `json:"role"`
			Scopes []string `json:"scopes"`
			User   string   `json:"user"`
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Role != "super-admin" {
		t.Errorf("role = %q want super-admin", env.Data.Role)
	}
	if len(env.Data.Scopes) == 0 {
		t.Errorf("scopes empty")
	}
	if env.Data.User != "Admin" {
		t.Errorf("user = %q want Admin", env.Data.User)
	}
}
