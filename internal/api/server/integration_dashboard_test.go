package server

import (
	"net/http"
	"testing"
)

// analytics

func TestAnalytics_TotalPatients(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	createPatientAndGetID(t, e, tok, "Counted")

	rec := doRequest(t, e, http.MethodGet, "/api/analytics/patients/total", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("total patients: %d body=%s", rec.Code, rec.Body.String())
	}
	var data struct {
		Total int `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &data)
	if data.Total != 1 {
		t.Errorf("total = %d want 1", data.Total)
	}
}

func TestAnalytics_HappyPathEndpoints(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	paths := []string{
		"/api/analytics/patients/new-this-month",
		"/api/analytics/appointments/counts",
		"/api/analytics/appointments/recent-today",
		"/api/analytics/appointments/cancellation-rate",
		"/api/analytics/revenue/this-month",
		"/api/analytics/revenue/outstanding",
		"/api/analytics/expenses/this-month",
		"/api/analytics/transactions/recent",
		"/api/analytics/procedures/completed-this-month",
		"/api/analytics/procedures/top",
		"/api/analytics/inventory/low-stock",
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

func TestAnalytics_SeriesRequiresMetric(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Missing metric returns 400.
	rec := doRequest(t, e, http.MethodGet, "/api/analytics/series", nil, tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 without metric, got %d body=%s", rec.Code, rec.Body.String())
	}

	// A valid metric returns 200.
	rec = doRequest(t, e, http.MethodGet, "/api/analytics/series?metric=revenue&from=2026-05-01&to=2026-05-07", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with metric, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAnalytics_RequiresAuth(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/api/analytics/patients/total", nil, "")
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
