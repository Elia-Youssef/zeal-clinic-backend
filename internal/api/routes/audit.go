package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupAuditRoutes(api *echo.Group) {
	api.GET("/audit-log", handlers.GetAllAuditLogs, scope("audit:read"))
}
