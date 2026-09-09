package server

import (
	"net/http"
	"slices"
	"sort"
	"testing"
	"time"

	"clinic-api/internal/realtime"
)

// expectEvent waits briefly for a realtime event of the given type.
func expectEvent(t *testing.T, c *realtime.Client, eventType string) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-c.Events():
			if !ok {
				t.Fatalf("event stream closed before %q", eventType)
			}
			if ev.Type == eventType {
				return
			}
		case <-timeout:
			t.Fatalf("no %q event", eventType)
		}
	}
}

// Deactivating a user ends all of their sessions at once and tells their
// open tabs.
func TestSession_DeactivationRevokesTokensAndNotifies(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	superTok := adminToken(t, e)
	id := createUserAccount(t, e, superTok, "leaving-user", "staff", "leaving-pw")
	first := loginAs(t, e, "leaving-user", "leaving-pw")
	second := loginAs(t, e, "leaving-user", "leaving-pw")
	events := realtime.Register(id)
	defer events.Close()

	rec := doRequest(t, e, http.MethodPut, "/api/users/"+id, asJSON(t, map[string]any{"isActive": false}), superTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("deactivate: %d %s", rec.Code, rec.Body.String())
	}
	expectEvent(t, events, "account_disabled")
	for i, tok := range []string{first, second} {
		if rec := doRequest(t, e, http.MethodGet, "/api/auth/verify", nil, tok); rec.Code != http.StatusUnauthorized {
			t.Errorf("session %d after deactivation: %d, want 401", i+1, rec.Code)
		}
	}
	if n := countTableRows(t, "tokens", "user_id = ?", id); n != 0 {
		t.Errorf("%d tokens left for the deactivated user", n)
	}
	rec = doRequest(t, e, http.MethodPost, "/api/auth/login",
		asJSON(t, map[string]string{"username": "leaving-user", "password": "leaving-pw"}), "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("sign-in after deactivation: %d, want 403", rec.Code)
	}
}

// A change to a role's scopes applies to its users' next request, with the
// token they already hold, and tells their open tabs.
func TestSession_RoleScopeChangeAppliesOnNextRequest(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	superTok := adminToken(t, e)
	id := createUserAccount(t, e, superTok, "role-watcher", "staff", "watcher-pw")
	tok := loginAs(t, e, "role-watcher", "watcher-pw")
	events := realtime.Register(id)
	defer events.Close()

	if rec := doRequest(t, e, http.MethodGet, "/api/suppliers", nil, tok); rec.Code != http.StatusOK {
		t.Fatalf("suppliers before the change: %d, want 200", rec.Code)
	}
	scopes := slices.DeleteFunc(roleScopes(t)["staff"], func(s string) bool { return s == "suppliers:read" })
	rec := doRequest(t, e, http.MethodPut, "/api/roles/staff", asJSON(t, map[string]any{"scopes": scopes}), superTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update role: %d %s", rec.Code, rec.Body.String())
	}
	expectEvent(t, events, "scopes_changed")
	if rec := doRequest(t, e, http.MethodGet, "/api/suppliers", nil, tok); rec.Code != http.StatusForbidden {
		t.Errorf("suppliers after the change: %d, want 403", rec.Code)
	}
}

// Moving a user to another role applies to their next request and tells
// their open tabs.
func TestSession_UserRoleChangeAppliesOnNextRequest(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	superTok := adminToken(t, e)
	id := createUserAccount(t, e, superTok, "moving-user", "staff", "moving-pw")
	tok := loginAs(t, e, "moving-user", "moving-pw")
	events := realtime.Register(id)
	defer events.Close()

	if rec := doRequest(t, e, http.MethodGet, "/api/suppliers", nil, tok); rec.Code != http.StatusOK {
		t.Fatalf("suppliers as staff: %d, want 200", rec.Code)
	}
	rec := doRequest(t, e, http.MethodPut, "/api/users/"+id, asJSON(t, map[string]any{"role": "nurse"}), superTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("change role: %d %s", rec.Code, rec.Body.String())
	}
	expectEvent(t, events, "scopes_changed")
	if rec := doRequest(t, e, http.MethodGet, "/api/suppliers", nil, tok); rec.Code != http.StatusForbidden {
		t.Errorf("suppliers as nurse: %d, want 403", rec.Code)
	}
}

