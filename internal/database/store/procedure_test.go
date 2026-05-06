package store

import (
	"errors"
	"testing"

	"clinic-api/internal/validation"
)

func TestProcedure_IsValid(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		p := Procedure{Name: "Botox"}
		if err := p.IsValid(); err != nil {
			t.Errorf("got %v", err)
		}
	})
	t.Run("missing name", func(t *testing.T) {
		p := Procedure{Name: ""}
		err := p.IsValid()
		if err == nil {
			t.Fatal("expected error")
		}
		var ve validation.Errors
		if !errors.As(err, &ve) {
			t.Fatalf("err is %T", err)
		}
		if _, ok := ve["name"]; !ok {
			t.Errorf("expected name key in %v", ve)
		}
	})
}

func TestProcedure_Create_StoresPriceInActivePriceTable(t *testing.T) {
	setupTestDB(t)
	p := Procedure{Name: "Treatment", Price: 250, IsActive: true}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	if p.ID == "" {
		t.Errorf("ID not assigned")
	}

	// procedure_prices should hold an active row with that price.
	var price float64
	var isActive int
	if err := RDB.QueryRow(`SELECT price, is_active FROM procedure_prices WHERE procedure_id = ? AND is_active = 1`, p.ID).Scan(&price, &isActive); err != nil {
		t.Fatal(err)
	}
	if price != 250 || isActive != 1 {
		t.Errorf("price row = price %v active %d", price, isActive)
	}

	// GetByID exposes the price via the join.
	var got Procedure
	if err := got.GetByID(p.ID); err != nil {
		t.Fatal(err)
	}
	if got.Price != 250 || !got.IsActive {
		t.Errorf("got %+v", got)
	}
}

func TestProcedure_GetByID_NotFound(t *testing.T) {
	setupTestDB(t)
	var p Procedure
	err := p.GetByID("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestProcedure_Update_PriceCreatesNewActiveRow(t *testing.T) {
	setupTestDB(t)
	p := Procedure{Name: "Facial", Price: 100, IsActive: true}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}

	// Update with a new price.
	if err := p.Update(map[string]any{"price": 150.0}); err != nil {
		t.Fatal(err)
	}
	if p.Price != 150 {
		t.Errorf("got Price=%v", p.Price)
	}
	// One active row, one inactive.
	if n := countRows(t, "procedure_prices", "procedure_id = ? AND is_active = 1", p.ID); n != 1 {
		t.Errorf("active rows = %d want 1", n)
	}
	if n := countRows(t, "procedure_prices", "procedure_id = ? AND is_active = 0", p.ID); n != 1 {
		t.Errorf("inactive rows = %d want 1", n)
	}
}

func TestProcedure_Update_SamePriceDoesNotCreateNewRow(t *testing.T) {
	setupTestDB(t)
	p := Procedure{Name: "Massage", Price: 80, IsActive: true}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}

	if err := p.Update(map[string]any{"price": 80.0}); err != nil {
		t.Fatal(err)
	}
	// Only one row total.
	if n := countRows(t, "procedure_prices", "procedure_id = ?", p.ID); n != 1 {
		t.Errorf("expected 1 row when price unchanged, got %d", n)
	}
}

func TestProcedure_Update_NonPriceFields(t *testing.T) {
	setupTestDB(t)
	p := Procedure{Name: "Old Name", Price: 100, IsActive: true}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	if err := p.Update(map[string]any{
		"name":     "New Name",
		"isActive": false,
		"remarks":  "be careful",
	}); err != nil {
		t.Fatal(err)
	}
	if p.Name != "New Name" || p.IsActive || p.Remarks != "be careful" {
		t.Errorf("got %+v", p)
	}
}

func TestProcedure_Update_PriceTypes(t *testing.T) {
	setupTestDB(t)
	p := Procedure{Name: "TypePrice", Price: 100, IsActive: true}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		val  any
		want float64
	}{
		{"int", 200, 200},
		{"int64", int64(300), 300},
		{"float32", float32(400), 400},
		{"float64", 500.5, 500.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := p.Update(map[string]any{"price": tc.val}); err != nil {
				t.Fatal(err)
			}
			if !approxEqual(p.Price, tc.want) {
				t.Errorf("got %v want %v", p.Price, tc.want)
			}
		})
	}
}

func TestProcedure_Delete(t *testing.T) {
	setupTestDB(t)
	p := Procedure{Name: "Remove", Price: 1, IsActive: true}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	if err := p.Delete(); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, "procedures", "id = ?", p.ID); n != 0 {
		t.Errorf("procedure not deleted")
	}
}

func TestProcedure_Delete_NotFound(t *testing.T) {
	setupTestDB(t)
	p := Procedure{ID: "ghost"}
	err := p.Delete()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestProcedureList_GetAll_FilterAndPaginate(t *testing.T) {
	setupTestDB(t)
	for _, n := range []string{"Botox", "Filler", "Peel", "Massage", "Laser"} {
		p := Procedure{Name: n, Price: 1, IsActive: true}
		if err := p.Create(); err != nil {
			t.Fatal(err)
		}
	}

	var list ProcedureList
	total, _ := list.GetAll(ListParams{})
	// Migrations may have seeded procedures too, so check at least 5 total.
	if total < 5 {
		t.Errorf("total = %d want >=5", total)
	}

	// Filter by name.
	list = nil
	total, _ = list.GetAll(ListParams{Filter: "Botox"})
	if total < 1 {
		t.Errorf("filter Botox total = %d", total)
	}

	// Pagination.
	list = nil
	_, _ = list.GetAll(ListParams{Limit: 2})
	if len(list) != 2 {
		t.Errorf("limit=2 got %d", len(list))
	}
}

func TestGetProcedureDropdown(t *testing.T) {
	setupTestDB(t)
	p := Procedure{Name: "DropdownTest", Price: 1, IsActive: true}
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	items, err := GetProcedureDropdown(ListParams{Filter: "DropdownTest"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "DropdownTest" {
		t.Errorf("got %+v", items)
	}
}
