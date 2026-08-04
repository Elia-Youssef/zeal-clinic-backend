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

// GetEmployeeSchedule returns one employee's projected week days, the schedule
// templates covering that week (superseded versions included, so every day the
// grid draws can be traced back to an editable row), overlapping schedule
// changes (any status), holidays, and monthly hour totals in one response.
func GetEmployeeSchedule(c echo.Context) error {
	employeeID := c.Param("id")
	if employeeID == "" {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Employee is required"})
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load employee schedules"})
	}
	templates, err := store.SchedulesForWeek(employeeID, weekStart, weekEnd)
	if err != nil {
		log.Println("Error: GetEmployeeSchedule templates:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load schedule templates"})
	}
	changes, err := store.EmployeeScheduleChangesOverlappingRange(employeeID, weekStart, weekEnd)
	if err != nil {
		log.Println("Error: GetEmployeeSchedule changes:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load schedule changes"})
	}
	holidays, err := store.HolidaysOverlappingRange(weekStart, weekEnd)
	if err != nil {
		log.Println("Error: GetEmployeeSchedule holidays:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load holidays"})
	}

	type employeeMonthHours struct {
		EmployeeID    string  `json:"employeeId"`
		EmployeeName  string  `json:"employeeName"`
		Hours         float64 `json:"hours"`
		OvertimeHours float64 `json:"overtimeHours"`
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
			monthHours[i].OvertimeHours += d.OvertimeHours
		}
	}
	for i := range monthHours {
		monthHours[i].Hours = math.Round(monthHours[i].Hours*100) / 100
		monthHours[i].OvertimeHours = math.Round(monthHours[i].OvertimeHours*100) / 100
	}
	if templates == nil {
		templates = []store.EmployeeSchedule{}
	}
	if changes == nil {
		changes = store.EmployeeScheduleChangeList{}
	}
	if holidays == nil {
		holidays = store.HolidayList{}
	}

	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]any{
		"weekStart":       weekStart,
		"weekEnd":         weekEnd,
		"monthStart":      monthStart,
		"monthEnd":        monthEnd,
		"days":            weekDays,
		"templates":       templates,
		"scheduleChanges": changes,
		"holidays":        holidays,
		"monthHours":      monthHours,
	}})
}

// GetEmployeeWorkingHours returns the projected working-hour totals for one
// employee across an arbitrary date range, broken into regular and overtime
// hours. Required query params: from=YYYY-MM-DD, to=YYYY-MM-DD.
func GetEmployeeWorkingHours(c echo.Context) error {
	employeeID := c.Param("id")
	if employeeID == "" {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Employee is required"})
	}
	from := store.Date(c.QueryParam("from"))
	to := store.Date(c.QueryParam("to"))
	if from.IsZero() || to.IsZero() {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Start and end dates are required"})
	}
	days, err := store.EmployeeScheduleForRange(employeeID, from, to)
	if err != nil {
		log.Println("Error: GetEmployeeWorkingHours project:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load working hours"})
	}
	var totalHours, overtimeHours float64
	for _, d := range days {
		totalHours += d.Hours
		overtimeHours += d.OvertimeHours
	}
	regularHours := math.Round((totalHours-overtimeHours)*100) / 100
	totalHours = math.Round(totalHours*100) / 100
	overtimeHours = math.Round(overtimeHours*100) / 100

	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]any{
		"employeeId":    employeeID,
		"from":          from,
		"to":            to,
		"regularHours":  regularHours,
		"overtimeHours": overtimeHours,
		"totalHours":    totalHours,
		"days":          days,
	}})
}

// ============================================================
// SCHEDULE CHANGES (write-only; reads come through GetEmployeeSchedule)
// ============================================================

