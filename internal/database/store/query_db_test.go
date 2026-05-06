package store

import (
	"testing"
)

// HasDependencies and DeleteDependencies use the global DB/RDB.

func TestHasDependencies_TrueWhenRowsExist(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	pat := makePatient(t, "Dep", "P", "9991111")
	pb := patientBalance(t, pat.ID, cur.ID)

	// Create a balance transaction whose from_balance_id is the patient balance.
	self := seededSelfBalance(t, cur.ID)
	bt := BalanceTransaction{FromBalanceID: pb.ID, ToBalanceID: self.ID, Amount: 1, CurrencyID: cur.ID, TransactionType: "payment"}
	if err := bt.Create(); err != nil {
		t.Fatal(err)
	}

	deps := map[string]string{
		"balance_transactions": "from_balance_id",
		"invoices":             "from_balance_id",
	}
	if !HasDependencies(pb.ID, deps) {
		t.Errorf("expected HasDependencies=true (balance_transactions row exists)")
	}
}

func TestHasDependencies_FalseWhenNoMatches(t *testing.T) {
	setupTestDB(t)
	deps := map[string]string{
		"balance_transactions": "from_balance_id",
	}
	if HasDependencies("does-not-exist", deps) {
		t.Errorf("expected HasDependencies=false")
	}
}

func TestHasDependencies_StopsAtFirstMatch(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	pat := makePatient(t, "First", "Match", "9991112")
	pb := patientBalance(t, pat.ID, cur.ID)
	self := seededSelfBalance(t, cur.ID)
	bt := BalanceTransaction{FromBalanceID: pb.ID, ToBalanceID: self.ID, Amount: 1, CurrencyID: cur.ID, TransactionType: "payment"}
	if err := bt.Create(); err != nil {
		t.Fatal(err)
	}

	// Add a non-existent table; the function should skip it (continue) and not
	// crash the call.
	deps := map[string]string{
		"definitely_not_a_real_table": "id",
		"balance_transactions":        "from_balance_id",
	}
	if !HasDependencies(pb.ID, deps) {
		t.Errorf("expected true despite the bogus table key")
	}
}

func TestHasDependencies_AllBadTablesReturnsFalse(t *testing.T) {
	// Bad table names make QueryRow error, and the function continues silently and
	// returns false. Document the behavior.
	setupTestDB(t)
	deps := map[string]string{
		"not_a_table":           "x",
		"another_phantom_table": "y",
	}
	if HasDependencies("anything", deps) {
		t.Errorf("expected false for all-bad tables")
	}
}

func TestDeleteDependencies_RemovesAllListedRows(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	pat := makePatient(t, "Cascade", "Patient", "9991113")
	pb := patientBalance(t, pat.ID, cur.ID)
	self := seededSelfBalance(t, cur.ID)

	for i := 0; i < 3; i++ {
		bt := BalanceTransaction{FromBalanceID: pb.ID, ToBalanceID: self.ID, Amount: float64(i + 1), CurrencyID: cur.ID, TransactionType: "payment"}
		if err := bt.Create(); err != nil {
			t.Fatal(err)
		}
	}

	if n := countRows(t, "balance_transactions", "from_balance_id = ?", pb.ID); n != 3 {
		t.Fatalf("seed failed: got %d", n)
	}

	if err := DeleteDependencies(pb.ID, map[string]string{
		"balance_transactions": "from_balance_id",
	}); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, "balance_transactions", "from_balance_id = ?", pb.ID); n != 0 {
		t.Errorf("expected 0 rows after DeleteDependencies, got %d", n)
	}
}

func TestDeleteDependencies_AtomicOnFailure(t *testing.T) {
	// Run a delete with one bad table; the whole transaction should roll
	// back and the good-table delete should NOT take effect.
	setupTestDB(t)
	cur := seededCurrency(t)
	pat := makePatient(t, "Atomic", "P", "9991114")
	pb := patientBalance(t, pat.ID, cur.ID)
	self := seededSelfBalance(t, cur.ID)

	bt := BalanceTransaction{FromBalanceID: pb.ID, ToBalanceID: self.ID, Amount: 5, CurrencyID: cur.ID, TransactionType: "payment"}
	if err := bt.Create(); err != nil {
		t.Fatal(err)
	}

	err := DeleteDependencies(pb.ID, map[string]string{
		"balance_transactions": "from_balance_id",
		"not_a_real_table":     "id",
	})
	if err == nil {
		t.Errorf("expected error from bad table delete")
	}
	// The good table delete must have been rolled back.
	if n := countRows(t, "balance_transactions", "from_balance_id = ?", pb.ID); n != 1 {
		t.Errorf("good-table delete should be rolled back; got %d rows want 1", n)
	}
}

func TestDeleteDependencies_NoMatches(t *testing.T) {
	setupTestDB(t)
	// Should not error even if nothing matches.
	if err := DeleteDependencies("nonexistent", map[string]string{
		"balance_transactions": "from_balance_id",
	}); err != nil {
		t.Errorf("got %v", err)
	}
}

func TestDeleteDependencies_EmptyDeps(t *testing.T) {
	setupTestDB(t)
	if err := DeleteDependencies("any", map[string]string{}); err != nil {
		t.Errorf("got %v", err)
	}
}
