package server

import (
	"clinic-api/internal/database/store"
	"net/http"
	"testing"
)

func TestCreateCurrency_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	body := asJSON(t, map[string]any{
		"code":         "ZED",
		"name":         "Zedland",
		"symbol":       "Z$",
		"exchangeRate": 1.5,
	})
	rec := doRequest(t, e, http.MethodPost, "/api/currencies", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID   string
		Code string
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.ID == "" || got.Code != "ZED" {
		t.Errorf("got %+v", got)
	}
}

func TestCreateCurrency_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/currencies",
		asJSON(t, map[string]any{"code": "", "name": ""}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "Please check your input")
}

func TestUpdateCurrency_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/currencies",
		asJSON(t, map[string]any{"code": "AAA", "name": "Alpha"}), tok)
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)

	rec = doRequest(t, e, http.MethodPut, "/api/currencies/"+c.ID,
		asJSON(t, map[string]any{"id": "stripped", "name": "AlphaX", "exchangeRate": 2.5}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID           string
		Name         string
		ExchangeRate float64
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Name != "AlphaX" || got.ExchangeRate != 2.5 {
		t.Errorf("got %+v", got)
	}
	if got.ID != c.ID {
		t.Errorf("ID changed: %q want %q", got.ID, c.ID)
	}
}

func TestUpdateCurrency_NotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodPut, "/api/currencies/missing",
		asJSON(t, map[string]any{"name": "X"}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteCurrency_BlockedByDependencies(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// USD has the seeded self balance referencing it. Delete should 409.
	rec := doRequest(t, e, http.MethodDelete, "/api/currencies/"+store.USDCurrencyID, nil, tok)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteCurrency_OrphanCanBeDeleted(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/currencies",
		asJSON(t, map[string]any{"code": "ORF", "name": "Orphan"}), tok)
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)

	rec = doRequest(t, e, http.MethodDelete, "/api/currencies/"+c.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "currencies", "id = ?", c.ID); n != 0 {
		t.Errorf("not deleted")
	}
}

func TestDeleteCurrency_NotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodDelete, "/api/currencies/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetAllCurrencies_PaginatedShape(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodGet, "/api/currencies", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	if page.Total < 1 {
		t.Errorf("expected at least 1 seeded currency, got %d", page.Total)
	}
}

func TestGetCurrencyDropdown(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodGet, "/api/currencies/dropdown", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	var items []map[string]any
	decodeEnvelope(t, rec.Body, &items)
	if len(items) < 1 {
		t.Errorf("expected at least 1 dropdown item")
	}
}
