package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

func GetAllAuditLogs(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	entries, err := (&models.AuditLogEntry{}).GetAll(
		c.QueryParam("entityType"),
		c.QueryParam("entityId"),
		c.QueryParam("action"),
		limit,
	)
	if err != nil {
		log.Println("Error: [GetAllAuditLogs] failed to fetch audit log:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch audit log"})
	}
	if entries == nil {
		entries = []models.AuditLogEntry{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: entries})
}