func CreateEmployeeScheduleChange(c echo.Context) error {
	var v store.EmployeeScheduleChange
	if err := c.Bind(&v); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	v.Status = "pending"
	// Non-admin callers can only request changes for themselves.
	if user, ok := c.Get("user").(store.User); ok && user.Role != "super-admin" && user.Role != "admin" {
		empID, err := store.EmployeeIDForUser(user.ID)
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusForbidden, httpx.Response{Error: "No employee linked to your account"})
		}
		if err != nil {
			log.Println("Error: CreateEmployeeScheduleChange employee lookup:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load employee"})
		}
		v.EmployeeID = empID
	}
	if err := v.Create(); err != nil {
		log.Println("Error: CreateEmployeeScheduleChange:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: v})
}

func UpdateEmployeeScheduleChange(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")
	delete(updates, "employeeId")
	delete(updates, "status") // status changes go through SetEmployeeScheduleChangeStatus
	v := store.EmployeeScheduleChange{ID: c.Param("id")}
	if err := v.Update(updates); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Schedule change not found"})
	} else if err != nil {
		log.Println("Error: UpdateEmployeeScheduleChange:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: v})
}

func SetEmployeeScheduleChangeStatus(c echo.Context) error {
	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	v := store.EmployeeScheduleChange{ID: c.Param("id")}
	if err := v.SetStatus(req.Status); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Schedule change not found"})
	} else if err != nil {
		log.Println("Error: SetEmployeeScheduleChangeStatus:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: v})
}

func DeleteEmployeeScheduleChange(c echo.Context) error {
	v := store.EmployeeScheduleChange{ID: c.Param("id")}
	if err := v.Delete(); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Schedule change not found"})
	} else if err != nil {
		log.Println("Error: DeleteEmployeeScheduleChange:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete schedule change"})
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load holidays"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func CreateHoliday(c echo.Context) error {
	var h store.Holiday
	if err := c.Bind(&h); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if user, ok := c.Get("user").(store.User); ok {
		h.CreatedBy = user.DisplayName
	}
	if err := h.Create(); err != nil {
		log.Println("Error: CreateHoliday:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create holiday"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: h})
}

func UpdateHoliday(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")
	h := store.Holiday{ID: c.Param("id")}
	if err := h.Update(updates); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Holiday not found"})
	} else if err != nil {
		log.Println("Error: UpdateHoliday:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update holiday"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: h})
}

func DeleteHoliday(c echo.Context) error {
	h := store.Holiday{ID: c.Param("id")}
	if err := h.Delete(); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Holiday not found"})
	} else if err != nil {
		log.Println("Error: DeleteHoliday:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete holiday"})
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
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	createdBy := ""
	if user, ok := c.Get("user").(store.User); ok {
		createdBy = user.DisplayName
	}
	prep, err := store.PrepareEmployeeSalaries(req.PeriodStart, req.PeriodEnd, req.Notes, createdBy)
	if err != nil {
		log.Println("Error: PrepareEmployeeSalaries:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Couldn't prepare salaries for this period"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: prep})
}

func GetEmployeePreparedSalaries(c echo.Context) error {
	params := parseListParams(c)
	preps, total, err := store.PreparedSalariesForEmployee(c.Param("id"), params)
	if err != nil {
		log.Println("Error: GetEmployeePreparedSalaries:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load prepared salaries"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: preps, Total: total}})
}

func DeleteEmployeeSalaryPreparation(c echo.Context) error {
	prep := store.EmployeeSalaryPreparation{ID: c.Param("id")}
	if err := prep.Delete(); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Salary preparation not found"})
	} else if err != nil {
		log.Println("Error: DeleteEmployeeSalaryPreparation:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete salary preparation"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}

func UpdateEmployeeSalaryPreparation(c echo.Context) error {
	var req struct {
		Adjustment float64 `json:"adjustment"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	prep := store.EmployeeSalaryPreparation{ID: c.Param("id")}
	if err := prep.SetAdjustment(req.Adjustment); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Salary preparation not found"})
	} else if err != nil {
		log.Println("Error: UpdateEmployeeSalaryPreparation:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Couldn't apply this adjustment"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: prep})
}
