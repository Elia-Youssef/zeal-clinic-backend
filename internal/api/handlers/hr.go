package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"math"
	"net/http"

	"github.com/labstack/echo/v4"
)

// ============================================================
// EMPLOYEE SCHEDULES (read-only projection)
// ============================================================

// GetEmployeeSchedule is the single unified weekly schedule endpoint for one
// employee. It returns the projected days, the active templates, the
// overlapping vacations (any status, so the UI can show pending requests),
// the overlapping holidays, and the monthly hour total, all in one shot.
func GetEmployeeSchedule(c echo.Context) error {
	employeeID := c.Param("id")
	if employeeID == "" {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "employee id is required"})
	}
	dateParam := store.Date(c.QueryParam("date"))
	weekStart, weekEnd := store.WeekRange(dateParam)
	monthStart, monthEnd := store.MonthRange(dateParam)

	// Project over the union of the week and the month so we can compute both
	// the per-day shifts (week) and the monthly hour totals from a single pass.
	rangeStart, rangeEnd := monthStart, monthEnd
	if weekStart.Before(rangeStart) {
		rangeStart = weekStart
	}
	if weekEnd.After(rangeEnd) {
		rangeEnd = weekEnd
	}
	days, err := store.EmployeeScheduleForRange(employeeID, rangeStart, rangeEnd)
	if err != nil {
		log.Println("Error: GetEmployeeSchedule project:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch employee schedules"})
	}
	templates, err := store.ActiveSchedulesForWeek(employeeID, weekStart, weekEnd)
	if err != nil {
		log.Println("Error: GetEmployeeSchedule templates:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch schedule templates"})
	}
	vacations, err := store.VacationsOverlappingWeek(employeeID, weekStart, weekEnd)
	if err != nil {
		log.Println("Error: GetEmployeeSchedule vacations:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch vacations"})
	}
	holidays, err := store.HolidaysOverlappingWeek(weekStart, weekEnd)
	if err != nil {
		log.Println("Error: GetEmployeeSchedule holidays:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch holidays"})
	}

	type employeeMonthHours struct {
		EmployeeID   string  `json:"employeeId"`
		EmployeeName string  `json:"employeeName"`
		Hours        float64 `json:"hours"`
	}
	weekDays := make([]store.EmployeeScheduleDay, 0, len(days))
	monthIdx := map[string]int{}
	monthHours := []employeeMonthHours{}
	for _, d := range days {
		if !d.WorkDate.Before(weekStart) && !d.WorkDate.After(weekEnd) {
			weekDays = append(weekDays, d)
		}
		if !d.WorkDate.Before(monthStart) && !d.WorkDate.After(monthEnd) {
			i, ok := monthIdx[d.EmployeeID]
			if !ok {
				monthIdx[d.EmployeeID] = len(monthHours)
				monthHours = append(monthHours, employeeMonthHours{EmployeeID: d.EmployeeID, EmployeeName: d.EmployeeName})
				i = len(monthHours) - 1
			}
			monthHours[i].Hours += d.Hours
		}
	}
	for i := range monthHours {
		monthHours[i].Hours = math.Round(monthHours[i].Hours*100) / 100
	}
	if templates == nil {
		templates = []store.ScheduleAvailability{}
	}
	if vacations == nil {
		vacations = store.EmployeeVacationList{}
	}
	if holidays == nil {
		holidays = store.HolidayList{}
	}

	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]any{
		"weekStart":  weekStart,
		"weekEnd":    weekEnd,
		"monthStart": monthStart,
		"monthEnd":   monthEnd,
		"days":       weekDays,
		"templates":  templates,
		"vacations":  vacations,
		"holidays":   holidays,
		"monthHours": monthHours,
	}})
}

// ============================================================
// EMPLOYEE VACATIONS (write-only; reads come through GetEmployeeSchedule)
// ============================================================

