package server

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

// createRoom creates a room via the API and returns its ID.
func createRoom(t *testing.T, e *echo.Echo, tok, name, roomType string) string {
	t.Helper()
	rec := doRequest(t, e, http.MethodPost, "/api/rooms",
		asJSON(t, map[string]any{"name": name, "type": roomType, "isAvailable": true}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create room: %d %s", rec.Code, rec.Body.String())
	}
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)
	return c.ID
}

func TestCreateAppointment_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	pid := createPatientAndGetID(t, e, tok, "Apt")
	rid := createRoom(t, e, tok, "Room A", "Procedure")

	body := asJSON(t, map[string]any{
		"patientId": pid,
		"roomId":    rid,
		"startTime": "2026-06-01T10:00:00Z",
		"endTime":   "2026-06-01T11:00:00Z",
		"notes":     "first",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/appointments", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apt struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	errMsg, _ := decodeEnvelope(t, rec.Body, &apt)
	if errMsg != "" {
		t.Fatalf("error in envelope: %s", errMsg)
	}
	if apt.ID == "" {
		t.Errorf("no id returned")
	}
	// Status defaults to Scheduled when omitted.
	if apt.Status != "Scheduled" {
		t.Errorf("status = %q want Scheduled", apt.Status)
	}
	if n := countTableRows(t, "appointments", "id = ?", apt.ID); n != 1 {
		t.Errorf("expected 1 appointment row, got %d", n)
	}
}

func TestCreateAppointment_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Missing patientId/roomId/times.
	rec := doRequest(t, e, http.MethodPost, "/api/appointments",
		asJSON(t, map[string]any{"notes": "incomplete"}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "Please check your input")
}

func TestCreateAppointment_RoomConflict(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	pid := createPatientAndGetID(t, e, tok, "Conf")
	rid := createRoom(t, e, tok, "Room Conf", "Procedure")

	body := asJSON(t, map[string]any{
		"patientId": pid, "roomId": rid,
		"startTime": "2026-06-02T10:00:00Z", "endTime": "2026-06-02T11:00:00Z",
	})
	if rec := doRequest(t, e, http.MethodPost, "/api/appointments", body, tok); rec.Code != http.StatusCreated {
		t.Fatalf("seed appointment: %d %s", rec.Code, rec.Body.String())
	}

	// Overlapping booking in the same room: the store returns a room conflict
	// error, which the create handler maps to 500 (it has no conflict branch).
	overlap := asJSON(t, map[string]any{
		"patientId": pid, "roomId": rid,
		"startTime": "2026-06-02T10:30:00Z", "endTime": "2026-06-02T11:30:00Z",
	})
	rec := doRequest(t, e, http.MethodPost, "/api/appointments", overlap, tok)
	// CreateAppointment has no explicit "room conflict" branch, so the conflict
	// surfaces as a 500 "failed to create appointment". Assert actual behavior.
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 (no conflict branch on create), got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetAllAppointments_RequiresDate(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// No date query param returns 400.
	rec := doRequest(t, e, http.MethodGet, "/api/appointments", nil, tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 without date, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetAllAppointments_ListShape(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	pid := createPatientAndGetID(t, e, tok, "List")
	rid := createRoom(t, e, tok, "Room List", "Procedure")
	body := asJSON(t, map[string]any{
		"patientId": pid, "roomId": rid,
		"startTime": "2026-06-03T10:00:00Z", "endTime": "2026-06-03T11:00:00Z",
	})
	if rec := doRequest(t, e, http.MethodPost, "/api/appointments", body, tok); rec.Code != http.StatusCreated {
		t.Fatalf("seed: %d %s", rec.Code, rec.Body.String())
	}

	rec := doRequest(t, e, http.MethodGet, "/api/appointments?date=2026-06-03", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d body=%s", rec.Code, rec.Body.String())
	}
	// The appointments list endpoint returns {items, total, holidays}.
	var data struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &data)
	if data.Total != 1 {
		t.Errorf("total = %d want 1", data.Total)
	}
	if len(data.Items) != 1 {
		t.Errorf("items len = %d want 1", len(data.Items))
	}
}

func TestUpdateAppointment_StatusTransition(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	pid := createPatientAndGetID(t, e, tok, "Trans")
	otherPID := createPatientAndGetID(t, e, tok, "OtherTrans")
	rid := createRoom(t, e, tok, "Room Trans", "Procedure")
	rec := doRequest(t, e, http.MethodPost, "/api/appointments",
		asJSON(t, map[string]any{
			"patientId": pid, "roomId": rid,
			"startTime": "2026-06-04T10:00:00Z", "endTime": "2026-06-04T11:00:00Z",
		}), tok)
	var apt struct{ ID string }
	decodeEnvelope(t, rec.Body, &apt)

	rec = doRequest(t, e, http.MethodPut, "/api/appointments/"+apt.ID,
		asJSON(t, map[string]any{"id": "stripped", "patientId": otherPID, "status": "Completed", "completionNotes": "done"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID              string `json:"id"`
		PatientID       string `json:"patientId"`
		Status          string `json:"status"`
		CompletionNotes string `json:"completionNotes"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Status != "Completed" {
		t.Errorf("status = %q want Completed", got.Status)
	}
	if got.ID != apt.ID {
		t.Errorf("ID changed")
	}
	if got.PatientID != pid {
		t.Errorf("patientId = %q want original %q", got.PatientID, pid)
	}
	if n := countTableRows(t, "appointments", "id = ? AND patient_id = ?", apt.ID, pid); n != 1 {
		t.Errorf("appointment patient changed")
	}
}

func TestUpdateAppointment_NotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodPut, "/api/appointments/ghost",
		asJSON(t, map[string]any{"status": "Cancelled"}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRescheduleAppointment_CreatesNewAndMarksOld(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	pid := createPatientAndGetID(t, e, tok, "Resched")
	otherPID := createPatientAndGetID(t, e, tok, "OtherResched")
	rid := createRoom(t, e, tok, "Room Resched", "Procedure")
	rec := doRequest(t, e, http.MethodPost, "/api/appointments",
		asJSON(t, map[string]any{
			"patientId": pid, "roomId": rid,
			"startTime": "2026-06-05T10:00:00Z", "endTime": "2026-06-05T11:00:00Z",
		}), tok)
	var old struct{ ID string }
	decodeEnvelope(t, rec.Body, &old)

	rec = doRequest(t, e, http.MethodPost, "/api/appointments/"+old.ID+"/reschedule",
		asJSON(t, map[string]any{
			"patientId": otherPID,
			"startTime": "2026-06-06T10:00:00Z", "endTime": "2026-06-06T11:00:00Z",
		}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("reschedule: %d body=%s", rec.Code, rec.Body.String())
	}
	var nu struct {
		ID              string `json:"id"`
		PatientID       string `json:"patientId"`
		RescheduledFrom string `json:"rescheduledFrom"`
		Status          string `json:"status"`
	}
	decodeEnvelope(t, rec.Body, &nu)
	if nu.ID == old.ID {
		t.Errorf("reschedule should create a new appointment row")
	}
	if nu.RescheduledFrom != old.ID {
		t.Errorf("rescheduledFrom = %q want %q", nu.RescheduledFrom, old.ID)
	}
	if nu.PatientID != pid {
		t.Errorf("patientId = %q want original %q", nu.PatientID, pid)
	}
	if n := countTableRows(t, "appointments", "id = ? AND patient_id = ?", nu.ID, pid); n != 1 {
		t.Errorf("rescheduled appointment patient changed")
	}
	// Old appointment should now be marked Rescheduled.
	if n := countTableRows(t, "appointments", "id = ? AND status = 'Rescheduled'", old.ID); n != 1 {
		t.Errorf("old appointment not marked Rescheduled")
	}
}

func TestDeleteAppointment_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	pid := createPatientAndGetID(t, e, tok, "Del")
	rid := createRoom(t, e, tok, "Room Del", "Procedure")
	rec := doRequest(t, e, http.MethodPost, "/api/appointments",
		asJSON(t, map[string]any{
			"patientId": pid, "roomId": rid,
			"startTime": "2026-06-07T10:00:00Z", "endTime": "2026-06-07T11:00:00Z",
		}), tok)
	var apt struct{ ID string }
	decodeEnvelope(t, rec.Body, &apt)

	rec = doRequest(t, e, http.MethodDelete, "/api/appointments/"+apt.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "appointments", "id = ?", apt.ID); n != 0 {
		t.Errorf("appointment not deleted")
	}
}

func TestDeleteAppointment_NotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodDelete, "/api/appointments/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestAppointments_RequiresAuth(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/api/appointments?date=2026-06-01", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", rec.Code)
	}
}

// employee-schedules

func TestEmployeeSchedule_CreateAndDelete(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	empID := createEmployee(t, e, tok, "Sched", "Avail")

	// A weekday is saved whole: the body lists every shift from startDate on.
	body := asJSON(t, map[string]any{
		"employeeId": empID,
		"dayOfWeek":  1,
		"startDate":  "2026-01-01",
		"shifts":     []map[string]any{{"startTime": "09:00", "endTime": "17:00"}},
	})
	rec := doRequest(t, e, http.MethodPut, "/api/employee-schedules/day", body, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("save schedule day: %d body=%s", rec.Code, rec.Body.String())
	}
	var shifts []struct {
		ID       string `json:"id"`
		IsActive bool   `json:"isActive"`
	}
	decodeEnvelope(t, rec.Body, &shifts)
	if len(shifts) != 1 {
		t.Fatalf("expected 1 shift row, got %d (body=%s)", len(shifts), rec.Body.String())
	}
	sa := shifts[0]
	if sa.ID == "" {
		t.Fatalf("no id returned")
	}
	if !sa.IsActive {
		t.Errorf("new schedule should be active")
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/employee-schedules/"+sa.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete schedule: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "employee_schedules", "id = ?", sa.ID); n != 0 {
		t.Errorf("schedule not deleted")
	}
}

func TestEmployeeSchedule_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// Missing employeeId, bad dayOfWeek.
	rec := doRequest(t, e, http.MethodPut, "/api/employee-schedules/day",
		asJSON(t, map[string]any{"dayOfWeek": 9}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "Please check your input")
}

func TestEmployeeSchedule_DeleteNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodDelete, "/api/employee-schedules/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}
