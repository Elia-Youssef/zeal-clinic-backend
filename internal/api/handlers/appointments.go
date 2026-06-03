package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"clinic-api/internal/tracking"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

func GetAllAppointments(c echo.Context) error {
	date := c.QueryParam("date")
	if date == "" {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Date is required"})
	}
	params := parseListParams(c)
	apts := store.AppointmentList{}
	total, err := apts.GetAll(date, params)
	if err != nil {
		log.Println("Error: GetAllAppointments failed to fetch appointments:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load appointments"})
	}
	holidays, err := store.HolidaysOverlappingRange(store.Date(date), store.Date(date))
	if err != nil {
		log.Println("Error: GetAllAppointments failed to fetch holidays:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load holidays"})
	}
	if holidays == nil {
		holidays = store.HolidayList{}
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]any{
		"items":    apts,
		"total":    total,
		"holidays": holidays,
	}})
}

func GetAppointmentCountPerRoom(c echo.Context) error {
	date := c.QueryParam("date")
	if date == "" {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Date is required"})
	}
	t, err := time.Parse(store.DateFormat, date)
	if err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid date"})
	}
	offset := int(t.Weekday() - time.Monday)
	if offset < 0 {
		offset = 6
	}
	weekStart := store.Date(t.AddDate(0, 0, -offset).Format(store.DateFormat))
	weekEnd := store.Date(t.AddDate(0, 0, -offset+6).Format(store.DateFormat))

	items, err := store.GetAppointmentCountPerRoom(weekStart, weekEnd)
	if err != nil {
		log.Println("Error: [GetAppointmentCountPerRoom] failed to fetch counts:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load appointment counts"})
	}
	holidays, err := store.HolidaysOverlappingRange(weekStart, weekEnd)
	if err != nil {
		log.Println("Error: [GetAppointmentCountPerRoom] failed to fetch holidays:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load holidays"})
	}
	if items == nil {
		items = []store.RoomDayCount{}
	}
	if holidays == nil {
		holidays = store.HolidayList{}
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]any{
		"rooms":    items,
		"holidays": holidays,
	}})
}

func GetPatientAppointments(c echo.Context) error {
	patientID := c.Param("patientId")
	params := parseListParams(c)
	apts := store.AppointmentList{}
	total, err := apts.GetByPatientID(patientID, params)
	if err != nil {
		log.Println("Error: GetPatientAppointments failed to fetch appointments:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load appointments"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: apts, Total: total}})
}

func GetEmployeeAppointments(c echo.Context) error {
	employeeID := c.Param("id")
	date := c.QueryParam("date")
	params := parseListParams(c)
	apts := store.AppointmentList{}
	total, err := apts.GetByEmployeeWeek(employeeID, date, params)
	if err != nil {
		log.Println("Error: GetEmployeeAppointments failed to fetch appointments:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load appointments"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: apts, Total: total}})
}

func CreateAppointment(c echo.Context) error {
	var a store.Appointment
	if err := c.Bind(&a); err != nil {
		log.Println("Error: CreateAppointment invalid request:", err)
		tracking.Warn(c, "[CreateAppointment] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if a.Status == "" {
		a.Status = "Scheduled"
	}
	if err := a.IsValid(); err != nil {
		log.Println("Error: CreateAppointment validation failed:", err)
		tracking.Warn(c, "[CreateAppointment] validation failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	if err := a.Create(); err != nil {
		log.Println("Error: CreateAppointment failed to create appointment:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create appointment"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: a})
}

func UpdateAppointment(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateAppointment invalid request:", err)
		tracking.Warn(c, "[UpdateAppointment] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")

	apt := store.Appointment{ID: c.Param("id")}
	if err := apt.Update(updates); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: UpdateAppointment appointment not found:", err)
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Appointment not found"})
		}
		if strings.Contains(err.Error(), "room conflict") {
			log.Println("Error: UpdateAppointment room conflict:", err)
			tracking.Warn(c, "[UpdateAppointment] room conflict: "+err.Error())
			return c.JSON(http.StatusConflict, httpx.Response{Error: "This room is already booked for that time"})
		}
		log.Println("Error: UpdateAppointment failed to update appointment:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update appointment"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: apt})
}

func RescheduleAppointment(c echo.Context) error {
	var overrides map[string]any
	if err := c.Bind(&overrides); err != nil {
		log.Println("Error: RescheduleAppointment invalid request:", err)
		tracking.Warn(c, "[RescheduleAppointment] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(overrides, "id")
	delete(overrides, "rescheduledFrom")

	apt := store.Appointment{ID: c.Param("id")}
	if err := apt.Reschedule(overrides); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: RescheduleAppointment appointment not found:", err)
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Appointment not found"})
		}
		if strings.Contains(err.Error(), "room conflict") {
			log.Println("Error: RescheduleAppointment room conflict:", err)
			tracking.Warn(c, "[RescheduleAppointment] room conflict: "+err.Error())
			return c.JSON(http.StatusConflict, httpx.Response{Error: "This room is already booked for that time"})
		}
		if strings.Contains(err.Error(), "cannot reschedule") {
			log.Println("Error: RescheduleAppointment invalid state:", err)
			tracking.Warn(c, "[RescheduleAppointment] invalid state: "+err.Error())
			return c.JSON(http.StatusBadRequest, httpx.Response{Error: "This appointment can no longer be rescheduled"})
		}
		log.Println("Error: RescheduleAppointment failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't reschedule appointment"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: apt})
}

func DeleteAppointment(c echo.Context) error {
	id := c.Param("id")
	apt := store.Appointment{ID: id}
	if err := apt.Delete(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [DeleteAppointment] appointment not found:", err)
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Appointment not found"})
		}
		log.Println("Error: [DeleteAppointment] failed to delete appointment:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete appointment"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
