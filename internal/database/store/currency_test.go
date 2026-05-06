package store

import (
	"errors"
	"testing"

	"clinic-api/internal/validation"
)

func TestCurrency_IsValid_Ok(t *testing.T) {
	c := Currency{Code: "USD", Name: "US Dollar"}
	if err := c.IsValid(); err != nil {
		t.Errorf("got %v", err)
	}
}

func TestCurrency_IsValid_RequiresCodeAndName(t *testing.T) {
	cases := []struct {
		name string
		c    Currency
		key  string
	}{
		{"missing code", Currency{Name: "X"}, "code"},
		{"missing name", Currency{Code: "X"}, "name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.c.IsValid()
			if err == nil {
				t.Fatalf("expected error")
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

func TestCurrency_CRUD(t *testing.T) {
	setupTestDB(t)
	c := Currency{Code: "ZED", Name: "Zedland Dollar", Symbol: "Z$", ExchangeRate: 1.5}
	if err := c.Create(); err != nil {
		t.Fatal(err)
	}
	if c.ID == "" {
		t.Errorf("ID not assigned")
	}
	if c.CreatedAt == "" || c.UpdatedAt == "" {
		t.Errorf("timestamps not set")
	}

	// GetByID
	var byID Currency
	if err := byID.GetByID(c.ID); err != nil {
		t.Fatal(err)
	}
	if byID.Code != "ZED" || byID.ExchangeRate != 1.5 {
		t.Errorf("got %+v", byID)
	}

	// GetByCode
	var byCode Currency
	if err := byCode.GetByCode("ZED"); err != nil {
		t.Fatal(err)
	}
	if byCode.ID != c.ID {
		t.Errorf("got %v want %v", byCode.ID, c.ID)
	}

	// Update
	c2 := Currency{ID: c.ID}
	if err := c2.Update(map[string]any{"name": "ZedlandX", "exchangeRate": 2.0}); err != nil {
		t.Fatal(err)
	}
	if c2.Name != "ZedlandX" || c2.ExchangeRate != 2.0 {
		t.Errorf("got %+v", c2)
	}
	if c2.Code != "ZED" {
		t.Errorf("Code should be unchanged, got %q", c2.Code)
	}

	// Delete
	if err := c2.Delete(); err != nil {
		t.Fatal(err)
	}
	var afterDel Currency
	if err := afterDel.GetByID(c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v after delete", err)
	}
}

func TestCurrency_Update_EmptyMapReloads(t *testing.T) {
	setupTestDB(t)
	c := Currency{Code: "RR", Name: "Rubble", Symbol: "R", ExchangeRate: 1}
	if err := c.Create(); err != nil {
		t.Fatal(err)
	}
	c.Name = "tampered"
	if err := c.Update(map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if c.Name != "Rubble" {
		t.Errorf("Update empty should reload from DB, got %q", c.Name)
	}
}

func TestCurrency_GetByID_NotFound(t *testing.T) {
	setupTestDB(t)
	var c Currency
	err := c.GetByID("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestCurrency_GetByCode_NotFound(t *testing.T) {
	setupTestDB(t)
	var c Currency
	err := c.GetByCode("nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestCurrency_Delete_NotFound(t *testing.T) {
	setupTestDB(t)
	c := Currency{ID: "ghost"}
	err := c.Delete()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestCurrencyList_GetAll_AndFilter(t *testing.T) {
	setupTestDB(t)
	preTotal := countRows(t, "currencies", "")

	for _, code := range []string{"AAA", "BBB", "CCC"} {
		c := Currency{Code: code, Name: code + " Name"}
		if err := c.Create(); err != nil {
			t.Fatal(err)
		}
	}

	var list CurrencyList
	total, err := list.GetAll(ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if total != preTotal+3 {
		t.Errorf("total = %d want %d", total, preTotal+3)
	}

	// Filter by code substring.
	list = nil
	total, err = list.GetAll(ListParams{Filter: "AAA"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 || list[0].Code != "AAA" {
		t.Errorf("got total=%d list=%+v", total, list)
	}

	// Filter by name (multi-column OR).
	list = nil
	total, _ = list.GetAll(ListParams{Filter: "BBB Name"})
	if total != 1 {
		t.Errorf("expected match by name, got total=%d", total)
	}

	// Pagination.
	list = nil
	_, _ = list.GetAll(ListParams{Limit: 2})
	if len(list) != 2 {
		t.Errorf("limit=2 returned %d", len(list))
	}
}

func TestGetCurrencyDropdown(t *testing.T) {
	setupTestDB(t)
	c := Currency{Code: "DRP", Name: "Dropdown Test"}
	if err := c.Create(); err != nil {
		t.Fatal(err)
	}
	items, err := GetCurrencyDropdown(ListParams{Filter: "Dropdown"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "Dropdown Test" || items[0].ID != c.ID {
		t.Errorf("got %+v", items)
	}
}
