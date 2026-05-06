package store

import (
	"errors"
	"testing"

	"clinic-api/internal/validation"
)

func validPatient() Patient {
	return Patient{
		FirstName:   "Anne",
		LastName:    "Smith",
		Gender:      "Female",
		DateOfBirth: "1990-01-01",
		Contact:     "0123456789",
	}
}

func TestPatient_IsValid_Ok(t *testing.T) {
	p := validPatient()
	if err := p.IsValid(); err != nil {
		t.Errorf("got %v", err)
	}
}

func TestPatient_IsValid_Errors(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Patient)
		key  string
	}{
		{"missing firstName", func(p *Patient) { p.FirstName = "" }, "firstName"},
		{"missing lastName", func(p *Patient) { p.LastName = "" }, "lastName"},
		{"missing gender", func(p *Patient) { p.Gender = "" }, "gender"},
		{"unknown gender", func(p *Patient) { p.Gender = "Other" }, "gender"},
		{"missing DOB", func(p *Patient) { p.DateOfBirth = "" }, "dateOfBirth"},
		{"bad DOB format", func(p *Patient) { p.DateOfBirth = "01/01/1990" }, "dateOfBirth"},
		{"impossible DOB", func(p *Patient) { p.DateOfBirth = "2024-13-01" }, "dateOfBirth"},
		{"missing phone", func(p *Patient) { p.Contact = "" }, "contact"},
		{"bad phone", func(p *Patient) { p.Contact = "abc" }, "contact"},
		{"bad email", func(p *Patient) { p.Email = "not-an-email" }, "email"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validPatient()
			tc.mut(&p)
			err := p.IsValid()
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

func TestPatient_Create_AssignsIDsAndCreatesBalance(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	preBalances := countRows(t, "balances", "entity_type='patient'")

	p := validPatient()
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	if p.ID == "" {
		t.Errorf("ID not assigned")
	}
	// Patient balance row should exist for every existing currency.
	currencies := countRows(t, "currencies", "")
	postBalances := countRows(t, "balances", "entity_type='patient'")
	if postBalances-preBalances != currencies {
		t.Errorf("expected %d new patient balance(s), got %d", currencies, postBalances-preBalances)
	}

	// Specifically for the seeded currency:
	bal := patientBalance(t, p.ID, cur.ID)
	if bal.EntityName != "Anne Smith" {
		t.Errorf("balance entity_name = %q want %q", bal.EntityName, "Anne Smith")
	}
}

func TestPatient_Create_PreservesExplicitCreatedAt(t *testing.T) {
	setupTestDB(t)
	p := validPatient()
	p.CreatedAt = "2020-06-15T12:00:00Z"
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	if p.CreatedAt != "2020-06-15T12:00:00Z" {
		t.Errorf("CreatedAt overwritten, got %q", p.CreatedAt)
	}
}

func TestPatient_GetByID_NotFound(t *testing.T) {
	setupTestDB(t)
	var p Patient
	err := p.GetByID("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestPatient_GetByPhone(t *testing.T) {
	setupTestDB(t)
	p := validPatient()
	p.Contact = "0987654321"
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}

	var got Patient
	if err := got.GetByPhone("0987654321"); err != nil {
		t.Fatal(err)
	}
	if got.ID != p.ID {
		t.Errorf("got %v", got.ID)
	}

	if err := got.GetByPhone("nonexistent"); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestPatient_Update(t *testing.T) {
	setupTestDB(t)
	p := validPatient()
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}

	if err := p.Update(map[string]any{
		"firstName": "Annie",
		"email":     "annie@example.com",
		"weight":    65.5,
	}); err != nil {
		t.Fatal(err)
	}
	if p.FirstName != "Annie" || p.Email != "annie@example.com" || p.Weight != 65.5 {
		t.Errorf("update did not apply: %+v", p)
	}
}

func TestPatient_Update_EmptyMapReloads(t *testing.T) {
	setupTestDB(t)
	p := validPatient()
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	p.FirstName = "tampered"
	if err := p.Update(map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if p.FirstName != "Anne" {
		t.Errorf("expected reload to reset FirstName, got %q", p.FirstName)
	}
}

func TestPatient_Delete(t *testing.T) {
	setupTestDB(t)
	p := validPatient()
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}
	if err := p.Delete(); err != nil {
		t.Fatal(err)
	}
	var got Patient
	if err := got.GetByID(p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestPatient_Delete_NotFound(t *testing.T) {
	setupTestDB(t)
	p := Patient{ID: "ghost"}
	err := p.Delete()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestPatientList_GetAll_FilterAndPaginate(t *testing.T) {
	setupTestDB(t)
	for _, fn := range []string{"Alice", "Bob", "Charlie", "David", "Eve"} {
		p := validPatient()
		p.FirstName = fn
		p.Contact = fn + "phone"
		if err := p.Create(); err != nil {
			t.Fatal(err)
		}
	}

	var list PatientList
	total, err := list.GetAll(ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("total = %d", total)
	}

	// Filter by first name.
	list = nil
	total, _ = list.GetAll(ListParams{Filter: "Alic"})
	if total != 1 || list[0].FirstName != "Alice" {
		t.Errorf("got total=%d list=%+v", total, list)
	}

	// Pagination.
	list = nil
	_, _ = list.GetAll(ListParams{Limit: 2})
	if len(list) != 2 {
		t.Errorf("limit=2 got %d", len(list))
	}
}

func TestGetPatientDropdown(t *testing.T) {
	setupTestDB(t)
	p := validPatient()
	p.FirstName = "Bob"
	p.LastName = "Marley"
	p.Contact = "marley-1"
	if err := p.Create(); err != nil {
		t.Fatal(err)
	}

	items, err := GetPatientDropdown(ListParams{Filter: "Marley"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "Bob Marley" {
		t.Errorf("got %+v", items)
	}
}
