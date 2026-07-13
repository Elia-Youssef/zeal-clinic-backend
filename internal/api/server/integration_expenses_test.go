package server

import (
	"net/http"
	"testing"

	"clinic-api/internal/database/store"
)

func expenseBalanceID(t *testing.T, expenseID string) string {
	t.Helper()
	var id string
	if err := store.RDB.QueryRow(`
		SELECT id FROM balances
		WHERE entity_type = 'expense' AND entity_id = ? AND currency_id = ?`,
		expenseID, store.USDCurrencyID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestDeleteExpense_AllowsUnusedZeroBalance(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	expenseID := createExpense(t, e, tok, "Unused expense")
	balanceID := expenseBalanceID(t, expenseID)

	if amount := fetchBalanceAmount(t, balanceID); !approxEqualF(amount, 0) {
		t.Fatalf("initial expense balance = %v want 0", amount)
	}

	rec := doRequest(t, e, http.MethodDelete, "/api/expenses/"+expenseID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "expenses", "id = ?", expenseID); n != 0 {
		t.Errorf("expense still exists")
	}
	if n := countTableRows(t, "balances", "id = ?", balanceID); n != 0 {
		t.Errorf("expense balance still exists")
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/expenses/"+expenseID, nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("second delete: expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteExpense_RejectsNonZeroBalanceWithoutRelatedRows(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	expenseID := createExpense(t, e, tok, "Outstanding expense")
	balanceID := expenseBalanceID(t, expenseID)

	if _, err := store.DB.Exec(`UPDATE balances SET amount = 5 WHERE id = ?`, balanceID); err != nil {
		t.Fatal(err)
	}
	rec := doRequest(t, e, http.MethodDelete, "/api/expenses/"+expenseID, nil, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "expenses", "id = ?", expenseID); n != 1 {
		t.Errorf("expense was deleted")
	}
	if n := countTableRows(t, "balances", "id = ?", balanceID); n != 1 {
		t.Errorf("expense balance was deleted")
	}
}

func TestDeleteExpense_RejectsRelatedRowsWhenBalanceNetsToZero(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	expenseID := createExpense(t, e, tok, "Paid expense")
	balanceID := expenseBalanceID(t, expenseID)

	payment := map[string]any{
		"expenseId":         expenseID,
		"amount":            25,
		"transactionMethod": "cash",
	}
	rec := doRequest(t, e, http.MethodPost, "/api/expense-payments", asJSON(t, payment), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create payment: %d body=%s", rec.Code, rec.Body.String())
	}
	if amount := fetchBalanceAmount(t, balanceID); !approxEqualF(amount, 0) {
		t.Fatalf("expense balance = %v want 0", amount)
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/expenses/"+expenseID, nil, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "expenses", "id = ?", expenseID); n != 1 {
		t.Errorf("expense was deleted")
	}
	if n := countTableRows(t, "balances", "id = ?", balanceID); n != 1 {
		t.Errorf("expense balance was deleted")
	}
}
