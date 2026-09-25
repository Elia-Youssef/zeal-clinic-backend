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
	// An absent isActive defaults to 1.
	if d.IsActive != 1 {
		t.Errorf("isActive = %d want 1", d.IsActive)
	}
}

// TestCreateDiscount_ExplicitInactiveStaysInactive confirms a create request
// carrying isActive 0 stores the discount as inactive instead of falling back
// to the active default.
func TestCreateDiscount_ExplicitInactiveStaysInactive(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	payload := offerPayload("Planned Sale")
	payload["isActive"] = 0
	rec := doRequest(t, e, http.MethodPost, "/api/discounts", asJSON(t, payload), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var d struct {
		ID       string `json:"id"`
		IsActive int    `json:"isActive"`
	}
	errMsg, _ := decodeEnvelope(t, rec.Body, &d)
	if errMsg != "" {
		t.Fatalf("error: %s", errMsg)
	}
	if d.IsActive != 0 {
		t.Errorf("created isActive = %d want 0", d.IsActive)
	}

	rec = doRequest(t, e, http.MethodGet, "/api/discounts/"+d.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		IsActive int `json:"isActive"`
	}
	errMsg, _ = decodeEnvelope(t, rec.Body, &got)
	if errMsg != "" {
		t.Fatalf("error: %s", errMsg)
	}
	if got.IsActive != 0 {
		t.Errorf("stored isActive = %d want 0", got.IsActive)
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
		{"percentage over 100", map[string]any{"name": "x", "discountType": "offer", "valueType": "percentage", "value": 100.01}},
		{"invalid start date", map[string]any{"name": "x", "discountType": "offer", "valueType": "fixed", "value": 1, "startDate": "2026-02-30"}},
		{"invalid end date", map[string]any{"name": "x", "discountType": "offer", "valueType": "fixed", "value": 1, "endDate": "not-a-date"}},
		{"end before start", map[string]any{"name": "x", "discountType": "offer", "valueType": "fixed", "value": 1, "startDate": "2026-03-02", "endDate": "2026-03-01"}},
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

func TestUpdateDiscount_ValidationFailsWithoutChangingDiscount(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	body := offerPayload("Validated update")
	body["startDate"] = "2026-03-10"
	body["endDate"] = "2026-03-20"
	rec := doRequest(t, e, http.MethodPost, "/api/discounts", asJSON(t, body), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d body=%s", rec.Code, rec.Body.String())
	}
	var d struct{ ID string }
	decodeEnvelope(t, rec.Body, &d)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"negative value", map[string]any{"value": -1}},
		{"percentage over 100", map[string]any{"value": 101}},
		{"invalid start date", map[string]any{"startDate": "2026-02-30"}},
		{"start after stored end", map[string]any{"startDate": "2026-03-21"}},
		{"end before stored start", map[string]any{"endDate": "2026-03-09"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, e, http.MethodPut, "/api/discounts/"+d.ID, asJSON(t, tc.body), tok)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}

	rec = doRequest(t, e, http.MethodGet, "/api/discounts/"+d.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Value     float64 `json:"value"`
		StartDate string  `json:"startDate"`
		EndDate   string  `json:"endDate"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if !approxEqualF(got.Value, 10) || got.StartDate != "2026-03-10" || got.EndDate != "2026-03-20" {
		t.Errorf("invalid updates changed discount: %+v", got)
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

func TestGetAllDiscounts_ActiveFilter(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	activeRec := doRequest(t, e, http.MethodPost, "/api/discounts", asJSON(t, offerPayload("Enabled offer")), tok)
	if activeRec.Code != http.StatusCreated {
		t.Fatalf("create active offer: %d body=%s", activeRec.Code, activeRec.Body.String())
	}
	var active struct{ ID string }
	decodeEnvelope(t, activeRec.Body, &active)

	inactiveRec := doRequest(t, e, http.MethodPost, "/api/discounts", asJSON(t, offerPayload("Disabled offer")), tok)
	if inactiveRec.Code != http.StatusCreated {
		t.Fatalf("create inactive offer: %d body=%s", inactiveRec.Code, inactiveRec.Body.String())
	}
	var inactive struct{ ID string }
	decodeEnvelope(t, inactiveRec.Body, &inactive)
	if rec := doRequest(t, e, http.MethodPut, "/api/discounts/"+inactive.ID,
		asJSON(t, map[string]any{"isActive": 0}), tok); rec.Code != http.StatusOK {
		t.Fatalf("deactivate offer: %d body=%s", rec.Code, rec.Body.String())
	}

	for _, tc := range []struct {
		name     string
		query    string
		wantID   string
		rejectID string
	}{
		{name: "active", query: "active=true&filter=Enabled", wantID: active.ID, rejectID: inactive.ID},
		{name: "inactive", query: "active=false", wantID: inactive.ID, rejectID: active.ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, e, http.MethodGet, "/api/discounts?"+tc.query, nil, tok)
			if rec.Code != http.StatusOK {
				t.Fatalf("list: %d body=%s", rec.Code, rec.Body.String())
			}
			var page struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
				Total int `json:"total"`
			}
			decodeEnvelope(t, rec.Body, &page)
			if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != tc.wantID {
				t.Errorf("got total=%d items=%+v, want only %q", page.Total, page.Items, tc.wantID)
			}
			for _, item := range page.Items {
				if item.ID == tc.rejectID {
					t.Errorf("filtered-out discount %q was returned", tc.rejectID)
				}
			}
		})
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
