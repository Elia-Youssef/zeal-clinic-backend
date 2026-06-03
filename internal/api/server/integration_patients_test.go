package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

// patientPayload returns a minimal valid CreatePatient body.
func patientPayload(first, last, phone string) map[string]any {
	return map[string]any{
		"firstName":   first,
		"lastName":    last,
		"gender":      "Female",
		"dateOfBirth": "1990-01-01",
		"contact":     phone,
	}
}

func TestCreatePatient_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/patients",
		asJSON(t, patientPayload("Anne", "Smith", "0700100100")), tok)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var data struct {
		ID        string `json:"id"`
		FirstName string `json:"firstName"`
	}
	errMsg, _ := decodeEnvelope(t, rec.Body, &data)
	if errMsg != "" {
		t.Fatalf("error in envelope: %s", errMsg)
	}
	if data.ID == "" || data.FirstName != "Anne" {
		t.Errorf("got %+v", data)
	}

	// DB row exists.
	if n := countTableRows(t, "patients", "first_name = ?", "Anne"); n != 1 {
		t.Errorf("expected 1 patient, got %d", n)
	}
}

func TestCreatePatient_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	bad := map[string]any{
		"firstName":   "",
		"lastName":    "",
		"gender":      "Other",
		"dateOfBirth": "not-a-date",
		"contact":     "abc",
	}
	rec := doRequest(t, e, http.MethodPost, "/api/patients", asJSON(t, bad), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "Please check your input")
}

func TestCreatePatient_BadJSON(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/patients", []byte(`{"firstName":`), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetPatientByID_Found(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/patients",
		asJSON(t, patientPayload("Bob", "Jones", "0700200200")), tok)
	if rec.Code != http.StatusCreated {
		t.Fatal(rec.Code)
	}
	var created struct{ ID string }
	decodeEnvelope(t, rec.Body, &created)

	rec = doRequest(t, e, http.MethodGet, "/api/patients/"+created.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetPatientByID_NotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodGet, "/api/patients/does-not-exist", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetAllPatients_PaginatedListShape(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	for i := 0; i < 3; i++ {
		rec := doRequest(t, e, http.MethodPost, "/api/patients",
			asJSON(t, patientPayload("First"+string(rune('A'+i)), "Last", "070010010"+string(rune('0'+i)))), tok)
		if rec.Code != http.StatusCreated {
			t.Fatalf("seed %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}

	rec := doRequest(t, e, http.MethodGet, "/api/patients", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	if page.Total != 3 {
		t.Errorf("Total = %d want 3", page.Total)
	}
	if len(page.Items) != 3 {
		t.Errorf("Items len = %d want 3", len(page.Items))
	}
}

func TestUpdatePatient_PartialUpdate(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/patients",
		asJSON(t, patientPayload("Carol", "Original", "0700300300")), tok)
	var created struct{ ID string }
	decodeEnvelope(t, rec.Body, &created)

	body := asJSON(t, map[string]any{
		"id":        "should-be-stripped",
		"createdAt": "should-be-stripped",
		"firstName": "Updated",
		"email":     "updated@example.com",
	})
	rec = doRequest(t, e, http.MethodPut, "/api/patients/"+created.ID, body, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID        string `json:"id"`
		FirstName string `json:"firstName"`
		Email     string `json:"email"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.FirstName != "Updated" || got.Email != "updated@example.com" {
		t.Errorf("got %+v", got)
	}
	if got.ID != created.ID {
		t.Errorf("ID changed: %q want %q", got.ID, created.ID)
	}
}

func TestUpdatePatient_NotFound(t *testing.T) {
	// Note: store.Patient.Update with no recognized keys returns the result of
	// GetByID, which IS ErrNotFound, but with at least one recognized key it
	// runs the UPDATE which silently affects 0 rows, then GetByID returns
	// ErrNotFound. Either way the handler should return 404.
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	body := asJSON(t, map[string]any{"firstName": "Nope"})
	rec := doRequest(t, e, http.MethodPut, "/api/patients/missing", body, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeletePatient_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/patients",
		asJSON(t, patientPayload("ToDelete", "Patient", "0700400400")), tok)
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)

	rec = doRequest(t, e, http.MethodDelete, "/api/patients/"+c.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "patients", "id = ?", c.ID); n != 0 {
		t.Errorf("expected patient deleted, got %d rows", n)
	}
}

func TestDeletePatient_NotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodDelete, "/api/patients/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestDeletePatient_DependencyConflict(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Create patient.
	rec := doRequest(t, e, http.MethodPost, "/api/patients",
		asJSON(t, patientPayload("HasDeps", "Patient", "0700500500")), tok)
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)

	// Need procedure + room + appointment rows first (FK chain).
	if _, err := getStoreDB().Exec(
		`INSERT INTO procedures (id, name, type_id, category_id, price_note, is_active, remarks, includes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"proc-dep", "Dep Proc", "", "", "", 1, "", "", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z"); err != nil {
		t.Fatalf("seed procedure: %v", err)
	}
	if _, err := getStoreDB().Exec(
		`INSERT INTO rooms (id, name, type, created_at) VALUES (?, ?, ?, ?)`,
		"room-dep", "Dep Room", "Procedure", "2024-01-01T00:00:00Z"); err != nil {
		t.Fatalf("seed room: %v", err)
	}
	if _, err := getStoreDB().Exec(
		`INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status, notes, cancel_notes, completion_notes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"apt-dep", c.ID, "room-dep", "2024-01-01T10:00:00Z", "2024-01-01T11:00:00Z", "Scheduled", "", "", "", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z"); err != nil {
		t.Fatalf("seed appointment: %v", err)
	}
	if _, err := getStoreDB().Exec(
		`INSERT INTO appointment_procedures (id, patient_id, procedure_id, appointment_id, notes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"ap-1", c.ID, "proc-dep", "apt-dep", "", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z"); err != nil {
		t.Fatalf("seed appointment_procedures: %v", err)
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/patients/"+c.ID, nil, tok)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPatientDropdown(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	doRequest(t, e, http.MethodPost, "/api/patients",
		asJSON(t, patientPayload("DropD", "Test", "0700600600")), tok)

	rec := doRequest(t, e, http.MethodGet, "/api/patients/dropdown", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var items []map[string]any
	decodeEnvelope(t, rec.Body, &items)
	found := false
	for _, it := range items {
		if it["name"] == "DropD Test" {
			found = true
		}
	}
	if !found {
		raw, _ := json.Marshal(items)
		t.Errorf("dropdown missing entry: %s", raw)
	}
}
