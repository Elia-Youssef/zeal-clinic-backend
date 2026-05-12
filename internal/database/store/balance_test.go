package store

import (
	"errors"
	"testing"

	"clinic-api/internal/validation"
)

func TestBalance_IsValid_AllowedTypes(t *testing.T) {
	for _, et := range []string{"patient", "employee", "self", "supplier", "expense"} {
		b := Balance{EntityType: et}
		if err := b.IsValid(); err != nil {
			t.Errorf("type %q: unexpected err %v", et, err)
		}
	}
}

func TestBalance_IsValid_Errors(t *testing.T) {
	cases := []struct {
		name string
		b    Balance
		key  string
	}{
		{"missing entity type", Balance{EntityType: ""}, "entityType"},
		{"unknown entity type", Balance{EntityType: "alien"}, "entityType"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.b.IsValid()
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

func TestBalance_GetOrCreate_FreshAndIdempotent(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	id := "exp-1"
	b := Balance{
		EntityType: "expense",
		EntityID:   &id,
		EntityName: "Rent",
		CurrencyID: cur.ID,
	}
	if err := b.GetOrCreate(); err != nil {
		t.Fatal(err)
	}
	if b.ID == "" {
		t.Errorf("ID not assigned")
	}
	first := b.ID

	// Second call must return the same row, not create a new one.
	b2 := Balance{
		EntityType: "expense",
		EntityID:   &id,
		EntityName: "ignored-on-existing",
		CurrencyID: cur.ID,
	}
	if err := b2.GetOrCreate(); err != nil {
		t.Fatal(err)
	}
	if b2.ID != first {
		t.Errorf("expected same ID %q, got %q", first, b2.ID)
	}

	// Initial amounts must be 0.
	got := fetchBalance(t, first)
	if got.Amount != 0 || got.TotalIn != 0 || got.TotalOut != 0 {
		t.Errorf("fresh balance not zeroed: %+v", got)
	}
}

func TestBalance_GetByID_AndNotFound(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	id := "exp"
	b := Balance{EntityType: "expense", EntityID: &id, EntityName: "X", CurrencyID: cur.ID}
	if err := b.GetOrCreate(); err != nil {
		t.Fatal(err)
	}

	var got Balance
	if err := got.GetByID(b.ID); err != nil {
		t.Fatal(err)
	}
	if got.EntityName != "X" {
		t.Errorf("got %+v", got)
	}

	var miss Balance
	if err := miss.GetByID("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestBalance_GetByEntityID(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	id := "patient-x"
	b := Balance{EntityType: "patient", EntityID: &id, EntityName: "P", CurrencyID: cur.ID}
	if err := b.GetOrCreate(); err != nil {
		t.Fatal(err)
	}
	var got Balance
	if err := got.GetByEntityID("patient", id); err != nil {
		t.Fatal(err)
	}
	if got.ID != b.ID {
		t.Errorf("got %v", got.ID)
	}
}

func TestBalanceList_GetAll_FiltersByEntityType(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)

	// Self balance is seeded by migrations. Add one expense balance.
	expID := "exp-1"
	exp := Balance{EntityType: "expense", EntityID: &expID, EntityName: "Rent", CurrencyID: cur.ID}
	if err := exp.GetOrCreate(); err != nil {
		t.Fatal(err)
	}

	var list BalanceList
	total, err := list.GetAll("expense", ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 || list[0].EntityType != "expense" {
		t.Errorf("got total=%d list=%+v", total, list)
	}

	// Filter by entity name.
	list = nil
	total, err = list.GetAll("", ListParams{Filter: "Rent"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("filter Rent: total=%d", total)
	}
}

// BalanceTransaction tests

func TestBalanceTransaction_IsValid(t *testing.T) {
	cases := []struct {
		name string
		bt   BalanceTransaction
		key  string
		ok   bool
	}{
		{"ok", BalanceTransaction{FromBalanceID: "a", ToBalanceID: "b", Amount: 10}, "", true},
		{"missing from", BalanceTransaction{ToBalanceID: "b", Amount: 1}, "fromBalanceId", false},
		{"missing to", BalanceTransaction{FromBalanceID: "a", Amount: 1}, "toBalanceId", false},
		{"negative amount", BalanceTransaction{FromBalanceID: "a", ToBalanceID: "b", Amount: -1}, "amount", false},
		{"zero amount allowed", BalanceTransaction{FromBalanceID: "a", ToBalanceID: "b", Amount: 0}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.bt.IsValid()
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

func TestBalanceTransaction_Create_PaymentMovesAmountAndUpdatesFlowTotals(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	// patient pays self 100; patient balance was 0 (no debt yet), this just
	// records a flow. We're testing the math, not the business case.
	pid := "patient-x"
	p := Balance{EntityType: "patient", EntityID: &pid, EntityName: "P", CurrencyID: cur.ID}
	if err := p.GetOrCreate(); err != nil {
		t.Fatal(err)
	}

	bt := BalanceTransaction{
		FromBalanceID:   p.ID,
		ToBalanceID:     self.ID,
		Amount:          100,
		CurrencyID:      cur.ID,
		TransactionType: "payment",
	}
	if err := bt.Create(); err != nil {
		t.Fatal(err)
	}
	if bt.ID == "" {
		t.Errorf("ID not assigned")
	}

	from := fetchBalance(t, p.ID)
	to := fetchBalance(t, self.ID)
	if !approxEqual(from.Amount, -100) {
		t.Errorf("from.Amount = %v want -100", from.Amount)
	}
	// TransactionType "payment" affects flow totals.
	if !approxEqual(from.TotalOut, 100) {
		t.Errorf("from.TotalOut = %v want 100", from.TotalOut)
	}
	if !approxEqual(from.TotalIn, 0) {
		t.Errorf("from.TotalIn = %v want 0", from.TotalIn)
	}

	if !approxEqual(to.Amount, self.Amount+100) {
		t.Errorf("to.Amount = %v want %v", to.Amount, self.Amount+100)
	}
	if !approxEqual(to.TotalIn, 100) {
		t.Errorf("to.TotalIn = %v want 100", to.TotalIn)
	}
}

func TestBalanceTransaction_Create_DefaultsTypeAndMethod(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pid := "patient-d"
	p := Balance{EntityType: "patient", EntityID: &pid, EntityName: "P", CurrencyID: cur.ID}
	if err := p.GetOrCreate(); err != nil {
		t.Fatal(err)
	}

	// Empty TransactionType / TransactionMethod should default to "payment" / "cash".
	bt := BalanceTransaction{FromBalanceID: p.ID, ToBalanceID: self.ID, Amount: 50, CurrencyID: cur.ID}
	if err := bt.Create(); err != nil {
		t.Fatal(err)
	}
	if bt.TransactionType != "payment" {
		t.Errorf("TransactionType default = %q", bt.TransactionType)
	}
	if bt.TransactionMethod != "cash" {
		t.Errorf("TransactionMethod default = %q", bt.TransactionMethod)
	}
}

func TestBalanceTransaction_Create_ChargeDoesNotAffectFlowTotals(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pid := "patient-c"
	p := Balance{EntityType: "patient", EntityID: &pid, EntityName: "P", CurrencyID: cur.ID}
	if err := p.GetOrCreate(); err != nil {
		t.Fatal(err)
	}

	bt := BalanceTransaction{
		FromBalanceID:   p.ID,
		ToBalanceID:     self.ID,
		Amount:          75,
		CurrencyID:      cur.ID,
		TransactionType: "charge",
	}
	if err := bt.Create(); err != nil {
		t.Fatal(err)
	}

	from := fetchBalance(t, p.ID)
	to := fetchBalance(t, self.ID)

	if !approxEqual(from.Amount, -75) {
		t.Errorf("from.Amount = %v", from.Amount)
	}
	if !approxEqual(to.Amount, self.Amount+75) {
		t.Errorf("to.Amount = %v", to.Amount)
	}
	// "charge" must NOT count toward total_in/total_out.
	if !approxEqual(from.TotalOut, 0) {
		t.Errorf("charge should leave from.TotalOut at 0, got %v", from.TotalOut)
	}
	if !approxEqual(to.TotalIn, 0) {
		t.Errorf("charge should leave to.TotalIn at 0, got %v", to.TotalIn)
	}
}

func TestBalanceTransaction_Create_AdjustmentDoesNotAffectFlowTotals(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pid := "patient-a"
	p := Balance{EntityType: "patient", EntityID: &pid, EntityName: "P", CurrencyID: cur.ID}
	if err := p.GetOrCreate(); err != nil {
		t.Fatal(err)
	}

	bt := BalanceTransaction{
		FromBalanceID:   p.ID,
		ToBalanceID:     self.ID,
		Amount:          10,
		CurrencyID:      cur.ID,
		TransactionType: "adjustment",
	}
	if err := bt.Create(); err != nil {
		t.Fatal(err)
	}
	from := fetchBalance(t, p.ID)
	to := fetchBalance(t, self.ID)
	if !approxEqual(from.TotalOut, 0) || !approxEqual(to.TotalIn, 0) {
		t.Errorf("adjustment must not move flow totals; from.TotalOut=%v to.TotalIn=%v", from.TotalOut, to.TotalIn)
	}
	// But the amount should still move.
	if !approxEqual(from.Amount, -10) || !approxEqual(to.Amount, self.Amount+10) {
		t.Errorf("amounts should still move; from.Amount=%v to.Amount=%v", from.Amount, to.Amount)
	}
}

func TestBalanceTransaction_CreateTwoWay_NetsToZero(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	exp := expenseBalance(t, "Rent", cur.ID)

	// CreateTwoWay treats `bt` as the payment leg (self to expense),
	// then derives the charge leg in reverse (expense to self).
	bt := BalanceTransaction{
		FromBalanceID:   self.ID,
		ToBalanceID:     exp.ID,
		Amount:          200,
		CurrencyID:      cur.ID,
		TransactionType: "payment",
	}
	if err := bt.CreateTwoWay(); err != nil {
		t.Fatal(err)
	}

	// Both balances should net to 0 (charge cancels payment on amount).
	selfAfter := fetchBalance(t, self.ID)
	expAfter := fetchBalance(t, exp.ID)
	if !approxEqual(selfAfter.Amount, self.Amount) {
		t.Errorf("self.Amount = %v want %v", selfAfter.Amount, self.Amount)
	}
	if !approxEqual(expAfter.Amount, 0) {
		t.Errorf("expense.Amount = %v want 0", expAfter.Amount)
	}

	// total_out / total_in should reflect the payment leg only (charge excluded).
	if !approxEqual(selfAfter.TotalOut, 200) {
		t.Errorf("self.TotalOut = %v want 200", selfAfter.TotalOut)
	}
	if !approxEqual(expAfter.TotalIn, 200) {
		t.Errorf("expense.TotalIn = %v want 200", expAfter.TotalIn)
	}

	// There should be exactly two transactions, one of each type.
	if n := countRows(t, "balance_transactions", ""); n != 2 {
		t.Errorf("expected 2 tx rows, got %d", n)
	}
	if n := countRows(t, "balance_transactions", "transaction_type='charge'"); n != 1 {
		t.Errorf("expected 1 charge, got %d", n)
	}
	if n := countRows(t, "balance_transactions", "transaction_type='payment'"); n != 1 {
		t.Errorf("expected 1 payment, got %d", n)
	}
}

func TestBalanceTransaction_Delete_ReversesPaymentOnly(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pid := "patient-r"
	p := Balance{EntityType: "patient", EntityID: &pid, EntityName: "P", CurrencyID: cur.ID}
	if err := p.GetOrCreate(); err != nil {
		t.Fatal(err)
	}

	bt := BalanceTransaction{
		FromBalanceID: p.ID, ToBalanceID: self.ID, Amount: 60, CurrencyID: cur.ID,
		TransactionType: "payment",
	}
	if err := bt.Create(); err != nil {
		t.Fatal(err)
	}

	if err := bt.Delete(); err != nil {
		t.Fatal(err)
	}

	from := fetchBalance(t, p.ID)
	to := fetchBalance(t, self.ID)
	if !approxEqual(from.Amount, 0) {
		t.Errorf("from reversed Amount = %v want 0", from.Amount)
	}
	if !approxEqual(to.Amount, self.Amount) {
		t.Errorf("to reversed Amount = %v want %v", to.Amount, self.Amount)
	}
	if !approxEqual(from.TotalOut, 0) || !approxEqual(to.TotalIn, 0) {
		t.Errorf("flow totals not reversed; from.TotalOut=%v to.TotalIn=%v", from.TotalOut, to.TotalIn)
	}
	if n := countRows(t, "balance_transactions", "voided_at = ''"); n != 0 {
		t.Errorf("expected 0 active tx rows after delete, got %d", n)
	}
	if n := countRows(t, "balance_transactions", "voided_at != ''"); n != 1 {
		t.Errorf("expected 1 voided tx row after delete, got %d", n)
	}
}

func TestBalanceTransaction_Delete_TwoWayRemovesPair(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	exp := expenseBalance(t, "Rent", cur.ID)

	bt := BalanceTransaction{
		FromBalanceID: self.ID, ToBalanceID: exp.ID, Amount: 300, CurrencyID: cur.ID,
		TransactionType: "payment",
	}
	if err := bt.CreateTwoWay(); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, "balance_transactions", ""); n != 2 {
		t.Fatalf("expected 2 tx rows, got %d", n)
	}

	// Deleting just the payment leg should remove its paired charge atomically.
	if err := bt.Delete(); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, "balance_transactions", "voided_at = ''"); n != 0 {
		t.Errorf("expected 0 active tx rows after pair delete, got %d", n)
	}
	if n := countRows(t, "balance_transactions", "voided_at != ''"); n != 2 {
		t.Errorf("expected 2 voided tx rows after pair delete, got %d", n)
	}
	selfAfter := fetchBalance(t, self.ID)
	expAfter := fetchBalance(t, exp.ID)
	if !approxEqual(selfAfter.Amount, self.Amount) || !approxEqual(expAfter.Amount, 0) {
		t.Errorf("balances should return to pre-state; self=%v exp=%v", selfAfter.Amount, expAfter.Amount)
	}
	if !approxEqual(selfAfter.TotalOut, 0) || !approxEqual(expAfter.TotalIn, 0) {
		t.Errorf("flow totals should be reversed; self.TotalOut=%v exp.TotalIn=%v", selfAfter.TotalOut, expAfter.TotalIn)
	}
}

func TestBalanceTransaction_Delete_NotFound(t *testing.T) {
	setupTestDB(t)
	bt := BalanceTransaction{ID: "ghost"}
	err := bt.Delete()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestBalanceTransactionList_GetByBalanceID(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pid := "patient-l"
	p := Balance{EntityType: "patient", EntityID: &pid, EntityName: "P", CurrencyID: cur.ID}
	if err := p.GetOrCreate(); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		bt := BalanceTransaction{FromBalanceID: p.ID, ToBalanceID: self.ID, Amount: float64(i + 1), CurrencyID: cur.ID, TransactionType: "payment"}
		if err := bt.Create(); err != nil {
			t.Fatal(err)
		}
	}

	var list BalanceTransactionList
	if err := list.GetByBalanceID(p.ID); err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3, got %d", len(list))
	}
	for _, bt := range list {
		if bt.FromEntityName == "" || bt.ToEntityName == "" {
			t.Errorf("expected joined entity names; got %+v", bt)
		}
	}
}

func TestBalanceTransactionList_GetEntityPayments_ExcludesCharges(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pid := "patient-y"
	p := Balance{EntityType: "patient", EntityID: &pid, EntityName: "P", CurrencyID: cur.ID}
	if err := p.GetOrCreate(); err != nil {
		t.Fatal(err)
	}

	// charge (should be excluded)
	c := BalanceTransaction{FromBalanceID: p.ID, ToBalanceID: self.ID, Amount: 200, CurrencyID: cur.ID, TransactionType: "charge"}
	if err := c.Create(); err != nil {
		t.Fatal(err)
	}
	// payment (should be included)
	pay := BalanceTransaction{FromBalanceID: p.ID, ToBalanceID: self.ID, Amount: 50, CurrencyID: cur.ID, TransactionType: "payment"}
	if err := pay.Create(); err != nil {
		t.Fatal(err)
	}
	// adjustment (should be included)
	adj := BalanceTransaction{FromBalanceID: self.ID, ToBalanceID: p.ID, Amount: 10, CurrencyID: cur.ID, TransactionType: "adjustment"}
	if err := adj.Create(); err != nil {
		t.Fatal(err)
	}

	var list BalanceTransactionList
	total, err := list.GetEntityPayments("patient", pid, ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("total = %d want 2 (charge excluded)", total)
	}
	for _, bt := range list {
		if bt.TransactionType == "charge" {
			t.Errorf("charge leaked into payments list: %+v", bt)
		}
	}

	// Without a specific entityID, returns ALL patient<->self transactions across all patients.
	list = nil
	total, _ = list.GetEntityPayments("patient", "", ListParams{})
	if total != 2 {
		t.Errorf("total without entityID = %d want 2", total)
	}
}

func TestBalanceTransactionList_GetAll_FiltersByDescription(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pid := "patient-f"
	p := Balance{EntityType: "patient", EntityID: &pid, EntityName: "P", CurrencyID: cur.ID}
	if err := p.GetOrCreate(); err != nil {
		t.Fatal(err)
	}

	bt1 := BalanceTransaction{FromBalanceID: p.ID, ToBalanceID: self.ID, Amount: 1, CurrencyID: cur.ID, TransactionType: "payment", Description: "uniqueQuery"}
	if err := bt1.Create(); err != nil {
		t.Fatal(err)
	}
	bt2 := BalanceTransaction{FromBalanceID: p.ID, ToBalanceID: self.ID, Amount: 2, CurrencyID: cur.ID, TransactionType: "payment", Description: "different"}
	if err := bt2.Create(); err != nil {
		t.Fatal(err)
	}

	var list BalanceTransactionList
	total, err := list.GetAll(ListParams{Filter: "uniqueQuery"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 || list[0].Description != "uniqueQuery" {
		t.Errorf("got total=%d list=%+v", total, list)
	}
}

func TestBalanceTransaction_Create_BadFromBalanceFails(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	bt := BalanceTransaction{FromBalanceID: "nope", ToBalanceID: self.ID, Amount: 1, CurrencyID: cur.ID, TransactionType: "payment"}
	err := bt.Create()
	if err == nil {
		t.Fatal("expected error for unknown from-balance")
	}
	// The schema's FK constraint catches this on insert, so no transaction row
	// should land in the table.
	if n := countRows(t, "balance_transactions", ""); n != 0 {
		t.Errorf("expected 0 tx rows after failure, got %d", n)
	}
}