func CreateEmployeeVacation(c echo.Context) error {
	var v store.EmployeeVacation
	if err := c.Bind(&v); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	// New requests always start as pending; only admins can pre-accept.
	v.Status = "pending"
	// Non-admin callers can only request vacations for themselves.
	if user, ok := c.Get("user").(store.User); ok && user.Role == "user" {
		empID, err := store.EmployeeIDForUser(user.ID)
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusForbidden, httpx.Response{Error: "no employee record linked to this user"})
		}
		if err != nil {
			log.Println("Error: CreateEmployeeVacation employee lookup:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to resolve employee"})
		}
		v.EmployeeID = empID
	}
	if err := v.Create(); err != nil {
		log.Println("Error: CreateEmployeeVacation:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: err.Error()})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: v})
}

func UpdateEmployeeVacation(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "employeeId")
	delete(updates, "status") // status changes go through SetEmployeeVacationStatus
	v := store.EmployeeVacation{ID: c.Param("id")}
	if err := v.Update(updates); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "employee vacation not found"})
	} else if err != nil {
		log.Println("Error: UpdateEmployeeVacation:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: v})
}

func SetEmployeeVacationStatus(c echo.Context) error {
	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	v := store.EmployeeVacation{ID: c.Param("id")}
	if err := v.SetStatus(req.Status); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "employee vacation not found"})
	} else if err != nil {
		log.Println("Error: SetEmployeeVacationStatus:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: v})
}

func DeleteEmployeeVacation(c echo.Context) error {
	v := store.EmployeeVacation{ID: c.Param("id")}
	if err := v.Delete(); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "employee vacation not found"})
	} else if err != nil {
		log.Println("Error: DeleteEmployeeVacation:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete employee vacation"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}

// ============================================================
// HOLIDAYS
// ============================================================

func GetAllHolidays(c echo.Context) error {
	params := parseListParams(c)
	items := store.HolidayList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: GetAllHolidays:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch holidays"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func CreateHoliday(c echo.Context) error {
	var h store.Holiday
	if err := c.Bind(&h); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if user, ok := c.Get("user").(store.User); ok {
		h.CreatedBy = user.DisplayName
	}
	if err := h.Create(); err != nil {
		log.Println("Error: CreateHoliday:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create holiday"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: h})
}

func UpdateHoliday(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	h := store.Holiday{ID: c.Param("id")}
	if err := h.Update(updates); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "holiday not found"})
	} else if err != nil {
		log.Println("Error: UpdateHoliday:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update holiday"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: h})
}

func DeleteHoliday(c echo.Context) error {
	h := store.Holiday{ID: c.Param("id")}
	if err := h.Delete(); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "holiday not found"})
	} else if err != nil {
		log.Println("Error: DeleteHoliday:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete holiday"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}

// ============================================================
// SALARY PREPARATION
// ============================================================

func PrepareEmployeeSalaries(c echo.Context) error {
	var req struct {
		PeriodStart store.Date `json:"periodStart"`
		PeriodEnd   store.Date `json:"periodEnd"`
		Notes       string     `json:"notes"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	createdBy := ""
	if user, ok := c.Get("user").(store.User); ok {
		createdBy = user.DisplayName
	}
	prep, err := store.PrepareEmployeeSalaries(req.PeriodStart, req.PeriodEnd, req.Notes, createdBy)
	if err != nil {
		log.Println("Error: PrepareEmployeeSalaries:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: err.Error()})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: prep})
}

func GetEmployeePreparedSalaries(c echo.Context) error {
	preps, err := store.PreparedSalariesForEmployee(c.Param("id"))
	if err != nil {
		log.Println("Error: GetEmployeePreparedSalaries:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch prepared salaries"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: preps})
}

func DeleteEmployeeSalaryPreparation(c echo.Context) error {
	prep := store.EmployeeSalaryPreparation{ID: c.Param("id")}
	if err := prep.Delete(); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "salary preparation not found"})
	} else if err != nil {
		log.Println("Error: DeleteEmployeeSalaryPreparation:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete salary preparation"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}

func UpdateEmployeeSalaryPreparation(c echo.Context) error {
	var req struct {
		Adjustment float64 `json:"adjustment"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	prep := store.EmployeeSalaryPreparation{ID: c.Param("id")}
	if err := prep.SetAdjustment(req.Adjustment); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "salary preparation not found"})
	} else if err != nil {
		log.Println("Error: UpdateEmployeeSalaryPreparation:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: prep})
}
