package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetProcedureSessionDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := models.GetProcedureSessionDropdown(c.Param("id"), params)
	if err != nil {
		log.Println("Error: [GetProcedureSessionDropdown] failed to fetch session dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch session dropdown"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateProcedureSession(c echo.Context) error {
	var s models.ProcedureSession
	if err := c.Bind(&s); err != nil {
		log.Println("Error: [CreateProcedureSession] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	s.ProcedureID = c.Param("id")
	if err := s.IsValid(); err != nil {
		log.Println("Error: [CreateProcedureSession] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}
	if err := s.Create(); err != nil {
		log.Println("Error: [CreateProcedureSession] failed to create session:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create session"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: s})
}

func DeleteProcedureSession(c echo.Context) error {
	id := c.Param("sessionId")
	if models.HasDependencies(id, map[string]string{"patient_procedure_sessions": "procedure_session_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete session: has related records"})
	}

	s := models.ProcedureSession{ID: id}
	if err := s.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteProcedureSession] session not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "session not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProcedureSession] failed to delete session:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete session"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
