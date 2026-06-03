package server

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

// createProcedureType creates a procedure type and returns its ID.
func createProcedureType(t *testing.T, e *echo.Echo, tok, name string) string {
	t.Helper()
	rec := doRequest(t, e, http.MethodPost, "/api/procedure-types",
		asJSON(t, map[string]any{"name": name, "description": "d"}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create procedure type: %d %s", rec.Code, rec.Body.String())
	}
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)
	return c.ID
}

// createProcedureCategory creates a procedure category and returns its ID.
func createProcedureCategory(t *testing.T, e *echo.Echo, tok, name string) string {
	t.Helper()
	rec := doRequest(t, e, http.MethodPost, "/api/procedure-categories",
		asJSON(t, map[string]any{"name": name}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create procedure category: %d %s", rec.Code, rec.Body.String())
	}
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)
	return c.ID
}

func TestCreateProcedure_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	typeID := createProcedureType(t, e, tok, "Laser")
	catID := createProcedureCategory(t, e, tok, "TestFace")

	body := asJSON(t, map[string]any{
		"name":       "Laser Facial",
		"typeId":     typeID,
		"categoryId": catID,
		"price":      120.0,
		"isActive":   true,
	})
	rec := doRequest(t, e, http.MethodPost, "/api/procedures", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var p struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	errMsg, _ := decodeEnvelope(t, rec.Body, &p)
	if errMsg != "" {
		t.Fatalf("error: %s", errMsg)
	}
	if p.ID == "" || p.Name != "Laser Facial" {
		t.Errorf("got %+v", p)
	}
	// Price lives in procedure_prices (active row).
	if n := countTableRows(t, "procedure_prices", "procedure_id = ? AND is_active = 1", p.ID); n != 1 {
		t.Errorf("expected 1 active price row, got %d", n)
	}
}

func TestCreateProcedure_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/procedures",
		asJSON(t, map[string]any{"name": ""}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "Please check your input")
}

func TestGetProcedureByID_FoundAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/procedures",
		asJSON(t, map[string]any{"name": "Peeling", "price": 50.0}), tok)
	var p struct{ ID string }
	decodeEnvelope(t, rec.Body, &p)

	rec = doRequest(t, e, http.MethodGet, "/api/procedures/"+p.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Price float64 `json:"price"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if !approxEqualF(got.Price, 50) {
		t.Errorf("price = %v want 50 (joined from procedure_prices)", got.Price)
	}

	rec = doRequest(t, e, http.MethodGet, "/api/procedures/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateProcedure_PriceVersioning(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/procedures",
		asJSON(t, map[string]any{"name": "Botox", "price": 200.0}), tok)
	var p struct{ ID string }
	decodeEnvelope(t, rec.Body, &p)

	rec = doRequest(t, e, http.MethodPut, "/api/procedures/"+p.ID,
		asJSON(t, map[string]any{"id": "stripped", "price": 250.0}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID    string  `json:"id"`
		Price float64 `json:"price"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if !approxEqualF(got.Price, 250) {
		t.Errorf("price = %v want 250", got.Price)
	}
	if got.ID != p.ID {
		t.Errorf("ID changed")
	}
	// Old price row deactivated, new one active: 2 total rows, 1 active.
	if n := countTableRows(t, "procedure_prices", "procedure_id = ?", p.ID); n != 2 {
		t.Errorf("expected 2 price rows after change, got %d", n)
	}
	if n := countTableRows(t, "procedure_prices", "procedure_id = ? AND is_active = 1", p.ID); n != 1 {
		t.Errorf("expected 1 active price row, got %d", n)
	}
}

func TestGetProcedurePrices(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/procedures",
		asJSON(t, map[string]any{"name": "Filler", "price": 300.0}), tok)
	var p struct{ ID string }
	decodeEnvelope(t, rec.Body, &p)

	rec = doRequest(t, e, http.MethodGet, "/api/procedures/"+p.ID+"/prices", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("prices: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteProcedure_SuccessAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/procedures",
		asJSON(t, map[string]any{"name": "Doomed", "price": 10.0}), tok)
	var p struct{ ID string }
	decodeEnvelope(t, rec.Body, &p)

	rec = doRequest(t, e, http.MethodDelete, "/api/procedures/"+p.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "procedures", "id = ?", p.ID); n != 0 {
		t.Errorf("procedure not deleted")
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/procedures/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestProcedureDropdownAndList(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Procedures may be seeded by migrations; capture the baseline.
	baseline := countTableRows(t, "procedures", "")
	doRequest(t, e, http.MethodPost, "/api/procedures",
		asJSON(t, map[string]any{"name": "Listed", "price": 1.0}), tok)

	rec := doRequest(t, e, http.MethodGet, "/api/procedures", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	if page.Total != baseline+1 {
		t.Errorf("total = %d want %d (baseline + 1)", page.Total, baseline+1)
	}

	rec = doRequest(t, e, http.MethodGet, "/api/procedures/dropdown", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("dropdown: %d", rec.Code)
	}
}

// procedure types

func TestProcedureType_CRUD(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createProcedureType(t, e, tok, "TypeA")

	rec := doRequest(t, e, http.MethodPut, "/api/procedure-types/"+id,
		asJSON(t, map[string]any{"name": "TypeA2"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Name string `json:"name"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Name != "TypeA2" {
		t.Errorf("name = %q", got.Name)
	}

	rec = doRequest(t, e, http.MethodGet, "/api/procedure-types", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("list: %d", rec.Code)
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/procedure-types/"+id, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d", rec.Code)
	}
	if n := countTableRows(t, "procedure_types", "id = ?", id); n != 0 {
		t.Errorf("type not deleted")
	}
}

func TestProcedureType_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodPost, "/api/procedure-types",
		asJSON(t, map[string]any{"name": ""}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestProcedureType_DuplicateName(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	createProcedureType(t, e, tok, "Unique")

	// Same name (case-insensitive) is rejected with 409.
	rec := doRequest(t, e, http.MethodPost, "/api/procedure-types",
		asJSON(t, map[string]any{"name": "unique"}), tok)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// procedure categories

func TestProcedureCategory_CRUD(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createProcedureCategory(t, e, tok, "CatA")

	rec := doRequest(t, e, http.MethodPut, "/api/procedure-categories/"+id,
		asJSON(t, map[string]any{"name": "CatA2"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodGet, "/api/procedure-categories/dropdown", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("dropdown: %d", rec.Code)
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/procedure-categories/"+id, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d", rec.Code)
	}
}

func TestProcedureCategory_DuplicateName(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	createProcedureCategory(t, e, tok, "Unique")

	rec := doRequest(t, e, http.MethodPost, "/api/procedure-categories",
		asJSON(t, map[string]any{"name": "unique"}), tok)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProcedureCategory_SelfParentRejected(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createProcedureCategory(t, e, tok, "Parentable")

	rec := doRequest(t, e, http.MethodPut, "/api/procedure-categories/"+id,
		asJSON(t, map[string]any{"parentId": id}), tok)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}
