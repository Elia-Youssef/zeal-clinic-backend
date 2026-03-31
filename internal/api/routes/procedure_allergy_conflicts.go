package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProcedureAllergyConflictRoutes(api *echo.Group) {
	api.GET("/procedures/:procedureId/allergy-conflicts", handlers.GetProcedureAllergyConflicts, scope("services:read"))
	api.POST("/procedures/:procedureId/allergy-conflicts", handlers.AddProcedureAllergyConflict, scope("services:write"))
	api.DELETE("/procedure-allergy-conflicts/:id", handlers.RemoveProcedureAllergyConflict, scope("services:write"))
}