// The last active admin can't be demoted or deactivated, and an admin can't
// change their own role or deactivate themselves.
func TestSession_AdminLockoutProtections(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	superTok := adminToken(t, e)
	adminID := createUserAccount(t, e, superTok, "only-admin", "admin", "only-pw")
	adminTok := loginAs(t, e, "only-admin", "only-pw")

	put := func(tok string, body map[string]any) (int, string) {
		t.Helper()
		rec := doRequest(t, e, http.MethodPut, "/api/users/"+adminID, asJSON(t, body), tok)
		msg, _ := decodeEnvelope(t, rec.Body, nil)
		return rec.Code, msg
	}
	for _, body := range []map[string]any{{"role": "staff"}, {"isActive": false}} {
		if code, msg := put(superTok, body); code != http.StatusBadRequest || msg != "Can't remove the last active admin" {
			t.Errorf("%v on the last active admin: %d %q", body, code, msg)
		}
	}
	if code, msg := put(adminTok, map[string]any{"role": "staff"}); code != http.StatusBadRequest || msg != "You can't change your own role" {
		t.Errorf("own role change: %d %q", code, msg)
	}
	if code, msg := put(adminTok, map[string]any{"isActive": false}); code != http.StatusBadRequest || msg != "You can't deactivate your own account" {
		t.Errorf("own deactivation: %d %q", code, msg)
	}

	createUserAccount(t, e, superTok, "second-admin", "admin", "second-pw")
	if code, msg := put(superTok, map[string]any{"role": "staff"}); code != http.StatusOK {
		t.Errorf("demote with another active admin left: %d %q, want 200", code, msg)
	}
}

// Search returns only the kinds of records the caller may read.
func TestSearch_ResultsFilteredByScope(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	superTok := adminToken(t, e)
	createPatientAndGetID(t, e, superTok, "Quokka")
	createSupplier(t, e, superTok, "Quokka Supplies")
	createStockProduct(t, e, superTok, "Quokka Gel", 1)
	createEmployee(t, e, superTok, "Quokka", "Keeper")
	createLinkedEmployee(t, e, superTok, "Search", "Staff", "search-staff", "staff", "staff-pw")
	createLinkedEmployee(t, e, superTok, "Search", "Nurse", "search-nurse", "nurse", "nurse-pw")

	for _, c := range []struct {
		name string
		tok  string
		want []string
	}{
		{"super-admin", superTok, []string{"employees", "patients", "procedures", "products", "suppliers"}},
		{"staff", loginAs(t, e, "search-staff", "staff-pw"), []string{"patients", "procedures", "products", "suppliers"}},
		{"nurse", loginAs(t, e, "search-nurse", "nurse-pw"), []string{"patients", "procedures", "products"}},
	} {
		rec := doRequest(t, e, http.MethodGet, "/api/search?q=Quokka", nil, c.tok)
		if rec.Code != http.StatusOK {
			t.Fatalf("search as %s: %d %s", c.name, rec.Code, rec.Body.String())
		}
		var results map[string][]map[string]any
		decodeEnvelope(t, rec.Body, &results)
		var got []string
		for table, hits := range results {
			got = append(got, table)
			if table != "procedures" && len(hits) == 0 {
				t.Errorf("search as %s: no %s hit", c.name, table)
			}
		}
		sort.Strings(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("search as %s: result kinds %v, want %v", c.name, got, c.want)
		}
	}
}

// The money figures on the analytics page need reports:read on top of the
// analytics and balance scopes; the other sections don't. Scopes are read
// per request, so the change applies to the same token.
func TestAnalytics_FinancialSectionsNeedReportsRead(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	superTok := adminToken(t, e)
	createLinkedEmployee(t, e, superTok, "Numbers", "Viewer", "numbers-viewer", "nurse", "numbers-pw")
	tok := loginAs(t, e, "numbers-viewer", "numbers-pw")

	financial := []string{
		"/api/analytics/money",
		"/api/analytics/transactions/recent",
		"/api/analytics/series?metric=revenue",
	}
	check := func(path string, want int) {
		t.Helper()
		if rec := doRequest(t, e, http.MethodGet, path, nil, tok); rec.Code != want {
			t.Errorf("%s: %d %s, want %d", path, rec.Code, rec.Body.String(), want)
		}
	}
	setRoleScopes(t, "nurse", "analytics:read", "balances:read", "patients:read")
	for _, p := range financial {
		check(p, http.StatusForbidden)
	}
	check("/api/analytics/patients", http.StatusOK)

	setRoleScopes(t, "nurse", "analytics:read", "balances:read", "patients:read", "reports:read")
	for _, p := range financial {
		check(p, http.StatusOK)
	}
}
