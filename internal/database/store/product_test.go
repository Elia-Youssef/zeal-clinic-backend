package store

import (
	"errors"
	"testing"

	"clinic-api/internal/validation"
)

func TestProduct_IsValid(t *testing.T) {
	cases := []struct {
		name string
		p    Product
		key  string
		ok   bool
	}{
		{"ok", Product{Name: "X", UnitPrice: 1}, "", true},
		{"zero unit price ok", Product{Name: "X", UnitPrice: 0}, "", true},
		{"missing name", Product{Name: "", UnitPrice: 1}, "name", false},
		{"negative price", Product{Name: "X", UnitPrice: -1}, "unitPrice", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.IsValid()
			if tc.ok {
				if err != nil {
					t.Errorf("expected ok, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			var ve validation.Errors
			if !errors.As(err, &ve) {
				t.Fatalf("err is %T", err)
			}
			if _, ok := ve[tc.key]; !ok {
				t.Errorf("expected key %q in %v", tc.key, ve)
			}
		})
	}
}

func TestProduct_Create_RoundTrip(t *testing.T) {
	setupTestDB(t)
	p := Product{Name: "Cream", Quantity: 5, MinThreshold: 2, UnitPrice: 12.5}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	if p.ID == "" {
		t.Errorf("ID not assigned")
	}

	var got Product
	if err := got.GetByID(p.ID); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Cream" || got.Quantity != 5 || got.MinThreshold != 2 || got.UnitPrice != 12.5 {
		t.Errorf("got %+v", got)
	}
}

func TestProduct_Update_PriceCreatesNewActiveRow(t *testing.T) {
	setupTestDB(t)
	p := Product{Name: "X", Quantity: 1, UnitPrice: 10}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	if err := p.Update(map[string]any{"unitPrice": 20.0}); err != nil {
		t.Fatal(err)
	}
	if p.UnitPrice != 20 {
		t.Errorf("UnitPrice = %v", p.UnitPrice)
	}
	if n := countRows(t, "product_prices", "product_id = ? AND is_active = 1", p.ID); n != 1 {
		t.Errorf("active rows = %d want 1", n)
	}
	if n := countRows(t, "product_prices", "product_id = ? AND is_active = 0", p.ID); n != 1 {
		t.Errorf("inactive rows = %d want 1", n)
	}
}

func TestProduct_Update_SamePriceNoNewRow(t *testing.T) {
	setupTestDB(t)
	p := Product{Name: "X", Quantity: 1, UnitPrice: 10}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	if err := p.Update(map[string]any{"unitPrice": 10.0}); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, "product_prices", "product_id = ?", p.ID); n != 1 {
		t.Errorf("expected 1 price row, got %d", n)
	}
}

func TestProduct_AdjustQuantity_InTx(t *testing.T) {
	setupTestDB(t)
	p := Product{Name: "Adj", Quantity: 10, UnitPrice: 1}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}

	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AdjustQuantity(-3, tx); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := productQuantity(t, p.ID); got != 7 {
		t.Errorf("qty = %d want 7", got)
	}
}

func TestProduct_AdjustQuantity_NegativeAllowed(t *testing.T) {
	// Schema does not enforce non-negative; capture the behavior.
	setupTestDB(t)
	p := Product{Name: "Neg", Quantity: 1, UnitPrice: 1}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	tx, _ := DB.Begin()
	if err := p.AdjustQuantity(-100, tx); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := productQuantity(t, p.ID); got != -99 {
		t.Errorf("qty = %d want -99 (no negative-stock guard exists in schema)", got)
	}
}

func TestProduct_Delete(t *testing.T) {
	setupTestDB(t)
	p := Product{Name: "ToDel", Quantity: 1, UnitPrice: 1}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	if err := p.Delete(); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, "products", "id = ?", p.ID); n != 0 {
		t.Errorf("not deleted")
	}
}

func TestProduct_Delete_NotFound(t *testing.T) {
	setupTestDB(t)
	p := Product{ID: "ghost"}
	err := p.Delete()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestProductList_GetAll_FilterPaginate(t *testing.T) {
	setupTestDB(t)
	for _, n := range []string{"alpha", "beta", "gamma"} {
		p := Product{Name: n, UnitPrice: 1}
		if err := p.Create(); err != nil {
			t.Fatal(err)
		}
	}

	var list ProductList
	total, _ := list.GetAll(ListParams{})
	if total < 3 {
		t.Errorf("total = %d want >=3", total)
	}

	list = nil
	total, _ = list.GetAll(ListParams{Filter: "alpha"})
	if total != 1 {
		t.Errorf("filter total = %d want 1", total)
	}
}
