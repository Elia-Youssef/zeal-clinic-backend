package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupPatientAllergyRoutes(api *echo.Group) {
	api.GET("/patients/:patientId/allergies", handlers.GetPatientAllergies, scope("patient-allergies:read"))
	api.POST("/patients/:patientId/allergies", handlers.AddPatientAllergy, scope("patient-allergies:write"))
	api.DELETE("/patient-allergies/:id", handlers.RemovePatientAllergy, scope("patient-allergies:write"))
}
