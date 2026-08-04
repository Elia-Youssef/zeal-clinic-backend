package server

import (
	"net/http"
	"testing"

	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
)

// employeePayload returns a minimal valid CreateEmployee body.
func employeePayload(first, last string) map[string]any {
	return map[string]any{
		"firstName":      first,
		"lastName":       last,
		"role":           "Nurse",
		"contact":        "0700123456",
		"email":          "",
		"employmentType": "Full-time",
	}
}

// createEmployee creates an employee via the API and returns its ID.
func createEmployee(t *testing.T, e *echo.Echo, tok, first, last string) string {
	t.Helper()
	rec := doRequest(t, e, http.MethodPost, "/api/employees", asJSON(t, employeePayload(first, last)), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create employee: %d %s", rec.Code, rec.Body.String())
	}
	var c struct{ ID string }
	decodeEnvelope(t, rec.Body, &c)
	return c.ID
}

func TestCreateEmployee_Success(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/employees", asJSON(t, employeePayload("Eve", "Worker")), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var emp struct {
		ID        string `json:"id"`
		FirstName string `json:"firstName"`
	}
	errMsg, _ := decodeEnvelope(t, rec.Body, &emp)
	if errMsg != "" {
		t.Fatalf("error in envelope: %s", errMsg)
	}
	if emp.ID == "" || emp.FirstName != "Eve" {
		t.Errorf("got %+v", emp)
	}
	if n := countTableRows(t, "employees", "id = ?", emp.ID); n != 1 {
		t.Errorf("expected 1 employee row, got %d", n)
	}
	// Employee.Create seeds a balance per currency. At least one should exist.
	if n := countTableRows(t, "balances", "entity_type = 'employee' AND entity_id = ?", emp.ID); n < 1 {
		t.Errorf("expected employee balances to be seeded, got %d", n)
	}
}

