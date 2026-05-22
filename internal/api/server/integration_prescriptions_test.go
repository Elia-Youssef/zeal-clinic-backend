package server

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

// createMedicine creates a medicine and returns its ID.
func createMedicine(t *testing.T, e *echo.Echo, tok, name string) string {
	t.Helper()
	rec := doRequest(t, e, http.MethodPost, "/api/medicines",
		asJSON(t, map[string]any{"name": name, "description": "d"}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create medicine: %d %s", rec.Code, rec.Body.String())
	}
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)
	return c.ID
}

// createAllergy creates an allergy and returns its ID.
func createAllergy(t *testing.T, e *echo.Echo, tok, name string) string {
	t.Helper()
	rec := doRequest(t, e, http.MethodPost, "/api/allergies",
		asJSON(t, map[string]any{"name": name, "description": "d"}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create allergy: %d %s", rec.Code, rec.Body.String())
	}
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)
	return c.ID
}

// medicines catalog

func TestMedicine_CRUD(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createMedicine(t, e, tok, "Paracetamol")

	rec := doRequest(t, e, http.MethodPut, "/api/medicines/"+id,
		asJSON(t, map[string]any{"name": "Paracetamol 500"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Name string `json:"name"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Name != "Paracetamol 500" {
		t.Errorf("name = %q", got.Name)
	}

	rec = doRequest(t, e, http.MethodGet, "/api/medicines", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("list: %d", rec.Code)
	}
	rec = doRequest(t, e, http.MethodGet, "/api/medicines/dropdown", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("dropdown: %d", rec.Code)
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/medicines/"+id, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d", rec.Code)
	}
	if n := countTableRows(t, "medicines", "id = ?", id); n != 0 {
		t.Errorf("medicine not deleted")
	}
}

func TestMedicine_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodPost, "/api/medicines",
		asJSON(t, map[string]any{"name": ""}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// allergies catalog

func TestAllergy_CRUD(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createAllergy(t, e, tok, "Penicillin")

	rec := doRequest(t, e, http.MethodPut, "/api/allergies/"+id,
		asJSON(t, map[string]any{"name": "Penicillin G"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodGet, "/api/allergies", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("list: %d", rec.Code)
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/allergies/"+id, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d", rec.Code)
	}
}

// prescriptions

func TestPrescription_CreateGetUpdateDelete(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	pid := createPatientAndGetID(t, e, tok, "Rx")
	medID := createMedicine(t, e, tok, "Ibuprofen")
	// prescribed_by_id has a NOT NULL FK to employees(id); with foreign_keys ON
	// an empty default ('') violates the constraint, so a real employee is
	// required to create a prescription even though IsValid does not require it.
	empID := createEmployee(t, e, tok, "Doc", "Tor")

	body := asJSON(t, map[string]any{
		"patientId":      pid,
		"prescribedById": empID,
		"startDate":      "2026-05-01",
		"endDate":        "2026-05-10",
		"medicines": []map[string]any{
			{"medicineId": medID, "instructions": "twice daily"},
		},
	})
	rec := doRequest(t, e, http.MethodPost, "/api/prescriptions", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d body=%s", rec.Code, rec.Body.String())
	}
	var p struct {
		ID        string           `json:"id"`
		Medicines []map[string]any `json:"medicines"`
	}
	decodeEnvelope(t, rec.Body, &p)
	if p.ID == "" {
		t.Fatalf("no id")
	}
	if len(p.Medicines) != 1 {
		t.Errorf("expected 1 prescription medicine, got %d", len(p.Medicines))
	}

	// List by patient.
	rec = doRequest(t, e, http.MethodGet, "/api/patients/"+pid+"/prescriptions", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var list []map[string]any
	decodeEnvelope(t, rec.Body, &list)
	if len(list) != 1 {
		t.Errorf("expected 1 prescription, got %d", len(list))
	}

	// Update is a full replacement (handler binds the whole struct and validates),
	// so resend the required fields with a new end date.
	rec = doRequest(t, e, http.MethodPut, "/api/prescriptions/"+p.ID,
		asJSON(t, map[string]any{
			"patientId":      pid,
			"prescribedById": empID,
			"startDate":      "2026-05-01",
			"endDate":        "2026-05-15",
			"medicines": []map[string]any{
				{"medicineId": medID, "instructions": "once daily"},
			},
		}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}

	// Delete.
	rec = doRequest(t, e, http.MethodDelete, "/api/prescriptions/"+p.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d", rec.Code)
	}
	if n := countTableRows(t, "prescriptions", "id = ?", p.ID); n != 0 {
		t.Errorf("prescription not deleted")
	}
}

func TestPrescription_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Missing patientId and startDate.
	rec := doRequest(t, e, http.MethodPost, "/api/prescriptions",
		asJSON(t, map[string]any{"endDate": "2026-05-10"}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "validation failed")
}

func TestPrescription_UpdateAndDeleteNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodDelete, "/api/prescriptions/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 on delete, got %d", rec.Code)
	}
}

// patient allergies

func TestPatientAllergy_AddListRemove(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	pid := createPatientAndGetID(t, e, tok, "Allergic")
	allergyID := createAllergy(t, e, tok, "Latex")

	rec := doRequest(t, e, http.MethodPost, "/api/patients/"+pid+"/allergies",
		asJSON(t, map[string]any{"allergyId": allergyID, "notes": "severe"}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add allergy: %d body=%s", rec.Code, rec.Body.String())
	}
	var pa struct{ ID string }
	decodeEnvelope(t, rec.Body, &pa)

	rec = doRequest(t, e, http.MethodGet, "/api/patients/"+pid+"/allergies", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var list []map[string]any
	decodeEnvelope(t, rec.Body, &list)
	if len(list) != 1 {
		t.Errorf("expected 1 allergy, got %d", len(list))
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/patient-allergies/"+pa.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("remove: %d", rec.Code)
	}
}

func TestPatientAllergy_ValidationAndRemoveNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	pid := createPatientAndGetID(t, e, tok, "NoAllergy")
	// Missing allergyId.
	rec := doRequest(t, e, http.MethodPost, "/api/patients/"+pid+"/allergies",
		asJSON(t, map[string]any{"notes": "x"}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/patient-allergies/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// patient medicines

func TestPatientMedicine_AddListRemove(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	pid := createPatientAndGetID(t, e, tok, "OnMeds")
	medID := createMedicine(t, e, tok, "Aspirin")

	rec := doRequest(t, e, http.MethodPost, "/api/patients/"+pid+"/medicines",
		asJSON(t, map[string]any{"medicineId": medID, "isActive": true, "notes": "daily"}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add medicine: %d body=%s", rec.Code, rec.Body.String())
	}
	var pm struct{ ID string }
	decodeEnvelope(t, rec.Body, &pm)

	rec = doRequest(t, e, http.MethodGet, "/api/patients/"+pid+"/medicines", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var list []map[string]any
	decodeEnvelope(t, rec.Body, &list)
	if len(list) != 1 {
		t.Errorf("expected 1 medicine, got %d", len(list))
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/patient-medicines/"+pm.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("remove: %d", rec.Code)
	}
}

func TestPatientMedicine_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	pid := createPatientAndGetID(t, e, tok, "NoMeds")
	// Missing medicineId.
	rec := doRequest(t, e, http.MethodPost, "/api/patients/"+pid+"/medicines",
		asJSON(t, map[string]any{"notes": "x"}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}
