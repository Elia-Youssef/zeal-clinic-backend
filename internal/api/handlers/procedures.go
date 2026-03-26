package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllProcedures(c echo.Context) error {
	items, err := (&models.Procedure{}).GetAll()
	if err != nil {
		log.Println("Error: [GetAllProcedures] failed to fetch procedures:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch procedures"})
	}
	if items == nil {
		items = []models.Procedure{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetProcedureByID(c echo.Context) error {
	var proc models.Procedure
	if err := proc.GetByID(c.Param("id")); err == sql.ErrNoRows {
		log.Println("Error: [GetProcedureByID] procedure not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "procedure not found"})
	} else if err != nil {
		log.Println("Error: [GetProcedureByID] failed to fetch procedure:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch procedure"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: proc})
}

func CreateProcedure(c echo.Context) error {
	var p models.Procedure
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreateProcedure] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("name", p.Name, "Name")
	if v.HasErrors() {
		log.Println("Error: [CreateProcedure] validation failed:", v.Fields)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}
	if p.ProcedureType == "" {
		p.ProcedureType = "Clinic Procedure"
	}
	if p.CommissionType == "" {
		p.CommissionType = "percentage"
	}
	p.IsActive = true
	if err := p.Create(); err != nil {
		log.Println("Error: [CreateProcedure] failed to create procedure:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create procedure"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: p})
}

func UpdateProcedure(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateProcedure] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	proc := models.Procedure{ID: c.Param("id")}
	if err := proc.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: [UpdateProcedure] procedure not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "procedure not found"})
	} else if err != nil {
		log.Println("Error: [UpdateProcedure] failed to update procedure:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update procedure"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: proc})
}

func DeleteProcedure(c echo.Context) error {
	proc := models.Procedure{ID: c.Param("id")}
	if err := proc.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteProcedure] procedure not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "procedure not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProcedure] failed to delete procedure:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete procedure"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

// Sessions
func CreateProcedureSession(c echo.Context) error {
	var s models.ProcedureSession
	if err := c.Bind(&s); err != nil {
		log.Println("Error: [CreateProcedureSession] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	s.ProcedureID = c.Param("id")
	v := NewValidator()
	v.Required("name", s.Name, "Name")
	v.Positive("sessionNumber", float64(s.SessionNumber), "Session number")
	if v.HasErrors() {
		log.Println("Error: [CreateProcedureSession] validation failed:", v.Fields)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}
	if s.Currency == "" {
		s.Currency = "USD"
	}
	if err := s.Create(); err != nil {
		log.Println("Error: [CreateProcedureSession] failed to create session:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create session"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: s})
}

func DeleteProcedureSession(c echo.Context) error {
	s := models.ProcedureSession{ID: c.Param("sessionId")}
	if err := s.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteProcedureSession] session not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "session not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProcedureSession] failed to delete session:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete session"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
