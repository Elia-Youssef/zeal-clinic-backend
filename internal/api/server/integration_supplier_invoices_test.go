package server

import (
	"net/http"
	"testing"

	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
)

func createStockProduct(t *testing.T, e *echo.Echo, tok, name string, quantity int) string {
	t.Helper()
	rec := doRequest(t, e, http.MethodPost, "/api/products", asJSON(t, map[string]any{
		"name": name, "quantity": quantity, "unitPrice": 5,
	}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create product: %d body=%s", rec.Code, rec.Body.String())
	}
	var product struct {
		ID string `json:"id"`
	}
	decodeEnvelope(t, rec.Body, &product)
	return product.ID
}

func TestDeleteSupplierInvoice_RejectsNegativeStockReversal(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	supplierID := createSupplier(t, e, tok, "Stock Supplier")
	productID := createStockProduct(t, e, tok, "Stocked Item", 0)

	rec := doRequest(t, e, http.MethodPost, "/api/supplier-invoices", asJSON(t, map[string]any{
		"supplierId": supplierID,
		"items": []map[string]any{
			{"itemType": "product", "itemId": productID, "quantity": 3, "amount": 15},
			{"itemType": "product", "itemId": productID, "quantity": 4, "amount": 20},
		},
	}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create supplier invoice: %d body=%s", rec.Code, rec.Body.String())
	}
	var invoice struct {
		ID string `json:"id"`
	}
	decodeEnvelope(t, rec.Body, &invoice)

	// One unit has since been consumed, leaving less than the invoice's total
	// received quantity across its duplicate product lines.
	if _, err := store.DB.Exec(`UPDATE products SET quantity = 6 WHERE id = ?`, productID); err != nil {
		t.Fatal(err)
	}
	rec = doRequest(t, e, http.MethodDelete, "/api/supplier-invoices/"+invoice.ID, nil, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "would make stock")

	var quantity int
	if err := store.RDB.QueryRow(`SELECT quantity FROM products WHERE id = ?`, productID).Scan(&quantity); err != nil {
		t.Fatal(err)
	}
	if quantity != 6 {
		t.Errorf("stock changed after rejected delete: got %d want 6", quantity)
	}
	if n := countTableRows(t, "invoices", "id = ? AND voided_at = ''", invoice.ID); n != 1 {
		t.Errorf("supplier invoice should remain active")
	}
}
