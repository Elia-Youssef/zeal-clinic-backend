package store

import "testing"

func TestSetupTestDB_Smoke(t *testing.T) {
	setupTestDB(t)

	// Migrations should have seeded at least one currency, role, and a self balance.
	if n := countRows(t, "currencies", ""); n == 0 {
		t.Errorf("expected seeded currencies, found 0")
	}
	if n := countRows(t, "roles", ""); n == 0 {
		t.Errorf("expected seeded roles, found 0")
	}
	if n := countRows(t, "balances", "entity_type = 'self'"); n == 0 {
		t.Errorf("expected at least one self balance, found 0")
	}
	if n := countRows(t, "users", ""); n == 0 {
		t.Errorf("expected the default admin user")
	}
}

func TestSetupTestDB_IsolatedBetweenTests(t *testing.T) {
	setupTestDB(t)
	c := seedCurrency(t, "ZZZ", "Zedland", "Z", 1.0)
	if c.ID == "" {
		t.Fatal("expected ID set")
	}
	// Look it up.
	var got Currency
	if err := got.GetByCode("ZZZ"); err != nil {
		t.Fatalf("GetByCode: %v", err)
	}
	if got.ID != c.ID {
		t.Errorf("got %v want %v", got.ID, c.ID)
	}
}

func TestSetupTestDB_SecondCallStartsClean(t *testing.T) {
	// Verify the previous test's ZZZ currency is gone in this fresh DB.
	setupTestDB(t)
	var got Currency
	if err := got.GetByCode("ZZZ"); err == nil {
		t.Errorf("expected fresh DB; ZZZ should not exist, got %+v", got)
	}
}
