package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProcedureRoutes(api *echo.Group) {
	api.GET("/procedures", handlers.GetAllProcedures, scope("services:read"))
	api.GET("/procedures/:id", handlers.GetProcedureByID, scope("services:read"))
	api.POST("/procedures", handlers.CreateProcedure, scope("services:write"))
	api.PUT("/procedures/:id", handlers.UpdateProcedure, scope("services:write"))
	api.DELETE("/procedures/:id", handlers.DeleteProcedure, scope("services:write"))
	api.POST("/procedures/:id/sessions", handlers.CreateProcedureSession, scope("services:write"))
	api.DELETE("/procedures/:id/sessions/:sessionId", handlers.DeleteProcedureSession, scope("services:write"))

	// Procedure allergy conflicts
	api.GET("/procedures/:procedureId/allergy-conflicts", handlers.GetProcedureAllergyConflicts, scope("services:read"))
	api.POST("/procedures/:procedureId/allergy-conflicts", handlers.AddProcedureAllergyConflict, scope("services:write"))
	api.DELETE("/procedure-allergy-conflicts/:id", handlers.RemoveProcedureAllergyConflict, scope("services:write"))
}
