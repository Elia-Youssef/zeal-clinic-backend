package server

import (
	"net/http"
	"strings"
	"testing"

	"clinic-api/internal/auth"
	"clinic-api/internal/database/store"
)

// generateTokenForRole creates a fresh user with the given role and returns a
// bearer token whose scopes match the role's scopes.
func generateTokenForRole(t *testing.T, role string, suffix string) string {
	t.Helper()
	u := store.User{
		Username:    "scope-" + suffix,
		DisplayName: "Scope " + suffix,
		Role:        role,
		IsActive:    true,
	}
	if err := u.Create("hash"); err != nil {
		t.Fatal(err)
	}

	var r store.Role
	if err := r.GetByName(role); err != nil {
		t.Fatal(err)
	}
	tr, err := auth.GenerateToken(u, r.Scopes)
	if err != nil {
		t.Fatal(err)
	}
	return tr.Token
}

func TestScope_ReadOnlyUserCannotWritePatients(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)

	// Narrow the user role to read-only patient scopes for this test so we can
	// exercise the scope-deny path on POST /api/patients. The seeded `user` role
	// actually carries patients:write, so we strip it here.
	if _, err := store.DB.Exec(`UPDATE roles SET scopes = 'patients:read' WHERE name = 'user'`); err != nil {
		t.Fatal(err)
	}

	tok := generateTokenForRole(t, "user", "ro")

	body := asJSON(t, map[string]any{
		"firstName":   "Scope",
		"lastName":    "Test",
		"gender":      "Male",
		"dateOfBirth": "1990-01-01",
		"contact":     "1234567",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/patients", body, tok)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "patients:write") {
		t.Errorf("body should mention required scope, got %s", rec.Body.String())
	}
}

func TestScope_ReadOnlyUserCanReadPatients(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)

	tok := generateTokenForRole(t, "user", "ro")

	rec := doRequest(t, e, http.MethodGet, "/api/patients", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestScope_AdminCanWritePatients(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)

	tok := adminToken(t, e)

	body := asJSON(t, map[string]any{
		"firstName":   "Admin",
		"lastName":    "Created",
		"gender":      "Male",
		"dateOfBirth": "1990-01-01",
		"contact":     "0700000",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/patients", body, tok)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestScope_NoTokenOn404PathStillReturns401(t *testing.T) {
	// Auth middleware runs before the not-found handler, so any /api/* path
	// requires auth, even bogus paths.
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/api/this-route-does-not-exist", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestScope_EmptyScopesUserGetsForbidden(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)

	// Manually craft a role with no scopes and assign to a user.
	if _, err := store.DB.Exec(`INSERT INTO roles (name, label, scopes) VALUES ('empty', 'Empty', '')`); err != nil {
		t.Fatal(err)
	}
	// Update the role check constraint isn't relevant; the users table CHECK
	// only allows super-admin/admin/user. Insert directly bypassing the model.
	// Insert a user via direct SQL since the User.IsValid blocks unknown roles.
	if _, err := store.DB.Exec(
		`INSERT INTO users (id, username, password_hash, display_name, role, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"emptyrole-user-id", "emptyrole", "h", "Empty", "user", 1, store.DateNow(), store.DateNow()); err != nil {
		t.Fatal(err)
	}
	// Generate a token with explicit empty scopes (the JWT carries scopes from
	// auth.GenerateToken, and the auth middleware uses role.Scopes from DB).
	// Easier: update the seeded `user` role to have no scopes and login as
	// that user, but `user` role is seeded earlier. Use AdminToken's flow on
	// a freshly built user with its role's scopes set to empty.
	if _, err := store.DB.Exec(`UPDATE roles SET scopes = '' WHERE name = 'user'`); err != nil {
		t.Fatal(err)
	}
	var u store.User
	if err := u.GetByUsername("emptyrole"); err != nil {
		t.Fatal(err)
	}
	tr, err := auth.GenerateToken(u, []string{})
	if err != nil {
		t.Fatal(err)
	}
	rec := doRequest(t, e, http.MethodGet, "/api/patients", nil, tr.Token)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for empty scopes, got %d", rec.Code)
	}
}
