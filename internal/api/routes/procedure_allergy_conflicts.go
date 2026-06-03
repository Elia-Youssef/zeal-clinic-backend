package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProcedureAllergyConflictRoutes(api *echo.Group) {
	api.GET("/procedures/:procedureId/allergy-conflicts", handlers.GetProcedureAllergyConflicts, scope("procedure-allergy-conflicts:read"))
	api.POST("/procedures/:procedureId/allergy-conflicts", handlers.AddProcedureAllergyConflict, scope("procedure-allergy-conflicts:write"))
	api.PUT("/procedure-allergy-conflicts/:id", handlers.UpdateProcedureAllergyConflictNotes, scope("procedure-allergy-conflicts:write"))
	api.DELETE("/procedure-allergy-conflicts/:id", handlers.RemoveProcedureAllergyConflict, scope("procedure-allergy-conflicts:write"))
}
