package server

import (
	"net/http"
	"testing"
)

// client payments

func TestClientPayment_CreateMovesBalances(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "Payer")

	body := asJSON(t, map[string]any{
		"patientId":         pid,
		"amount":            100.0,
		"currencyId":        curID,
		"transactionMethod": "cash",
		"description":       "deposit",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/client-payments", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var bt struct {
		ID              string  `json:"id"`
		TransactionType string  `json:"transactionType"`
		Amount          float64 `json:"amount"`
	}
	decodeEnvelope(t, rec.Body, &bt)
	if bt.ID == "" || bt.TransactionType != "payment" {
		t.Errorf("got %+v", bt)
	}
	if n := countTableRows(t, "balance_transactions", "transaction_type = 'payment' AND voided_at = ''"); n != 1 {
		t.Errorf("expected 1 active payment tx, got %d", n)
	}
}

func TestClientPayment_ValidationFailures(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "ValPayer")

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"missing patientId", map[string]any{"currencyId": curID, "amount": 1.0}, http.StatusBadRequest},
		{"negative amount", map[string]any{"patientId": pid, "currencyId": curID, "amount": -5}, http.StatusBadRequest},
		{"unknown patient", map[string]any{"patientId": "ghost", "currencyId": curID, "amount": 1.0}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, e, http.MethodPost, "/api/client-payments", asJSON(t, tc.body), tok)
			if rec.Code != tc.want {
				t.Errorf("code = %d want %d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestClientRefund_Create(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "Refundee")

	body := asJSON(t, map[string]any{
		"patientId": pid, "amount": 20.0, "currencyId": curID,
		"transactionMethod": "cash", "description": "refund",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/client-refunds", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("refund: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "balance_transactions", "transaction_type = 'refund'"); n != 1 {
		t.Errorf("expected 1 refund tx, got %d", n)
	}
}

func TestClientAdjustment_DirectionRequired(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "Adjustee")

	// Valid incoming adjustment.
	good := asJSON(t, map[string]any{
		"patientId": pid, "amount": 15.0, "currencyId": curID,
		"direction": "incoming", "description": "manual",
	})
	if rec := doRequest(t, e, http.MethodPost, "/api/client-adjustments", good, tok); rec.Code != http.StatusCreated {
		t.Fatalf("adjustment: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "balance_transactions", "transaction_type = 'adjustment'"); n != 1 {
		t.Errorf("expected 1 adjustment tx, got %d", n)
	}

	// Invalid direction returns 400.
	bad := asJSON(t, map[string]any{
		"patientId": pid, "amount": 15.0, "currencyId": curID,
		"direction": "sideways", "description": "manual",
	})
	if rec := doRequest(t, e, http.MethodPost, "/api/client-adjustments", bad, tok); rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad direction, got %d", rec.Code)
	}
}

func TestClientWriteOff_Create(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "WriteOff")

	body := asJSON(t, map[string]any{
		"patientId": pid, "amount": 35.0, "currencyId": curID,
		"direction": "incoming", "description": "forgive debt",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/client-write-offs", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("write-off: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "balance_transactions", "transaction_type = 'write-off'"); n != 1 {
		t.Errorf("expected 1 write-off tx, got %d", n)
	}
}

func TestClientPayment_DeleteVoidsTransaction(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "DelPay")

	rec := doRequest(t, e, http.MethodPost, "/api/client-payments",
		asJSON(t, map[string]any{
			"patientId": pid, "amount": 50.0, "currencyId": curID,
			"transactionMethod": "cash", "description": "p",
		}), tok)
	var bt struct{ ID string }
	decodeEnvelope(t, rec.Body, &bt)

	rec = doRequest(t, e, http.MethodDelete, "/api/client-payments/"+bt.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "balance_transactions", "id = ? AND voided_at != ''", bt.ID); n != 1 {
		t.Errorf("payment tx should be voided")
	}
}

func TestGetClientPayments_ListShape(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "Traceable")

	doRequest(t, e, http.MethodPost, "/api/client-payments",
		asJSON(t, map[string]any{
			"patientId": pid, "amount": 10.0, "currencyId": curID,
			"transactionMethod": "cash", "description": "p",
		}), tok)

	rec := doRequest(t, e, http.MethodGet, "/api/patients/"+pid+"/payments", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d body=%s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	if page.Total != 1 {
		t.Errorf("total = %d want 1", page.Total)
	}
}

