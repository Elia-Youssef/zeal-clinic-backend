package server

import (
	"net/http"
	"strings"
	"testing"

	"clinic-api/internal/database/store"
)

// auditEntries returns all audit_log rows in chronological order. created_at
// has one-second resolution, so ties fall back to the time-ordered id.
func auditEntries(t *testing.T) []store.AuditLogEntry {
	t.Helper()
	rows, err := store.RDB.Query(
		`SELECT a.id, a.user_id, COALESCE(u.username, ''), a.user_role, a.action, a.entity_type, a.entity_id, a.details, a.ip_address, a.created_at
		 FROM audit_log a LEFT JOIN users u ON u.id = a.user_id
		 ORDER BY a.created_at ASC, a.id ASC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []store.AuditLogEntry
	for rows.Next() {
		var e store.AuditLogEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.UserRole, &e.Action, &e.EntityType, &e.EntityID, &e.Details, &e.IPAddress, &e.CreatedAt); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}

func TestAudit_LogsCreateUpdateDelete(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Login should NOT be logged (auth path is excluded).
	if n := countTableRows(t, "audit_log", "entity_type LIKE 'auth%'"); n != 0 {
		t.Errorf("auth path should be skipped, got %d rows", n)
	}

	// CREATE patient
	pid := createPatientAndGetID(t, e, tok, "AuditMe")
	// UPDATE
	rec := doRequest(t, e, http.MethodPut, "/api/patients/"+pid,
		asJSON(t, map[string]any{"firstName": "Updated"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	// DELETE
	rec = doRequest(t, e, http.MethodDelete, "/api/patients/"+pid, nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}

	entries := auditEntries(t)
	if len(entries) != 3 {
		t.Fatalf("expected 3 audit rows (create/update/delete), got %d", len(entries))
	}

	const adminID = "b10829b3-19a0-4813-8e1c-df6f5990737b"
	wantActions := []string{"create", "update", "delete"}
	for i, e := range entries {
		if e.Action != wantActions[i] {
			t.Errorf("entries[%d].Action = %q want %q", i, e.Action, wantActions[i])
		}
		if e.UserID != adminID {
			t.Errorf("entries[%d].UserID = %q want %q", i, e.UserID, adminID)
		}
		if e.UserRole != "super-admin" {
			t.Errorf("entries[%d].UserRole = %q want super-admin", i, e.UserRole)
		}
	}

	// Create should have entity_type = "patients" and the new patient's id, read
	// from the answer (the path /api/patients has none).
	if entries[0].EntityType != "patients" {
		t.Errorf("create entity_type = %q want patients", entries[0].EntityType)
	}
	if entries[0].EntityID != pid {
		t.Errorf("create entity_id = %q want %q", entries[0].EntityID, pid)
	}
	// Update + Delete should set entity_id to the patient ID (path is /api/patients/:id).
	if entries[1].EntityID != pid {
		t.Errorf("update entity_id = %q want %q", entries[1].EntityID, pid)
	}
	if entries[2].EntityID != pid {
		t.Errorf("delete entity_id = %q want %q", entries[2].EntityID, pid)
	}
	// Create body should be in details.
	if !strings.Contains(entries[0].Details, "AuditMe") {
		t.Errorf("create details should include payload, got %q", entries[0].Details)
	}
}

func TestAudit_DoesNotLogGET(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	doRequest(t, e, http.MethodGet, "/api/patients", nil, tok)
	doRequest(t, e, http.MethodGet, "/api/auth/verify", nil, tok)

	if n := countTableRows(t, "audit_log", ""); n != 0 {
		t.Errorf("GET requests should not be audited, got %d rows", n)
	}
}

func TestAudit_DoesNotLogFailedMutation(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Validation failure returns 400. Should NOT be audited.
	rec := doRequest(t, e, http.MethodPost, "/api/patients",
		asJSON(t, map[string]any{"firstName": ""}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code)
	}
	if n := countTableRows(t, "audit_log", ""); n != 0 {
		t.Errorf("4xx mutations should not be audited, got %d", n)
	}

	// A 404 update is not audited either.
	rec = doRequest(t, e, http.MethodPut, "/api/patients/missing",
		asJSON(t, map[string]any{"firstName": "X"}), tok)
	if rec.Code != http.StatusNotFound {
		t.Fatal(rec.Code)
	}
	if n := countTableRows(t, "audit_log", ""); n != 0 {
		t.Errorf("4xx PUT should not be audited, got %d", n)
	}
}

// PATCH is audited like the other writes: a successful salary adjustment
// writes one update row, a refused one writes none.
func TestAudit_LogsPatch(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	empID := createEmployee(t, e, tok, "Patch", "Audit")
	curID := firstSeededCurrencyID(t)
	if rec := doRequest(t, e, http.MethodPost, "/api/employees/"+empID+"/salaries",
		asJSON(t, map[string]any{"amount": 800.0, "currencyId": curID, "effectiveDate": "2026-01-01"}), tok); rec.Code != http.StatusCreated {
		t.Fatalf("seed salary: %d %s", rec.Code, rec.Body.String())
	}
	rec := doRequest(t, e, http.MethodPost, "/api/employee-salaries/prepare",
		asJSON(t, map[string]any{"periodStart": "2026-03-01", "periodEnd": "2026-03-31"}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("prepare: %d body=%s", rec.Code, rec.Body.String())
	}
	var preps []struct {
		ID string `json:"id"`
	}
	decodeEnvelope(t, rec.Body, &preps)
	if len(preps) != 1 {
		t.Fatalf("expected 1 preparation, got %d", len(preps))
	}
	prepPath := "/api/employee-salary-preparations/" + preps[0].ID
	before := len(auditEntries(t))

	rec = doRequest(t, e, http.MethodPatch, prepPath, asJSON(t, map[string]any{"adjustment": -100.0}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("adjust: %d body=%s", rec.Code, rec.Body.String())
	}
	entries := auditEntries(t)
	if len(entries) != before+1 {
		t.Fatalf("2xx PATCH: %d new audit rows, want 1", len(entries)-before)
	}
	last := entries[len(entries)-1]
	if last.Action != "update" || last.EntityType != "employee-salary-preparations" || last.EntityID != preps[0].ID {
		t.Errorf("2xx PATCH row = %s %s %s, want update employee-salary-preparations %s",
			last.Action, last.EntityType, last.EntityID, preps[0].ID)
	}
	if last.UserRole != "super-admin" || !strings.Contains(last.Details, "adjustment") {
		t.Errorf("2xx PATCH row: role %q, details %q", last.UserRole, last.Details)
	}

	// A refused adjustment (below zero) and an unknown preparation are not audited.
	for _, bad := range []struct {
		path string
		want int
	}{
		{prepPath, http.StatusBadRequest},
		{"/api/employee-salary-preparations/missing", http.StatusNotFound},
	} {
		rec = doRequest(t, e, http.MethodPatch, bad.path, asJSON(t, map[string]any{"adjustment": -5000.0}), tok)
		if rec.Code != bad.want {
			t.Fatalf("PATCH %s: %d body=%s, want %d", bad.path, rec.Code, rec.Body.String(), bad.want)
		}
	}
	if n := len(auditEntries(t)); n != before+1 {
		t.Errorf("4xx PATCH: %d new audit rows, want 0", n-before-1)
	}
}

func TestAudit_DoesNotLogAuthOrHealth(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Logout (POST under /api/auth/) should be skipped.
	rec := doRequest(t, e, http.MethodPost, "/api/auth/logout", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	if n := countTableRows(t, "audit_log", ""); n != 0 {
		t.Errorf("auth path should not be audited, got %d", n)
	}
}

func TestAudit_TruncatesLongDetailsTo2000Chars(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Build a long notes string. The audit middleware reads the request body
	// and truncates to 2000 chars.
	huge := strings.Repeat("X", 5000)
	pid := createPatientAndGetID(t, e, tok, "Long")

	rec := doRequest(t, e, http.MethodPut, "/api/patients/"+pid,
		asJSON(t, map[string]any{"notes": huge}), tok)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}

	entries := auditEntries(t)
	last := entries[len(entries)-1]
	if !strings.HasSuffix(last.Details, "...(truncated)") {
		t.Errorf("expected truncation suffix; details length=%d", len(last.Details))
	}
	// 2000 + len("...(truncated)") = 2014.
	if len(last.Details) > 2014 {
		t.Errorf("details too long: %d", len(last.Details))
	}
}

func TestAudit_NestedPathParseEntityType(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	curID := firstSeededCurrencyID(t)
	expID := createExpense(t, e, tok, "AuditNest")

	body := asJSON(t, map[string]any{
		"expenseId":         expID,
		"amount":            10,
		"currencyId":        curID,
		"transactionMethod": "cash",
		"description":       "x",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/expense-payments", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatal(rec.Code)
	}

	entries := auditEntries(t)
	last := entries[len(entries)-1]
	if last.EntityType != "expense-payments" {
		t.Errorf("entity_type = %q want expense-payments", last.EntityType)
	}
}

func TestAudit_IPAddressIsCaptured(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	createPatientAndGetID(t, e, tok, "IPAddr")
	entries := auditEntries(t)
	if len(entries) == 0 {
		t.Fatal("no audit entries")
	}
	if entries[0].IPAddress == "" {
		t.Errorf("IPAddress should be captured (echo RealIP), got empty")
	}
}
