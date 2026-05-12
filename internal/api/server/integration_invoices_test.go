package server

import (
	"net/http"
	"testing"

	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
)

// firstSeededCurrencyID returns the ID of any seeded currency.
func firstSeededCurrencyID(t *testing.T) string {
	t.Helper()
	var id string
	if err := store.RDB.QueryRow(`SELECT id FROM currencies ORDER BY code LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("first currency: %v", err)
	}
	return id
}

// createPatientAndGetID is a small helper that creates a patient via the API
// and returns the patient ID. phone is padded to a length that passes
// validation.Phone (>=7 chars).
func createPatientAndGetID(t *testing.T, e *echo.Echo, tok, name string) string {
	t.Helper()
	phone := "070-100-200" // valid format, unique not required at API level
	rec := doRequest(t, e, http.MethodPost, "/api/patients",
		asJSON(t, patientPayload(name, "Test", phone)), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create patient: %d %s", rec.Code, rec.Body.String())
	}
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)
	return c.ID
}

func TestClientInvoice_CreateAndDeleteReversesCharge(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "Inv1")

	// Create a client invoice via the public API.
	body := asJSON(t, map[string]any{
		"patientId":  pid,
		"currencyId": curID,
		"notes":      "first visit",
		"items": []map[string]any{
			{"itemType": "other", "quantity": 1, "amount": 50, "notes": "consultation"},
			{"itemType": "other", "quantity": 1, "amount": 25, "notes": "supplies"},
		},
	})
	rec := doRequest(t, e, http.MethodPost, "/api/client-invoices", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invoice: %d %s", rec.Code, rec.Body.String())
	}
	var inv struct {
		ID            string  `json:"id"`
		Amount        float64 `json:"amount"`
		FinalAmount   float64 `json:"finalAmount"`
		FromBalanceID string  `json:"fromBalanceId"`
		ToBalanceID   string  `json:"toBalanceId"`
	}
	decodeEnvelope(t, rec.Body, &inv)
	if inv.ID == "" || !approxEqualF(inv.Amount, 75) || !approxEqualF(inv.FinalAmount, 75) {
		t.Errorf("got %+v", inv)
	}

	// Direction: invoice's FromBalance is self; ToBalance is the patient.
	// CreateWithTx subtracts from FROM, adds to TO. So self goes -75, patient
	// goes +75 (patient now "owes 75" in this codebase's convention).
	patAmount := fetchBalanceAmount(t, inv.ToBalanceID)
	selfAmount := fetchBalanceAmount(t, inv.FromBalanceID)
	if !approxEqualF(patAmount, 75) {
		t.Errorf("patient amount = %v want 75 (charge accrues to patient)", patAmount)
	}
	if !approxEqualF(selfAmount, -75) {
		t.Errorf("self amount = %v want -75", selfAmount)
	}

	// Charge tx persisted.
	if n := countTableRows(t, "balance_transactions",
		"source_type = 'invoice' AND source_id = ? AND voided_at = ''", inv.ID); n != 1 {
		t.Errorf("expected 1 charge tx, got %d", n)
	}

	// Delete the invoice; everything should reverse.
	rec = doRequest(t, e, http.MethodDelete, "/api/client-invoices/"+inv.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete invoice: %d %s", rec.Code, rec.Body.String())
	}

	if !approxEqualF(fetchBalanceAmount(t, inv.ToBalanceID), 0) {
		t.Errorf("patient amount after delete should be 0, got %v", fetchBalanceAmount(t, inv.ToBalanceID))
	}
	if !approxEqualF(fetchBalanceAmount(t, inv.FromBalanceID), 0) {
		t.Errorf("self amount after delete should be 0, got %v", fetchBalanceAmount(t, inv.FromBalanceID))
	}
	if n := countTableRows(t, "balance_transactions",
		"source_type = 'invoice' AND source_id = ? AND voided_at = ''", inv.ID); n != 0 {
		t.Errorf("active charge tx should be gone, got %d", n)
	}
	if n := countTableRows(t, "balance_transactions",
		"source_type = 'invoice' AND source_id = ? AND voided_at != ''", inv.ID); n != 1 {
		t.Errorf("voided charge tx missing, got %d", n)
	}
	if n := countTableRows(t, "invoices", "id = ?", inv.ID); n != 0 {
		t.Errorf("invoice still exists")
	}
}

func TestClientInvoice_Create_ValidationFailures(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	cur := firstSeededCurrencyID(t)

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{
			"missing patientId",
			map[string]any{"currencyId": cur, "items": []any{map[string]any{"amount": 1}}},
			http.StatusBadRequest,
		},
		{
			"missing currencyId",
			map[string]any{"patientId": "x", "items": []any{map[string]any{"amount": 1}}},
			http.StatusBadRequest,
		},
		{
			"empty items",
			map[string]any{"patientId": "x", "currencyId": cur, "items": []any{}},
			http.StatusBadRequest,
		},
		{
			"unknown patient",
			map[string]any{"patientId": "phantom", "currencyId": cur, "items": []any{map[string]any{"amount": 1}}},
			http.StatusBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, e, http.MethodPost, "/api/client-invoices", asJSON(t, tc.body), tok)
			if rec.Code != tc.want {
				t.Errorf("code = %d want %d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestClientInvoice_DeleteNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodDelete, "/api/client-invoices/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestClientInvoice_GetByPatient(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "InvList")

	for i := 0; i < 2; i++ {
		body := asJSON(t, map[string]any{
			"patientId":  pid,
			"currencyId": curID,
			"items":      []map[string]any{{"itemType": "other", "quantity": 1, "amount": 10}},
		})
		rec := doRequest(t, e, http.MethodPost, "/api/client-invoices", body, tok)
		if rec.Code != http.StatusCreated {
			t.Fatalf("seed: %d", rec.Code)
		}
	}

	rec := doRequest(t, e, http.MethodGet, "/api/patients/"+pid+"/invoices", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var list []map[string]any
	decodeEnvelope(t, rec.Body, &list)
	if len(list) != 2 {
		t.Errorf("got %d invoices, want 2", len(list))
	}
}

func TestClientInvoice_UpdateNotes(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "Edit")

	rec := doRequest(t, e, http.MethodPost, "/api/client-invoices",
		asJSON(t, map[string]any{
			"patientId":  pid,
			"currencyId": curID,
			"items":      []map[string]any{{"itemType": "other", "quantity": 1, "amount": 10}},
		}), tok)
	var inv struct{ ID string }
	decodeEnvelope(t, rec.Body, &inv)

	rec = doRequest(t, e, http.MethodPut, "/api/client-invoices/"+inv.ID,
		asJSON(t, map[string]any{"id": "stripped", "invoiceNumber": 999, "notes": "edited"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID            string
		Notes         string
		InvoiceNumber int `json:"invoiceNumber"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Notes != "edited" {
		t.Errorf("Notes = %q", got.Notes)
	}
	if got.ID != inv.ID {
		t.Errorf("ID changed")
	}
	// invoiceNumber must be the original (1), not the spoof value 999.
	if got.InvoiceNumber == 999 {
		t.Errorf("invoiceNumber should not be writable, got %d", got.InvoiceNumber)
	}
}
