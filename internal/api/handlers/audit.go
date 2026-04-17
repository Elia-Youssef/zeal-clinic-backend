package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllAuditLogs(c echo.Context) error {
	params := parseListParams(c)
	if params.Limit == 0 {
		params.Limit = 200
	}
	entries := models.AuditLogEntryList{}
	total, err := entries.GetAll(
		c.QueryParam("entityType"),
		c.QueryParam("entityId"),
		c.QueryParam("action"),
		params,
	)
	if err != nil {
		log.Println("Error: [GetAllAuditLogs] failed to fetch audit log:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch audit log"})
	}
	if entries == nil {
		entries = []models.AuditLogEntry{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: entries, Total: total}})
}
