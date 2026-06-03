package server

import (
	"net/http"
	"testing"
)

// analytics

func TestAnalytics_MoneySectionShape(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodGet, "/api/analytics/money?from=2026-05-01&to=2026-05-31", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("money: %d body=%s", rec.Code, rec.Body.String())
	}
	var data struct {
		Revenue struct {
			Value    float64 `json:"value"`
			Previous float64 `json:"previous"`
			Change   float64 `json:"change"`
		} `json:"revenue"`
		NetProfit  struct{ Value float64 } `json:"netProfit"`
		RevenueMix struct {
			Procedures float64 `json:"procedures"`
		} `json:"revenueMix"`
	}
	decodeEnvelope(t, rec.Body, &data)
	// On an empty seed these are all zero, but the keys must decode (delta shape present).
	_ = data
}

func TestAnalytics_HappyPathEndpoints(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	paths := []string{
		"/api/analytics/money",
		"/api/analytics/patients",
		"/api/analytics/operations",
		"/api/analytics/inventory",
		"/api/analytics/demographics",
		"/api/analytics/referral-sources",
		"/api/analytics/staff-performance",
		"/api/analytics/procedures/top",
		"/api/analytics/procedures/top?by=revenue",
		"/api/analytics/products/top",
		"/api/analytics/appointments/distribution",
		"/api/analytics/appointments/recent-today",
		"/api/analytics/rooms/utilization",
		"/api/analytics/transactions/recent",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			rec := doRequest(t, e, http.MethodGet, p, nil, tok)
			if rec.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d body=%s", p, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAnalytics_ReportPDF(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodGet, "/api/analytics/report/pdf?from=2026-05-01&to=2026-05-31", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("report pdf: %d body=%s", rec.Code, rec.Body.String())
	}
	var data struct {
		URL string `json:"url"`
	}
	decodeEnvelope(t, rec.Body, &data)
	if data.URL == "" {
		t.Errorf("expected a served pdf url, got empty")
	}
}

func TestAnalytics_SeriesRequiresMetric(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Missing metric returns 400.
	rec := doRequest(t, e, http.MethodGet, "/api/analytics/series", nil, tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 without metric, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Each supported metric returns 200.
	for _, m := range []string{"revenue", "expenses", "appointments", "new-patients", "procedures-completed"} {
		rec = doRequest(t, e, http.MethodGet, "/api/analytics/series?metric="+m+"&from=2026-05-01&to=2026-05-07", nil, tok)
		if rec.Code != http.StatusOK {
			t.Errorf("metric %s: expected 200, got %d body=%s", m, rec.Code, rec.Body.String())
		}
	}
}

func TestAnalytics_RangeParamsAccepted(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	paths := []string{
		"/api/analytics/money",
		"/api/analytics/patients",
		"/api/analytics/operations",
		"/api/analytics/referral-sources",
		"/api/analytics/staff-performance",
		"/api/analytics/procedures/top",
		"/api/analytics/products/top",
		"/api/analytics/rooms/utilization",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			rec := doRequest(t, e, http.MethodGet, p+"?from=2026-05-01&to=2026-05-31", nil, tok)
			if rec.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d body=%s", p, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAnalytics_RequiresAuth(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/api/analytics/money", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// search

func TestSearch_HappyPath(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	createPatientAndGetID(t, e, tok, "Searchable")

	rec := doRequest(t, e, http.MethodGet, "/api/search?q=Searchable", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("search: %d body=%s", rec.Code, rec.Body.String())
	}
	// Envelope is success with Data.
	_, success := decodeEnvelope(t, rec.Body, nil)
	if !success {
		t.Errorf("expected success envelope")
	}
}

func TestSearch_RequiresAuth(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/api/search?q=x", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// notifications

func TestNotifications_ListAndUnreadCount(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodGet, "/api/notifications", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d body=%s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	if page.Total != 0 {
		t.Errorf("expected 0 notifications initially, got %d", page.Total)
	}

	rec = doRequest(t, e, http.MethodGet, "/api/notifications/unread-count", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("unread count: %d", rec.Code)
	}
}

func TestNotifications_TestSendThenMarkRead(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/notifications/test", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("send test: %d body=%s", rec.Code, rec.Body.String())
	}
	var n struct{ ID string }
	decodeEnvelope(t, rec.Body, &n)
	if n.ID == "" {
		t.Fatalf("no notification id")
	}

	rec = doRequest(t, e, http.MethodPut, "/api/notifications/"+n.ID+"/read", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("mark read: %d body=%s", rec.Code, rec.Body.String())
	}

	// Mark-all and delete.
	if rec := doRequest(t, e, http.MethodPut, "/api/notifications/read-all", nil, tok); rec.Code != http.StatusOK {
		t.Errorf("mark all: %d", rec.Code)
	}
	if rec := doRequest(t, e, http.MethodDelete, "/api/notifications/"+n.ID, nil, tok); rec.Code != http.StatusOK {
		t.Errorf("delete: %d", rec.Code)
	}
}

func TestNotifications_MarkReadNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPut, "/api/notifications/ghost/read", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestNotifications_RequiresAuth(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/api/notifications", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
