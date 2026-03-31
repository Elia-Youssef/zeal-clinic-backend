package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func UpdatePatientProcedureSession(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdatePatientProcedureSession] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	s := models.PatientProcedureSession{ID: c.Param("sessionId")}
	if err := s.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: [UpdatePatientProcedureSession] session not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "session not found"})
	} else if err != nil {
		log.Println("Error: [UpdatePatientProcedureSession] failed to update session:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update session"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: s})
}
