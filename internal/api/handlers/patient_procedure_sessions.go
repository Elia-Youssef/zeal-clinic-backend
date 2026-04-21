package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func UpdatePatientProcedureSession(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdatePatientProcedureSession] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	s := store.PatientProcedureSession{ID: c.Param("sessionId")}
	if err := s.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [UpdatePatientProcedureSession] session not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "session not found"})
	} else if err != nil {
		log.Println("Error: [UpdatePatientProcedureSession] failed to update session:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update session"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: s})
}
