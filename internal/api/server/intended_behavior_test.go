package server

import (
	"net/http"
	"strings"
	"testing"

	"clinic-api/internal/database/store"
)

// Behavior kept on purpose. A failure here means the behavior changed, which
// needs a decision rather than a quick fix.

// An account created without a password takes the first password used to
// sign in to it; later sign-ins must use that password. The seeded
// super-admin works the same way (see TestLogin_FirstLoginCapturesPassword).
func TestAuth_FirstLoginSetsPasswordOfAccountCreatedWithoutOne(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	id := createUserAccount(t, e, tok, "no-password-yet", "staff", "")

	var u store.User
	if err := u.GetByID(id); err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash != "" {
		t.Fatal("the account was created with a password")
	}

	login := func(password string) int {
		t.Helper()
		rec := doRequest(t, e, http.MethodPost, "/api/auth/login",
			asJSON(t, map[string]string{"username": "no-password-yet", "password": password}), "")
		return rec.Code
	}
	if code := login("first-choice"); code != http.StatusOK {
		t.Fatalf("first sign-in: %d, want 200", code)
	}
	if code := login("first-choice"); code != http.StatusOK {
		t.Errorf("second sign-in with the same password: %d, want 200", code)
	}
	if code := login("another-password"); code != http.StatusUnauthorized {
		t.Errorf("sign-in with another password: %d, want 401", code)
	}
}

// The super-admin account stays out of the audit log API and the role
// dropdown (the users and roles lists are covered by their own tests), and
// the role can't be given to anyone.
func TestUsers_SuperAdminHiddenAndNotAssignable(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	superTok := adminToken(t, e)
	adminID := createUserAccount(t, e, superTok, "visible-admin", "admin", "admin-pw")
	adminTok := loginAs(t, e, "visible-admin", "admin-pw")

	createRoom(t, e, superTok, "Room by super-admin", "Procedure")
	createRoom(t, e, adminTok, "Room by admin", "Procedure")
	if n := countTableRows(t, "audit_log", "user_role = 'super-admin'"); n == 0 {
		t.Fatal("the super-admin's write was not audited")
	}

	rec := doRequest(t, e, http.MethodGet, "/api/audit-log", nil, adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("audit log: %d %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []store.AuditLogEntry `json:"items"`
		Total int                   `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	byAdmin := 0
	for _, entry := range page.Items {
		if entry.UserRole == "super-admin" {
			t.Errorf("audit log shows a super-admin entry: %s %s", entry.Action, entry.EntityType)
		}
		if entry.UserID == adminID {
			byAdmin++
		}
	}
	if byAdmin == 0 || page.Total != len(page.Items) {
		t.Errorf("audit log: %d entries by the admin, total %d for %d items", byAdmin, page.Total, len(page.Items))
	}

	rec = doRequest(t, e, http.MethodGet, "/api/roles/dropdown", nil, adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("role dropdown: %d %s", rec.Code, rec.Body.String())
	}
	var roles []store.DropdownItem
	decodeEnvelope(t, rec.Body, &roles)
	if len(roles) == 0 {
		t.Fatal("role dropdown is empty")
	}
	for _, r := range roles {
		if r.ID == "super-admin" {
			t.Error("role dropdown offers super-admin")
		}
	}

	rec = doRequest(t, e, http.MethodPut, "/api/users/"+adminID, asJSON(t, map[string]any{"role": "super-admin"}), superTok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("assign super-admin to a user: %d %s, want 400", rec.Code, rec.Body.String())
	}
	body := employeePayload("Would", "Be")
	body["username"] = "would-be-super"
	body["userRole"] = "super-admin"
	rec = doRequest(t, e, http.MethodPost, "/api/employees", asJSON(t, body), superTok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("create an employee account as super-admin: %d %s, want 400", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "users", "role = 'super-admin'"); n != 1 {
		t.Errorf("%d super-admin accounts, want the seeded one only", n)
	}
}

// Generated files under /files are served to any signed-in user, whatever
// their scopes, and to nobody else: a nurse can't generate the revenue
// report but can download the one generated for an admin.
func TestFiles_GeneratedPDFsReadableByAnySignedInUser(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	superTok := adminToken(t, e)
	createLinkedEmployee(t, e, superTok, "File", "Reader", "file-reader", "nurse", "reader-pw")
	nurseTok := loginAs(t, e, "file-reader", "reader-pw")

	const report = "/api/reports/revenue/pdf?from=2026-05-01&to=2026-05-31"
	if rec := doRequest(t, e, http.MethodGet, report, nil, nurseTok); rec.Code != http.StatusForbidden {
		t.Fatalf("revenue report as the nurse: %d, want 403", rec.Code)
	}
	rec := doRequest(t, e, http.MethodGet, report, nil, superTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("revenue report: %d %s", rec.Code, rec.Body.String())
	}
	var data struct {
		URL string `json:"url"`
	}
	decodeEnvelope(t, rec.Body, &data)
	if !strings.HasPrefix(data.URL, "/files/") {
		t.Fatalf("report url = %q", data.URL)
	}

	rec = doRequest(t, e, http.MethodGet, data.URL, nil, nurseTok)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "%PDF-") {
		t.Errorf("file as the nurse: %d, %d bytes, want 200 and a PDF", rec.Code, rec.Body.Len())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("content type %q", ct)
	}
	rec = doRequest(t, e, http.MethodGet, data.URL, nil, "")
	if got := classify(rec.Code, rec.Body.String()); got != outcomeNoAuth {
		t.Errorf("file without a token: %d, want the sign-in 401", rec.Code)
	}
}

// A product's invoice history needs only products:read, so a nurse, who
// can't list invoices, sees it.
func TestProducts_InvoiceHistoryNeedsOnlyProductsRead(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	superTok := adminToken(t, e)
	productID := createStockProduct(t, e, superTok, "History Gel", 3)
	createLinkedEmployee(t, e, superTok, "Stock", "Viewer", "stock-viewer", "nurse", "viewer-pw")
	nurseTok := loginAs(t, e, "stock-viewer", "viewer-pw")

	if rec := doRequest(t, e, http.MethodGet, "/api/invoices", nil, nurseTok); rec.Code != http.StatusForbidden {
		t.Fatalf("invoice list as the nurse: %d, want 403", rec.Code)
	}
	rec := doRequest(t, e, http.MethodGet, "/api/products/"+productID+"/invoices", nil, nurseTok)
	if rec.Code != http.StatusOK {
		t.Errorf("product invoice history as the nurse: %d %s, want 200", rec.Code, rec.Body.String())
	}
}
