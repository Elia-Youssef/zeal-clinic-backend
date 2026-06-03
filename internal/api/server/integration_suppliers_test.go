package server

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

// createSupplier creates a supplier via the API and returns its ID.
func createSupplier(t *testing.T, e *echo.Echo, tok, name string) string {
	t.Helper()
	rec := doRequest(t, e, http.MethodPost, "/api/suppliers",
		asJSON(t, map[string]any{"name": name, "contact": "0700111222", "email": ""}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create supplier: %d %s", rec.Code, rec.Body.String())
	}
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)
	return c.ID
}

func TestCreateSupplier_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/suppliers",
		asJSON(t, map[string]any{
			"name": "Acme Supplies", "contact": "0700111222",
			"email": "acme@example.com", "address": "1 Main St",
		}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var s struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	errMsg, _ := decodeEnvelope(t, rec.Body, &s)
	if errMsg != "" {
		t.Fatalf("error: %s", errMsg)
	}
	if s.ID == "" || s.Name != "Acme Supplies" {
		t.Errorf("got %+v", s)
	}
	// Supplier.Create seeds a balance per currency.
	if n := countTableRows(t, "balances", "entity_type = 'supplier' AND entity_id = ?", s.ID); n < 1 {
		t.Errorf("expected supplier balances to be seeded, got %d", n)
	}
}

func TestCreateSupplier_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Bad email fails Supplier.IsValid.
	rec := doRequest(t, e, http.MethodPost, "/api/suppliers",
		asJSON(t, map[string]any{"name": "Bad", "email": "not-an-email"}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "Please check your input")
}

func TestGetSupplierByID_FoundAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createSupplier(t, e, tok, "Findable")
	rec := doRequest(t, e, http.MethodGet, "/api/suppliers/"+id, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodGet, "/api/suppliers/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateSupplier_PartialAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createSupplier(t, e, tok, "Original")
	rec := doRequest(t, e, http.MethodPut, "/api/suppliers/"+id,
		asJSON(t, map[string]any{"id": "stripped", "name": "Renamed", "notes": "vip"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Notes string `json:"notes"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Name != "Renamed" || got.Notes != "vip" {
		t.Errorf("got %+v", got)
	}
	if got.ID != id {
		t.Errorf("ID changed")
	}

	rec = doRequest(t, e, http.MethodPut, "/api/suppliers/ghost",
		asJSON(t, map[string]any{"name": "X"}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// TestDeleteSupplier_Clean: a supplier that has only seeded (empty) balances and
// no invoices/transactions can be deleted.
func TestDeleteSupplier_Clean(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createSupplier(t, e, tok, "NoActivity")
	rec := doRequest(t, e, http.MethodDelete, "/api/suppliers/"+id, nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "suppliers", "id = ?", id); n != 0 {
		t.Errorf("supplier not deleted")
	}
}

// TestDeleteSupplier_BlockedByTransaction: a supplier with a payment recorded
// against its balance has dependent transactions and cannot be deleted.
func TestDeleteSupplier_BlockedByTransaction(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)

	id := createSupplier(t, e, tok, "Active")
	rec := doRequest(t, e, http.MethodPost, "/api/supplier-payments",
		asJSON(t, map[string]any{
			"supplierId": id, "amount": 100.0, "currencyId": curID,
			"transactionMethod": "transfer", "description": "po-1",
		}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("supplier payment: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/suppliers/"+id, nil, tok)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 (transaction blocks delete), got %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "suppliers", "id = ?", id); n != 1 {
		t.Errorf("supplier should still exist after blocked delete")
	}
}

func TestDeleteSupplier_NotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodDelete, "/api/suppliers/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSupplierListAndDropdown(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	createSupplier(t, e, tok, "ListMe")
	rec := doRequest(t, e, http.MethodGet, "/api/suppliers", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	if page.Total != 1 {
		t.Errorf("total = %d want 1", page.Total)
	}

	rec = doRequest(t, e, http.MethodGet, "/api/suppliers/dropdown", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("dropdown: %d", rec.Code)
	}
}

func TestSuppliers_RequiresAuth(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/api/suppliers", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
