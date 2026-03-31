package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProcedureSessionRoutes(api *echo.Group) {
	api.POST("/procedures/:id/sessions", handlers.CreateProcedureSession, scope("services:write"))
	api.DELETE("/procedures/:id/sessions/:sessionId", handlers.DeleteProcedureSession, scope("services:write"))
}
