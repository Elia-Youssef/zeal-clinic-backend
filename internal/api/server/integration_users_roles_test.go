package server

import (
	"net/http"
	"testing"
)

// users

func TestCreateUser_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/users",
		asJSON(t, map[string]any{
			"username": "newuser", "displayName": "New User",
			"role": "user", "password": "pw-12345",
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

func TestCreateUser_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"short username", map[string]any{"username": "ab", "displayName": "X", "role": "user"}},
		{"missing displayName", map[string]any{"username": "validname", "role": "user"}},
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
		asJSON(t, map[string]any{"username": "findme", "displayName": "Find Me", "role": "user"}), tok)
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
		asJSON(t, map[string]any{"username": "promoteme", "displayName": "Promote", "role": "user"}), tok)
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
	// super-admin/admin/user are seeded.
	if page.Total < 3 {
		t.Errorf("expected at least 3 seeded roles, got %d", page.Total)
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

	rec := doRequest(t, e, http.MethodPut, "/api/roles/user",
		asJSON(t, map[string]any{"name": "stripped", "scopes": []string{"patients:read"}}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodPut, "/api/roles/nonexistent",
		asJSON(t, map[string]any{"scopes": []string{"x"}}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
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