func TestCreateEmployee_WithUserAccount(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	body := employeePayload("Linked", "User")
	body["username"] = "linkeduser"
	body["password"] = "pw-12345"
	body["userRole"] = "staff"
	rec := doRequest(t, e, http.MethodPost, "/api/employees", asJSON(t, body), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var emp struct {
		ID     string  `json:"id"`
		UserID *string `json:"userId"`
	}
	decodeEnvelope(t, rec.Body, &emp)
	if emp.UserID == nil || *emp.UserID == "" {
		t.Errorf("expected linked userId, got %+v", emp)
	}
	if n := countTableRows(t, "users", "username = 'linkeduser'"); n != 1 {
		t.Errorf("expected 1 linked user, got %d", n)
	}
}

func TestCreateEmployee_WithUserAccountRequiresUsersWrite(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)

	if _, err := store.DB.Exec(`UPDATE roles SET scopes = 'employees:write' WHERE name = 'staff'`); err != nil {
		t.Fatal(err)
	}
	tok := generateTokenForRole(t, "staff", "employee-writer")

	body := employeePayload("No", "AccountPermission")
	body["username"] = "forbidden-linked-user"
	body["password"] = "pw-12345"
	rec := doRequest(t, e, http.MethodPost, "/api/employees", asJSON(t, body), tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "users", "username = ?", "forbidden-linked-user"); n != 0 {
		t.Errorf("expected no linked user to be created, got %d", n)
	}
	if n := countTableRows(t, "employees", "first_name = ? AND last_name = ?", "No", "AccountPermission"); n != 0 {
		t.Errorf("expected no employee to be created, got %d", n)
	}

	rec = doRequest(t, e, http.MethodPost, "/api/employees",
		asJSON(t, employeePayload("Employee", "Only")), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("employee-only create expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateEmployee_ValidationFails(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/employees",
		asJSON(t, map[string]any{"firstName": "", "employmentType": "Casual"}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "Please check your input")
}

func TestGetEmployeeByID_FoundAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createEmployee(t, e, tok, "Get", "Me")
	rec := doRequest(t, e, http.MethodGet, "/api/employees/"+id, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodGet, "/api/employees/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestGetAllEmployees_ListShape(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	for i := 0; i < 2; i++ {
		createEmployee(t, e, tok, "Emp"+string(rune('A'+i)), "Last")
	}
	rec := doRequest(t, e, http.MethodGet, "/api/employees", nil, tok)
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

func TestUpdateEmployee_PartialAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createEmployee(t, e, tok, "Up", "Date")
	rec := doRequest(t, e, http.MethodPut, "/api/employees/"+id,
		asJSON(t, map[string]any{"id": "stripped", "role": "Doctor"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID   string `json:"id"`
		Role string `json:"role"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Role != "Doctor" || got.ID != id {
		t.Errorf("got %+v", got)
	}

	rec = doRequest(t, e, http.MethodPut, "/api/employees/ghost",
		asJSON(t, map[string]any{"role": "X"}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteEmployee_SuccessAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	id := createEmployee(t, e, tok, "Doomed", "Worker")
	rec := doRequest(t, e, http.MethodDelete, "/api/employees/"+id, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d body=%s", rec.Code, rec.Body.String())
	}
	if n := countTableRows(t, "employees", "id = ?", id); n != 0 {
		t.Errorf("employee not deleted")
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/employees/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestEmployeeDropdown(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	createEmployee(t, e, tok, "Drop", "Down")
	rec := doRequest(t, e, http.MethodGet, "/api/employees/dropdown", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("dropdown: %d", rec.Code)
	}
	var items []map[string]any
	decodeEnvelope(t, rec.Body, &items)
	if len(items) < 1 {
		t.Errorf("expected at least 1 dropdown item")
	}
}

// employee schedule projection

func TestGetEmployeeSchedule_Projection(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	empID := createEmployee(t, e, tok, "Proj", "Schedule")
	// Add a split Monday: mornings and late afternoons.
	if rec := doRequest(t, e, http.MethodPut, "/api/employee-schedules/day",
		asJSON(t, map[string]any{
			"employeeId": empID, "dayOfWeek": 1, "startDate": "2026-01-01",
			"shifts": []map[string]any{
				{"startTime": "09:00", "endTime": "13:00"},
				{"startTime": "15:00", "endTime": "18:00"},
			},
		}), tok); rec.Code != http.StatusOK {
		t.Fatalf("seed schedule: %d %s", rec.Code, rec.Body.String())
	}

	rec := doRequest(t, e, http.MethodGet, "/api/employees/"+empID+"/schedule?date=2026-06-01", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("schedule: %d body=%s", rec.Code, rec.Body.String())
	}
	var data struct {
		Days      []map[string]any `json:"days"`
		Templates []map[string]any `json:"templates"`
	}
	decodeEnvelope(t, rec.Body, &data)
	if len(data.Days) == 0 {
		t.Errorf("expected projected days")
	}
	if len(data.Templates) != 2 {
		t.Errorf("expected both shifts as templates, got %d", len(data.Templates))
	}
}

func TestSaveEmployeeScheduleDay_RejectsOverlappingShifts(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	empID := createEmployee(t, e, tok, "Overlap", "Schedule")
	rec := doRequest(t, e, http.MethodPut, "/api/employee-schedules/day",
		asJSON(t, map[string]any{
			"employeeId": empID, "dayOfWeek": 1, "startDate": "2026-01-01",
			"shifts": []map[string]any{
				{"startTime": "09:00", "endTime": "14:00"},
				{"startTime": "13:00", "endTime": "18:00"},
			},
		}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for overlapping shifts, got %d %s", rec.Code, rec.Body.String())
	}
}

// holidays

func TestHolidays_CRUD(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	rec := doRequest(t, e, http.MethodPost, "/api/holidays",
		asJSON(t, map[string]any{"name": "New Year", "startDate": "2026-01-01", "endDate": "2026-01-01"}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create holiday: %d body=%s", rec.Code, rec.Body.String())
	}
	var h struct{ ID string }
	decodeEnvelope(t, rec.Body, &h)
	if h.ID == "" {
		t.Fatalf("no id")
	}

	rec = doRequest(t, e, http.MethodGet, "/api/holidays", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &page)
	if page.Total != 1 {
		t.Errorf("total = %d want 1", page.Total)
	}

	rec = doRequest(t, e, http.MethodPut, "/api/holidays/"+h.ID,
		asJSON(t, map[string]any{"name": "NYE", "startDate": "2026-01-01", "endDate": "2026-01-02"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Name string `json:"name"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Name != "NYE" {
		t.Errorf("name = %q want NYE", got.Name)
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/holidays/"+h.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d", rec.Code)
	}
	if n := countTableRows(t, "holidays", "id = ?", h.ID); n != 0 {
		t.Errorf("holiday not deleted")
	}
}

func TestHoliday_UpdateNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)
	rec := doRequest(t, e, http.MethodPut, "/api/holidays/ghost",
		asJSON(t, map[string]any{"name": "X", "startDate": "2026-01-01", "endDate": "2026-01-01"}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// schedule changes

func TestEmployeeScheduleChange_TimeoffPendingThenAccepted(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	empID := createEmployee(t, e, tok, "Vac", "Taker")

	rec := doRequest(t, e, http.MethodPost, "/api/employee-schedule-changes",
		asJSON(t, map[string]any{
			"employeeId": empID, "type": "timeoff",
			"startDate": "2026-07-01", "endDate": "2026-07-05",
			"notes": "summer",
		}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create schedule change: %d body=%s", rec.Code, rec.Body.String())
	}
	var v struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Type   string `json:"type"`
	}
	decodeEnvelope(t, rec.Body, &v)
	if v.Status != "pending" {
		t.Errorf("status = %q want pending", v.Status)
	}
	if v.Type != "timeoff" {
		t.Errorf("type = %q want timeoff", v.Type)
	}

	rec = doRequest(t, e, http.MethodPost, "/api/employee-schedule-changes/"+v.ID+"/status",
		asJSON(t, map[string]any{"status": "accepted"}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("set status: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Status string `json:"status"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if got.Status != "accepted" {
		t.Errorf("status = %q want accepted", got.Status)
	}
}

func TestEmployeeScheduleChange_OvertimeRequiresTimes(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	empID := createEmployee(t, e, tok, "OT", "NoTimes")
	rec := doRequest(t, e, http.MethodPost, "/api/employee-schedule-changes",
		asJSON(t, map[string]any{
			"employeeId": empID, "type": "overtime",
			"startDate": "2026-07-01", "endDate": "2026-07-01",
		}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for overtime without times, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEmployeeScheduleChange_InvalidStatus(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	empID := createEmployee(t, e, tok, "BadStatus", "Vac")
	rec := doRequest(t, e, http.MethodPost, "/api/employee-schedule-changes",
		asJSON(t, map[string]any{
			"employeeId": empID, "type": "timeoff",
			"startDate": "2026-07-01", "endDate": "2026-07-02",
		}), tok)
	var v struct{ ID string }
	decodeEnvelope(t, rec.Body, &v)

	rec = doRequest(t, e, http.MethodPost, "/api/employee-schedule-changes/"+v.ID+"/status",
		asJSON(t, map[string]any{"status": "maybe"}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid status, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEmployeeScheduleChange_DeleteAndStatusNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	empID := createEmployee(t, e, tok, "DelVac", "Taker")
	rec := doRequest(t, e, http.MethodPost, "/api/employee-schedule-changes",
		asJSON(t, map[string]any{
			"employeeId": empID, "type": "timeoff",
			"startDate": "2026-08-01", "endDate": "2026-08-02",
		}), tok)
	var v struct{ ID string }
	decodeEnvelope(t, rec.Body, &v)

	rec = doRequest(t, e, http.MethodDelete, "/api/employee-schedule-changes/"+v.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodPost, "/api/employee-schedule-changes/ghost/status",
		asJSON(t, map[string]any{"status": "accepted"}), tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// employee salaries

func TestEmployeeSalary_CreateListUpdateDelete(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	empID := createEmployee(t, e, tok, "Salaried", "Staff")
	curID := firstSeededCurrencyID(t)

	rec := doRequest(t, e, http.MethodPost, "/api/employees/"+empID+"/salaries",
		asJSON(t, map[string]any{
			"amount": 2000.0, "currencyId": curID,
			"effectiveDate": "2026-01-01", "notes": "base",
		}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create salary: %d body=%s", rec.Code, rec.Body.String())
	}
	var s struct {
		ID       string `json:"id"`
		IsActive bool   `json:"isActive"`
	}
	decodeEnvelope(t, rec.Body, &s)
	if s.ID == "" || !s.IsActive {
		t.Errorf("got %+v", s)
	}

	// List by employee.
	rec = doRequest(t, e, http.MethodGet, "/api/employees/"+empID+"/salaries", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var list struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &list)
	if len(list.Items) != 1 {
		t.Errorf("expected 1 salary, got %d", len(list.Items))
	}

	// Update amount.
	rec = doRequest(t, e, http.MethodPut, "/api/employee-salaries/"+s.ID,
		asJSON(t, map[string]any{"amount": 2500.0}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Amount float64 `json:"amount"`
	}
	decodeEnvelope(t, rec.Body, &got)
	if !approxEqualF(got.Amount, 2500) {
		t.Errorf("amount = %v want 2500", got.Amount)
	}

	// Delete.
	rec = doRequest(t, e, http.MethodDelete, "/api/employee-salaries/"+s.ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d", rec.Code)
	}
	if n := countTableRows(t, "employee_salaries", "id = ?", s.ID); n != 0 {
		t.Errorf("salary not deleted")
	}
}

func TestEmployeeSalary_CreateSupersedesPrevious(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	empID := createEmployee(t, e, tok, "Raise", "Staff")
	curID := firstSeededCurrencyID(t)

	for _, amt := range []float64{1000, 1500} {
		rec := doRequest(t, e, http.MethodPost, "/api/employees/"+empID+"/salaries",
			asJSON(t, map[string]any{"amount": amt, "currencyId": curID, "effectiveDate": "2026-01-01"}), tok)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create salary %v: %d %s", amt, rec.Code, rec.Body.String())
		}
	}
	// Two rows, but only the latest is active (Create deactivates prior ones).
	if n := countTableRows(t, "employee_salaries", "employee_id = ?", empID); n != 2 {
		t.Errorf("expected 2 salary rows, got %d", n)
	}
	if n := countTableRows(t, "employee_salaries", "employee_id = ? AND is_active = 1", empID); n != 1 {
		t.Errorf("expected exactly 1 active salary, got %d", n)
	}
}

func TestEmployeeSalary_ValidationAndNotFound(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	empID := createEmployee(t, e, tok, "BadSalary", "Staff")
	// Missing currencyId defaults to USD.
	rec := doRequest(t, e, http.MethodPost, "/api/employees/"+empID+"/salaries",
		asJSON(t, map[string]any{"amount": 100.0}), tok)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodDelete, "/api/employee-salaries/ghost", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// salary preparation

func TestSalaryPreparation_PrepareListAndDelete(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	empID := createEmployee(t, e, tok, "Prep", "Staff")
	curID := firstSeededCurrencyID(t)
	// Give the employee an active salary so a preparation row is produced.
	if rec := doRequest(t, e, http.MethodPost, "/api/employees/"+empID+"/salaries",
		asJSON(t, map[string]any{"amount": 800.0, "currencyId": curID, "effectiveDate": "2026-01-01"}), tok); rec.Code != http.StatusCreated {
		t.Fatalf("seed salary: %d %s", rec.Code, rec.Body.String())
	}

	rec := doRequest(t, e, http.MethodPost, "/api/employee-salaries/prepare",
		asJSON(t, map[string]any{"periodStart": "2026-03-01", "periodEnd": "2026-03-31", "notes": "march"}), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("prepare: %d body=%s", rec.Code, rec.Body.String())
	}
	var preps []struct {
		ID             string  `json:"id"`
		PreparedAmount float64 `json:"preparedAmount"`
	}
	decodeEnvelope(t, rec.Body, &preps)
	if len(preps) != 1 {
		t.Fatalf("expected 1 preparation, got %d", len(preps))
	}
	if !approxEqualF(preps[0].PreparedAmount, 800) {
		t.Errorf("preparedAmount = %v want 800", preps[0].PreparedAmount)
	}

	// List for the employee.
	rec = doRequest(t, e, http.MethodGet, "/api/employees/"+empID+"/prepared-salaries", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list prepared: %d body=%s", rec.Code, rec.Body.String())
	}
	var list struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeEnvelope(t, rec.Body, &list)
	if len(list.Items) != 1 {
		t.Errorf("expected 1 prepared salary, got %d", len(list.Items))
	}

	// Adjust via PATCH.
	rec = doRequest(t, e, http.MethodPatch, "/api/employee-salary-preparations/"+preps[0].ID,
		asJSON(t, map[string]any{"adjustment": -100.0}), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("adjust: %d body=%s", rec.Code, rec.Body.String())
	}
	var adj struct {
		PreparedAmount float64 `json:"preparedAmount"`
	}
	decodeEnvelope(t, rec.Body, &adj)
	if !approxEqualF(adj.PreparedAmount, 700) {
		t.Errorf("preparedAmount after -100 adjustment = %v want 700", adj.PreparedAmount)
	}

	// Delete (reverses the linked transaction).
	rec = doRequest(t, e, http.MethodDelete, "/api/employee-salary-preparations/"+preps[0].ID, nil, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("delete prep: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSalaryPreparation_InvalidPeriod(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	tok := adminToken(t, e)

	// end before start returns 400.
	rec := doRequest(t, e, http.MethodPost, "/api/employee-salaries/prepare",
		asJSON(t, map[string]any{"periodStart": "2026-03-31", "periodEnd": "2026-03-01"}), tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for inverted period, got %d body=%s", rec.Code, rec.Body.String())
	}
}
