package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllAuditLogs(c echo.Context) error {
	params := parseListParams(c)
	if params.Limit == 0 {
		params.Limit = 200
	}
	entries := store.AuditLogEntryList{}
	total, err := entries.GetAll(
		c.QueryParam("entityType"),
		c.QueryParam("entityId"),
		c.QueryParam("action"),
		c.QueryParam("userId"),
		params,
	)
	if err != nil {
		log.Println("Error: [GetAllAuditLogs] failed to fetch audit log:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load audit log"})
	}
	if entries == nil {
		entries = []store.AuditLogEntry{}
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: entries, Total: total}})
}
