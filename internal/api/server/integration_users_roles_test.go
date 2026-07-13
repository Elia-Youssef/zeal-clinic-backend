package server

import (
	"net/http"
	"testing"

	"clinic-api/internal/database/store"
)

// users

func TestCreateUser_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/users",
		asJSON(t, map[string]any{
			"username": "newuser", "displayName": "New User",
			"role": "staff", "password": "pw-12345",
		}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var u struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		IsActive bool   `json:"isActive"`
	}
	errMsg, _ := decodeEnvelope(t, rec.Body, &u)
	if errMsg != "" {
		t.Fatalf("error: %s", errMsg)
	}
	if u.ID == "" || u.Username != "newuser" {
		t.Errorf("got %+v", u)
	}
	// CreateUser forces IsActive true.
	if !u.IsActive {
		t.Errorf("expected user active")
	}
}

func TestCreateUser_TrimsPassword(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/users",
		asJSON(t, map[string]any{
			"username": "trimmed-create", "displayName": "Trimmed Create",
			"role": "staff", "password": "  pw-12345  ",
		}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodPost, "/api/auth/login",
		asJSON(t, map[string]string{"username": "trimmed-create", "password": "pw-12345"}), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login with trimmed password expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodPost, "/api/users",
		asJSON(t, map[string]any{
			"username": "blank-password", "displayName": "Blank Password",
			"role": "staff", "password": "   ",
		}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("blank-password create expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var blankPasswordUser store.User
	if err := blankPasswordUser.GetByUsername("blank-password"); err != nil {
		t.Fatal(err)
	}
	if blankPasswordUser.PasswordHash != "" {
		t.Error("whitespace-only password should be treated as empty")
	}
}

func TestUpdateUser_TrimsPassword(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/users",
		asJSON(t, map[string]any{
			"username": "trimmed-update", "displayName": "Trimmed Update",
			"role": "staff", "password": "old-pw",
		}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct{ ID string }
	decodeEnvelope(t, rec.Body, &created)

	var before store.User
	if err := before.GetByID(created.ID); err != nil {
		t.Fatal(err)
	}
	rec = doRequest(t, e, http.MethodPut, "/api/users/"+created.ID,
		asJSON(t, map[string]any{"password": "   "}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("whitespace-only update expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var after store.User
	if err := after.GetByID(created.ID); err != nil {
		t.Fatal(err)
	}
	if after.PasswordHash != before.PasswordHash {
		t.Error("whitespace-only password update should leave the password unchanged")
	}

	rec = doRequest(t, e, http.MethodPut, "/api/users/"+created.ID,
		asJSON(t, map[string]any{"password": "  new-pw  "}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("password update expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, e, http.MethodPost, "/api/auth/login",
		asJSON(t, map[string]string{"username": "trimmed-update", "password": "new-pw"}), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login with trimmed updated password expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateUser_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"short username", map[string]any{"username": "ab", "displayName": "X", "role": "staff"}},
		{"missing displayName", map[string]any{"username": "validname", "role": "staff"}},
		{"bad role", map[string]any{"username": "validname", "displayName": "X", "role": "wizard"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, e, http.MethodPost, "/api/users", asJSON(t, tc.body), tok)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGetUserByID_FoundAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/users",
		asJSON(t, map[string]any{"username": "findme", "displayName": "Find Me", "role": "staff"}), tok)
	var u struct{ ID string }
	decodeEnvelope(t, rec.Body, &u)

	rec = doRequest(t, e, http.MethodGet, "/api/users/"+u.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodGet, "/api/users/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateUser_RoleChangeAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/users",
		asJSON(t, map[string]any{"username": "promoteme", "displayName": "Promote", "role": "staff"}), tok)
	var u struct{ ID string }
	decodeEnvelope(t, rec.Body, &u)

	rec = doRequest(t, e, http.MethodPut, "/api/users/"+u.ID,
		asJSON(t, map[string]any{"id": "stripped", "role": "admin"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID   string `json:"id"`
		Role string `json:"role"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Role != "admin" {
		t.Errorf("role = %q want admin", got.Role)
	}
	if got.ID != u.ID {
		t.Errorf("ID changed")
	}

	rec = doRequest(t, e, http.MethodPut, "/api/users/ghost",
		asJSON(t, map[string]any{"role": "admin"}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestGetAllUsers_ListShape(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodGet, "/api/users", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	// At least the seeded admin user exists.
	if page.Total < 1 {
		t.Errorf("expected at least 1 user, got %d", page.Total)
	}
}

// roles

func TestGetAllRoles_ListShape(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodGet, "/api/roles", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	// super-admin/admin/staff/nurse are seeded, but super-admin is hidden (dev/support only).
	if page.Total < 3 {
		t.Errorf("expected at least 3 visible roles, got %d", page.Total)
	}
	for _, r := range page.Items {
		if r["name"] == "super-admin" {
			t.Error("super-admin role should not be listed")
		}
	}
}

func TestGetRoleByName_FoundAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodGet, "/api/roles/admin", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodGet, "/api/roles/nonexistent", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateRole_ScopesAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPut, "/api/roles/staff",
		asJSON(t, map[string]any{"name": "stripped", "scopes": []string{"patients:read"}}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodPut, "/api/roles/nonexistent",
		asJSON(t, map[string]any{"scopes": []string{"patients:read"}}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateRole_RejectsUnknownScope(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPut, "/api/roles/staff",
		asJSON(t, map[string]any{"scopes": []string{"patients:read", "patient:read"}}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown scope, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateRole_AdminCantDropManagementScopes(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPut, "/api/roles/admin",
		asJSON(t, map[string]any{"scopes": []string{"patients:read"}}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRole_SuperAdminNotReachable(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodGet, "/api/roles/super-admin", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET super-admin: expected 404, got %d", rec.Code)
	}

	rec = doRequest(t, e, http.MethodPut, "/api/roles/super-admin",
		asJSON(t, map[string]any{"scopes": []string{"patients:read"}}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("PUT super-admin: expected 404, got %d", rec.Code)
	}
}

func TestRoleDropdown(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodGet, "/api/roles/dropdown", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("dropdown: %d", rec.Code)
	}
}

func TestUsers_RequiresAuth(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/api/users", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