// supplier payments

func TestSupplierPayment_Create(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	supID := createSupplier(t, e, tok, "PaidSupplier")

	body := asJSON(t, map[string]any{
		"supplierId": supID, "amount": 200.0, "currencyId": curID,
		"transactionMethod": "transfer", "description": "invoice 1",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/supplier-payments", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("supplier payment: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "balance_transactions", "transaction_type = 'payment'"); n != 1 {
		t.Errorf("expected 1 payment tx, got %d", n)
	}
}

func TestSupplierPayment_UnknownSupplier(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)

	body := asJSON(t, map[string]any{
		"supplierId": "ghost", "amount": 1.0, "currencyId": curID,
		"transactionMethod": "cash", "description": "x",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/supplier-payments", body, tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSupplierAdjustmentAndWriteOff(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	supID := createSupplier(t, e, tok, "AdjSupplier")

	adj := asJSON(t, map[string]any{
		"supplierId": supID, "amount": 10.0, "currencyId": curID,
		"direction": "outgoing", "description": "adj",
	})
	if rec := doRequest(t, e, http.MethodPost, "/api/supplier-adjustments", adj, tok); rec.Code != http.StatusCreated {
		t.Fatalf("supplier adjustment: %d body=%s", rec.Code, rec.Body.String())
	}

	wo := asJSON(t, map[string]any{
		"supplierId": supID, "amount": 5.0, "currencyId": curID,
		"direction": "outgoing", "description": "wo",
	})
	if rec := doRequest(t, e, http.MethodPost, "/api/supplier-write-offs", wo, tok); rec.Code != http.StatusCreated {
		t.Fatalf("supplier write-off: %d body=%s", rec.Code, rec.Body.String())
	}
}

// employee payments

func TestEmployeePayment_Create(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	empID := createEmployee(t, e, tok, "Paid", "Staff")

	body := asJSON(t, map[string]any{
		"employeeId": empID, "amount": 1000.0, "currencyId": curID,
		"transactionMethod": "transfer", "description": "salary",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/employee-payments", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("employee payment: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "balance_transactions", "transaction_type = 'payment'"); n != 1 {
		t.Errorf("expected 1 payment tx, got %d", n)
	}
}

func TestEmployeePayment_UnknownEmployee(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)

	body := asJSON(t, map[string]any{
		"employeeId": "ghost", "amount": 1.0, "currencyId": curID,
		"transactionMethod": "cash", "description": "x",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/employee-payments", body, tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEmployeeAdjustmentAndWriteOff(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	empID := createEmployee(t, e, tok, "Adj", "Staff")

	adj := asJSON(t, map[string]any{
		"employeeId": empID, "amount": 50.0, "currencyId": curID,
		"direction": "outgoing", "description": "bonus",
	})
	if rec := doRequest(t, e, http.MethodPost, "/api/employee-adjustments", adj, tok); rec.Code != http.StatusCreated {
		t.Fatalf("employee adjustment: %d body=%s", rec.Code, rec.Body.String())
	}

	wo := asJSON(t, map[string]any{
		"employeeId": empID, "amount": 5.0, "currencyId": curID,
		"direction": "incoming", "description": "deduct",
	})
	if rec := doRequest(t, e, http.MethodPost, "/api/employee-write-offs", wo, tok); rec.Code != http.StatusCreated {
		t.Fatalf("employee write-off: %d body=%s", rec.Code, rec.Body.String())
	}
}

// balances

func TestGetAllBalances_ByType(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// The self balance is seeded for USD.
	rec := doRequest(t, e, http.MethodGet, "/api/balances/self", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("balances: %d body=%s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	if page.Total < 1 {
		t.Errorf("expected at least 1 self balance, got %d", page.Total)
	}
}

func TestGetEntityBalance(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "BalCheck")

	// Make a payment so the patient has a balance row.
	doRequest(t, e, http.MethodPost, "/api/client-payments",
		asJSON(t, map[string]any{
			"patientId": pid, "amount": 10.0, "currencyId": curID,
			"transactionMethod": "cash", "description": "p",
		}), tok)

	rec := doRequest(t, e, http.MethodGet, "/api/balances/patient/"+pid, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("entity balance: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBalances_RequiresAuth(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/api/balances/self", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
