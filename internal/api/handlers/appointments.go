package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
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
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "date query parameter is required"})
	}
	params := parseListParams(c)
	apts := store.AppointmentList{}
	total, err := apts.GetAll(date, params)
	if err != nil {
		log.Println("Error: GetAllAppointments failed to fetch appointments:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch appointments"})
	}
	holidays, err := store.HolidaysOverlappingRange(store.Date(date), store.Date(date))
	if err != nil {
		log.Println("Error: GetAllAppointments failed to fetch holidays:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch holidays"})
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
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "date query parameter is required"})
	}
	t, err := time.Parse(store.DateFormat, date)
	if err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid date format"})
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch appointment counts"})
	}
	holidays, err := store.HolidaysOverlappingRange(weekStart, weekEnd)
	if err != nil {
		log.Println("Error: [GetAppointmentCountPerRoom] failed to fetch holidays:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch holidays"})
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch appointments"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: apts, Total: total}})
}

func CreateAppointment(c echo.Context) error {
	var a store.Appointment
	if err := c.Bind(&a); err != nil {
		log.Println("Error: CreateAppointment invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if a.Status == "" {
		a.Status = "Scheduled"
	}
	if err := a.IsValid(); err != nil {
		log.Println("Error: CreateAppointment validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	if err := a.Create(); err != nil {
		log.Println("Error: CreateAppointment failed to create appointment:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create appointment"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: a})
}

func UpdateAppointment(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateAppointment invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")

	apt := store.Appointment{ID: c.Param("id")}
	if err := apt.Update(updates); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: UpdateAppointment appointment not found:", err)
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "appointment not found"})
		}
		if strings.Contains(err.Error(), "room conflict") {
			log.Println("Error: UpdateAppointment room conflict:", err)
			return c.JSON(http.StatusConflict, httpx.Response{Error: err.Error()})
		}
		log.Println("Error: UpdateAppointment failed to update appointment:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update appointment"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: apt})
}

func DeleteAppointment(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"bookings": "appointment_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "cannot delete appointment: has related records"})
	}

	apt := store.Appointment{ID: id}
	if err := apt.Delete(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [DeleteAppointment] appointment not found:", err)
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "appointment not found"})
		}
		log.Println("Error: [DeleteAppointment] failed to delete appointment:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete appointment"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
