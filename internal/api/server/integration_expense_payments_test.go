package server

import (
	"net/http"
	"testing"

	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
)

func createExpense(t *testing.T, e *echo.Echo, tok, name string) string {
	t.Helper()
	rec := doRequest(t, e, http.MethodPost, "/api/expenses",
		asJSON(t, map[string]any{"name": name}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create expense: %d %s", rec.Code, rec.Body.String())
	}
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)
	return c.ID
}

func TestExpensePayment_CreateTwoWayBalancesNetToZero(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	curID := firstSeededCurrencyID(t)
	expID := createExpense(t, e, tok, "Office Rent")

	// Pre-state: capture self balance.
	var selfBalanceID string
	if err := store.RDB.QueryRow(
		`SELECT id FROM balances WHERE entity_type = 'self' AND currency_id = ?`,
		curID).Scan(&selfBalanceID); err != nil {
		t.Fatal(err)
	}
	preSelfAmount := fetchBalanceAmount(t, selfBalanceID)

	body := asJSON(t, map[string]any{
		"expenseId":         expID,
		"amount":            500,
		"currencyId":        curID,
		"transactionMethod": "cash",
		"description":       "rent jan",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/expense-payments", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expense payment: %d %s", rec.Code, rec.Body.String())
	}

	var pay struct {
		ID            string `json:"id"`
		Amount        float64
		FromBalanceID string `json:"fromBalanceId"`
		ToBalanceID   string `json:"toBalanceId"`
	}
	decodeEnvelope(t, rec.Body, &pay)

	// Self balance should net to its previous amount (charge + payment cancel).
	if !approxEqualF(fetchBalanceAmount(t, selfBalanceID), preSelfAmount) {
		t.Errorf("self balance after two-way payment = %v want %v",
			fetchBalanceAmount(t, selfBalanceID), preSelfAmount)
	}

	// Expense balance should net to 0.
	var expBalanceID string
	if err := store.RDB.QueryRow(
		`SELECT id FROM balances WHERE entity_type = 'expense' AND entity_id = ? AND currency_id = ?`,
		expID, curID).Scan(&expBalanceID); err != nil {
		t.Fatal(err)
	}
	if !approxEqualF(fetchBalanceAmount(t, expBalanceID), 0) {
		t.Errorf("expense balance = %v want 0", fetchBalanceAmount(t, expBalanceID))
	}

	// Two transaction rows should exist (charge + payment).
	if n := countTableRows(t, "balance_transactions", ""); n != 2 {
		t.Errorf("expected 2 tx rows, got %d", n)
	}
	if n := countTableRows(t, "balance_transactions", "transaction_type = 'charge'"); n != 1 {
		t.Errorf("expected 1 charge, got %d", n)
	}
	if n := countTableRows(t, "balance_transactions", "transaction_type = 'payment'"); n != 1 {
		t.Errorf("expected 1 payment, got %d", n)
	}

	// total_in / total_out should reflect the payment leg only (charge excluded).
	var selfOut float64
	store.RDB.QueryRow(`SELECT total_out FROM balances WHERE id = ?`, selfBalanceID).Scan(&selfOut)
	if !approxEqualF(selfOut, 500) {
		t.Errorf("self total_out = %v want 500", selfOut)
	}
	var expIn float64
	store.RDB.QueryRow(`SELECT total_in FROM balances WHERE id = ?`, expBalanceID).Scan(&expIn)
	if !approxEqualF(expIn, 500) {
		t.Errorf("expense total_in = %v want 500", expIn)
	}
}

