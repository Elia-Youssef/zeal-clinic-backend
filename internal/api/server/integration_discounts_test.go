package server

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

func offerPayload(name string) map[string]any {
	return map[string]any{
		"name":         name,
		"discountType": "offer",
		"valueType":    "percentage",
		"value":        10.0,
	}
}

func TestCreateDiscount_OfferSuccess(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/discounts", asJSON(t, offerPayload("Spring Sale")), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var d struct {
		ID           string `json:"id"`
		DiscountType string `json:"discountType"`
		IsActive     int    `json:"isActive"`
	}
	errMsg, _ := decodeEnvelope(t, rec.Body, &d)
	if errMsg != "" {
		t.Fatalf("error: %s", errMsg)
	}
	if d.ID == "" || d.DiscountType != "offer" {
		t.Errorf("got %+v", d)
	}
	// Create defaults IsActive to 1.
	if d.IsActive != 1 {
		t.Errorf("isActive = %d want 1", d.IsActive)
	}
}

// TestCreateDiscount_GiftRejected confirms gift discounts cannot be created
// directly through POST /api/discounts (only via invoice lines).
func TestCreateDiscount_GiftRejected(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	body := map[string]any{
		"name":         "Gift50",
		"discountType": "gift",
		"valueType":    "fixed",
		"value":        50.0,
		"code":         "GIFT50",
	}
	rec := doRequest(t, e, http.MethodPost, "/api/discounts", asJSON(t, body), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "Gift cards can only be created through an invoice")
}

func TestCreateDiscount_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"missing name", map[string]any{"discountType": "offer", "valueType": "fixed", "value": 1.0}},
		{"bad discountType", map[string]any{"name": "x", "discountType": "weird", "valueType": "fixed", "value": 1.0}},
		{"bad valueType", map[string]any{"name": "x", "discountType": "offer", "valueType": "weird", "value": 1.0}},
		{"negative value", map[string]any{"name": "x", "discountType": "offer", "valueType": "fixed", "value": -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, e, http.MethodPost, "/api/discounts", asJSON(t, tc.body), tok)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGetDiscountByID_FoundAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/discounts", asJSON(t, offerPayload("Findable")), tok)
	var d struct{ ID string }
	decodeEnvelope(t, rec.Body, &d)

	rec = doRequest(t, e, http.MethodGet, "/api/discounts/"+d.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodGet, "/api/discounts/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateDiscount_PartialAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/discounts", asJSON(t, offerPayload("Editable")), tok)
	var d struct{ ID string }
	decodeEnvelope(t, rec.Body, &d)

	rec = doRequest(t, e, http.MethodPut, "/api/discounts/"+d.ID,
		asJSON(t, map[string]any{"id": "stripped", "name": "Edited", "value": 25.0}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID    string  `json:"id"`
		Name  string  `json:"name"`
		Value float64 `json:"value"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Name != "Edited" || !approxEqualF(got.Value, 25) {
		t.Errorf("got %+v", got)
	}
	if got.ID != d.ID {
		t.Errorf("ID changed")
	}

	rec = doRequest(t, e, http.MethodPut, "/api/discounts/ghost",
		asJSON(t, map[string]any{"name": "X"}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteDiscount_SuccessAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/discounts", asJSON(t, offerPayload("Doomed")), tok)
	var d struct{ ID string }
	decodeEnvelope(t, rec.Body, &d)

	rec = doRequest(t, e, http.MethodDelete, "/api/discounts/"+d.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "discounts", "id = ?", d.ID); n != 0 {
		t.Errorf("discount not deleted")
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/discounts/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestGetAllDiscounts_ListShape(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	doRequest(t, e, http.MethodPost, "/api/discounts", asJSON(t, offerPayload("L1")), tok)
	doRequest(t, e, http.MethodPost, "/api/discounts", asJSON(t, offerPayload("L2")), tok)

	rec := doRequest(t, e, http.MethodGet, "/api/discounts", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	if page.Total != 2 {
		t.Errorf("total = %d want 2", page.Total)
	}
}

// gift-card redeem

// createGiftViaInvoice creates a gift discount through a client invoice line
// (the only supported path) and returns the gift discount ID and code.
func createGiftViaInvoice(t *testing.T, e *echo.Echo, tok, patientID, currencyID, code string, value float64) string {
	t.Helper()
	body := asJSON(t, map[string]any{
		"patientId":  patientID,
		"currencyId": currencyID,
		"items": []map[string]any{
			{
				"itemType": "gift",
				"quantity": 1,
				"amount":   value,
				"giftCode": code,
				"giftName": "Gift " + code,
			},
		},
	})
	rec := doRequest(t, e, http.MethodPost, "/api/client-invoices", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create gift invoice: %d %s", rec.Code, rec.Body.String())
	}
	// The gift discount row is created with the supplied code.
	var giftID string
	if err := getStoreDB().QueryRow(`SELECT id FROM discounts WHERE code = ? AND discount_type = 'gift'`, code).Scan(&giftID); err != nil {
		t.Fatalf("gift discount not created: %v", err)
	}
	return giftID
}

func TestRedeemGiftCard_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	curID := firstSeededCurrencyID(t)
	// Patient who buys the gift.
	buyer := createPatientAndGetID(t, e, tok, "Buyer")
	// Patient who redeems it.
	redeemer := createPatientAndGetID(t, e, tok, "Redeemer")

	createGiftViaInvoice(t, e, tok, buyer, curID, "REDEEM-OK", 40.0)

	rec := doRequest(t, e, http.MethodPost, "/api/gift-cards/redeem",
		asJSON(t, map[string]any{
			"code":       "REDEEM-OK",
			"patientId":  redeemer,
			"currencyId": curID,
		}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("redeem: %d body=%s", rec.Code, rec.Body.String())
	}
	// Gift is now marked redeemed.
	if n := countTableRows(t, "discounts", "code = 'REDEEM-OK' AND redeemed_at IS NOT NULL AND redeemed_at != ''"); n != 1 {
		t.Errorf("gift should be marked redeemed")
	}
}

func TestRedeemGiftCard_NotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	curID := firstSeededCurrencyID(t)
	pid := createPatientAndGetID(t, e, tok, "NoGift")

	rec := doRequest(t, e, http.MethodPost, "/api/gift-cards/redeem",
		asJSON(t, map[string]any{"code": "NONEXISTENT", "patientId": pid, "currencyId": curID}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRedeemGiftCard_AlreadyRedeemed(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	curID := firstSeededCurrencyID(t)
	buyer := createPatientAndGetID(t, e, tok, "Buyer2")
	redeemer := createPatientAndGetID(t, e, tok, "Redeemer2")
	createGiftViaInvoice(t, e, tok, buyer, curID, "ONCE-ONLY", 30.0)

	body := asJSON(t, map[string]any{"code": "ONCE-ONLY", "patientId": redeemer, "currencyId": curID})
	if rec := doRequest(t, e, http.MethodPost, "/api/gift-cards/redeem", body, tok); rec.Code != http.StatusOK {
		t.Fatalf("first redeem: %d %s", rec.Code, rec.Body.String())
	}
	// Second redemption is rejected (single-use) with 400.
	rec := doRequest(t, e, http.MethodPost, "/api/gift-cards/redeem", body, tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on re-redeem, got %d body=%s", rec.Code, rec.Body.String())
	}
}
