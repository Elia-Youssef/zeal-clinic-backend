package server

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

// createProductCategory creates a product category and returns its ID.
func createProductCategory(t *testing.T, e *echo.Echo, tok, name string) string {
	t.Helper()
	rec := doRequest(t, e, http.MethodPost, "/api/product-categories",
		asJSON(t, map[string]any{"name": name}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create product category: %d %s", rec.Code, rec.Body.String())
	}
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)
	return c.ID
}

func TestCreateProduct_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	catID := createProductCategory(t, e, tok, "Consumables")
	body := asJSON(t, map[string]any{
		"name":         "Gloves",
		"categoryId":   catID,
		"quantity":     100,
		"minThreshold": 10,
		"unitPrice":    5.0,
	})
	rec := doRequest(t, e, http.MethodPost, "/api/products", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var p struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		UnitPrice float64 `json:"unitPrice"`
	}
	errMsg, _ := decodeEnvelope(t, rec.Body, &p)
	if errMsg != "" {
		t.Fatalf("error: %s", errMsg)
	}
	if p.ID == "" || p.Name != "Gloves" {
		t.Errorf("got %+v", p)
	}
	if n := countTableRows(t, "product_prices", "product_id = ? AND is_active = 1", p.ID); n != 1 {
		t.Errorf("expected 1 active price row, got %d", n)
	}
}

func TestCreateProduct_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Missing name and non-positive unitPrice.
	rec := doRequest(t, e, http.MethodPost, "/api/products",
		asJSON(t, map[string]any{"name": "", "unitPrice": 0}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "Please check your input")
}

func TestGetProductByID_FoundAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/products",
		asJSON(t, map[string]any{"name": "Syringe", "quantity": 5, "unitPrice": 2.0}), tok)
	var p struct{ ID string }
	decodeEnvelope(t, rec.Body, &p)

	rec = doRequest(t, e, http.MethodGet, "/api/products/"+p.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodGet, "/api/products/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateProduct_QuantityAndPrice(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/products",
		asJSON(t, map[string]any{"name": "Mask", "quantity": 50, "unitPrice": 1.0}), tok)
	var p struct{ ID string }
	decodeEnvelope(t, rec.Body, &p)

	rec = doRequest(t, e, http.MethodPut, "/api/products/"+p.ID,
		asJSON(t, map[string]any{"id": "stripped", "quantity": 75, "unitPrice": 1.5}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID        string  `json:"id"`
		Quantity  int     `json:"quantity"`
		UnitPrice float64 `json:"unitPrice"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Quantity != 75 || !approxEqualF(got.UnitPrice, 1.5) {
		t.Errorf("got %+v", got)
	}
	if got.ID != p.ID {
		t.Errorf("ID changed")
	}
}

// TestUpdateProduct_NegativeQuantityAllowed captures the intentional behavior
// that Product allows negative stock (see also store/product_test.go).
func TestUpdateProduct_NegativeQuantityAllowed(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/products",
		asJSON(t, map[string]any{"name": "Negable", "quantity": 1, "unitPrice": 1.0}), tok)
	var p struct{ ID string }
	decodeEnvelope(t, rec.Body, &p)

	rec = doRequest(t, e, http.MethodPut, "/api/products/"+p.ID,
		asJSON(t, map[string]any{"quantity": -5}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Quantity int `json:"quantity"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Quantity != -5 {
		t.Errorf("quantity = %d want -5 (negative stock is intentional)", got.Quantity)
	}
}

// TestDeleteProduct_Clean: a product with no invoice references deletes cleanly.
func TestDeleteProduct_Clean(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/products",
		asJSON(t, map[string]any{"name": "Disposable", "quantity": 1, "unitPrice": 1.0}), tok)
	var p struct{ ID string }
	decodeEnvelope(t, rec.Body, &p)

	rec = doRequest(t, e, http.MethodDelete, "/api/products/"+p.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "products", "id = ?", p.ID); n != 0 {
		t.Errorf("product not deleted")
	}
}

// TestDeleteProduct_BlockedByInvoiceItem: a product sold on an invoice is
// referenced by an invoice_items row (item_id), so DeleteProduct returns 409.
func TestDeleteProduct_BlockedByInvoiceItem(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "Buyer")

	rec := doRequest(t, e, http.MethodPost, "/api/products",
		asJSON(t, map[string]any{"name": "Sold", "quantity": 10, "unitPrice": 5.0}), tok)
	var prod struct{ ID string }
	decodeEnvelope(t, rec.Body, &prod)

	// Sell the product on a client invoice, creating an invoice_items row whose
	// item_id points at the product.
	rec = doRequest(t, e, http.MethodPost, "/api/client-invoices",
		asJSON(t, map[string]any{
			"patientId":  pid,
			"currencyId": curID,
			"items": []map[string]any{
				{"itemType": "product", "itemId": prod.ID, "quantity": 1, "amount": 5},
			},
		}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invoice: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/products/"+prod.ID, nil, tok)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 (referenced by invoice item), got %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "products", "id = ?", prod.ID); n != 1 {
		t.Errorf("product should still exist after blocked delete")
	}
}

func TestDeleteProduct_NotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodDelete, "/api/products/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProductDropdownListAndPrices(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/products",
		asJSON(t, map[string]any{"name": "Listed", "quantity": 1, "unitPrice": 1.0}), tok)
	var p struct{ ID string }
	decodeEnvelope(t, rec.Body, &p)

	rec = doRequest(t, e, http.MethodGet, "/api/products", nil, tok)
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

	rec = doRequest(t, e, http.MethodGet, "/api/products/dropdown", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("dropdown: %d", rec.Code)
	}

	rec = doRequest(t, e, http.MethodGet, "/api/products/"+p.ID+"/prices", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("prices: %d body=%s", rec.Code, rec.Body.String())
	}
}

// product categories

func TestProductCategory_CRUD(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createProductCategory(t, e, tok, "CatA")

	rec := doRequest(t, e, http.MethodPut, "/api/product-categories/"+id,
		asJSON(t, map[string]any{"name": "CatA2"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodGet, "/api/product-categories", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("list: %d", rec.Code)
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/product-categories/"+id, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d", rec.Code)
	}
}

func TestProductCategory_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodPost, "/api/product-categories",
		asJSON(t, map[string]any{"name": ""}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestProductCategory_DuplicateName(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	createProductCategory(t, e, tok, "Unique")

	rec := doRequest(t, e, http.MethodPost, "/api/product-categories",
		asJSON(t, map[string]any{"name": "unique"}), tok)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProductCategory_SelfParentRejected(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createProductCategory(t, e, tok, "Parentable")

	rec := doRequest(t, e, http.MethodPut, "/api/product-categories/"+id,
		asJSON(t, map[string]any{"parentId": id}), tok)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}