func TestExpensePayment_DeleteRemovesPair(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	expID := createExpense(t, e, tok, "Utilities")

	body := asJSON(t, map[string]any{
		"expenseId":         expID,
		"amount":            120,
		"currencyId":        curID,
		"transactionMethod": "cash",
		"description":       "feb electric",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/expense-payments", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatal(rec.Code)
	}
	var pay struct{ ID string }
	decodeEnvelope(t, rec.Body, &pay)

	if n := countTableRows(t, "balance_transactions", ""); n != 2 {
		t.Fatalf("expected 2 tx rows, got %d", n)
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/expense-payments/"+pay.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "balance_transactions", "voided_at = ''"); n != 0 {
		t.Errorf("expected 0 active tx rows after delete (pair voided), got %d", n)
	}
	if n := countTableRows(t, "balance_transactions", "voided_at != ''"); n != 2 {
		t.Errorf("expected 2 voided tx rows after delete, got %d", n)
	}
}

func TestExpensePayment_ValidationFailures(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	expID := createExpense(t, e, tok, "X")

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{
			"missing expenseId",
			map[string]any{"currencyId": curID, "amount": 1},
			http.StatusBadRequest,
		},
		{
			"zero amount",
			map[string]any{"expenseId": expID, "currencyId": curID, "amount": 0},
			http.StatusBadRequest,
		},
		{
			"negative amount",
			map[string]any{"expenseId": expID, "currencyId": curID, "amount": -1},
			http.StatusBadRequest,
		},
		{
			"unknown expense",
			map[string]any{"expenseId": "ghost", "currencyId": curID, "amount": 1},
			http.StatusBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, e, http.MethodPost, "/api/expense-payments", asJSON(t, tc.body), tok)
			if rec.Code != tc.want {
				t.Errorf("code = %d want %d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestExpenseAdjustment_Outgoing(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	expID := createExpense(t, e, tok, "Adj")

	body := asJSON(t, map[string]any{
		"expenseId":   expID,
		"currencyId":  curID,
		"amount":      30,
		"direction":   "outgoing",
		"description": "manual adjustment",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/expense-adjustments", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expense adjustment: %d body=%s", rec.Code, rec.Body.String())
	}
	// Should have created a single 'adjustment' tx.
	if n := countTableRows(t, "balance_transactions", "transaction_type = 'adjustment'"); n != 1 {
		t.Errorf("expected 1 adjustment, got %d", n)
	}
	// Adjustment should NOT move flow totals (capture this).
	var selfBalanceID string
	store.RDB.QueryRow(`SELECT id FROM balances WHERE entity_type='self' AND currency_id=?`, curID).Scan(&selfBalanceID)
	var selfOut float64
	store.RDB.QueryRow(`SELECT total_out FROM balances WHERE id = ?`, selfBalanceID).Scan(&selfOut)
	if !approxEqualF(selfOut, 0) {
		t.Errorf("adjustment should not move total_out, got %v", selfOut)
	}
}

func TestExpenseAdjustment_DirectionRequired(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	expID := createExpense(t, e, tok, "Bad")

	body := asJSON(t, map[string]any{
		"expenseId":   expID,
		"currencyId":  curID,
		"amount":      10,
		"description": "x",
		"direction":   "sideways", // invalid
	})
	rec := doRequest(t, e, http.MethodPost, "/api/expense-adjustments", body, tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExpenseWriteOff_Outgoing(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	expID := createExpense(t, e, tok, "WO")

	body := asJSON(t, map[string]any{
		"expenseId":   expID,
		"currencyId":  curID,
		"amount":      40,
		"direction":   "outgoing",
		"description": "write off",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/expense-write-offs", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expense write-off: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "balance_transactions", "transaction_type = 'write-off'"); n != 1 {
		t.Errorf("expected 1 write-off, got %d", n)
	}
}

func TestGetExpensePayments(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	expID := createExpense(t, e, tok, "Trace")

	body := asJSON(t, map[string]any{
		"expenseId":         expID,
		"amount":            10,
		"currencyId":        curID,
		"transactionMethod": "cash",
		"description":       "p",
	})
	if rec := doRequest(t, e, http.MethodPost, "/api/expense-payments", body, tok); rec.Code != http.StatusCreated {
		t.Fatal(rec.Code)
	}

	rec := doRequest(t, e, http.MethodGet, "/api/expenses/"+expID+"/payments", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d body=%s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	// Charges are excluded from the payments listing.
	if page.Total != 1 {
		t.Errorf("expected 1 (payment leg only), got %d", page.Total)
	}
}
