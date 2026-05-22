package server

import (
	"net/http"
	"testing"
)

func TestCreateRoom_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/rooms",
		asJSON(t, map[string]any{"name": "Suite 1", "type": "Consultation", "isAvailable": true}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var r struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	errMsg, _ := decodeEnvelope(t, rec.Body, &r)
	if errMsg != "" {
		t.Fatalf("error: %s", errMsg)
	}
	if r.ID == "" || r.Name != "Suite 1" {
		t.Errorf("got %+v", r)
	}
}

func TestCreateRoom_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Invalid type fails OneOf.
	rec := doRequest(t, e, http.MethodPost, "/api/rooms",
		asJSON(t, map[string]any{"name": "Bad", "type": "Closet"}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "validation failed")
}

func TestUpdateRoom_PartialAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createRoom(t, e, tok, "Old", "General")
	rec := doRequest(t, e, http.MethodPut, "/api/rooms/"+id,
		asJSON(t, map[string]any{"id": "stripped", "name": "New", "isAvailable": false}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		IsAvailable bool   `json:"isAvailable"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Name != "New" || got.IsAvailable {
		t.Errorf("got %+v", got)
	}
	if got.ID != id {
		t.Errorf("ID changed")
	}

	rec = doRequest(t, e, http.MethodPut, "/api/rooms/ghost",
		asJSON(t, map[string]any{"name": "X"}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteRoom_SuccessAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createRoom(t, e, tok, "Doomed", "General")
	rec := doRequest(t, e, http.MethodDelete, "/api/rooms/"+id, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "rooms", "id = ?", id); n != 0 {
		t.Errorf("room not deleted")
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/rooms/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteRoom_BlockedByAppointment(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	pid := createPatientAndGetID(t, e, tok, "RoomDep")
	rid := createRoom(t, e, tok, "Busy", "Procedure")
	if rec := doRequest(t, e, http.MethodPost, "/api/appointments",
		asJSON(t, map[string]any{
			"patientId": pid, "roomId": rid,
			"startTime": "2026-09-01T10:00:00Z", "endTime": "2026-09-01T11:00:00Z",
		}), tok); rec.Code != http.StatusCreated {
		t.Fatalf("seed appointment: %d %s", rec.Code, rec.Body.String())
	}

	rec := doRequest(t, e, http.MethodDelete, "/api/rooms/"+rid, nil, tok)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 (room has appointments), got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRoomListAndDropdown(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Rooms are seeded by migration; capture the baseline count.
	baseline := countTableRows(t, "rooms", "")
	createRoom(t, e, tok, "Listed", "General")
	rec := doRequest(t, e, http.MethodGet, "/api/rooms", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	if page.Total != baseline+1 {
		t.Errorf("total = %d want %d (baseline + 1)", page.Total, baseline+1)
	}

	rec = doRequest(t, e, http.MethodGet, "/api/rooms/dropdown", nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("dropdown: %d", rec.Code)
	}
}
