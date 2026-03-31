package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupPatientAllergyRoutes(api *echo.Group) {
	api.GET("/patients/:patientId/allergies", handlers.GetPatientAllergies, scope("patients:read"))
	api.POST("/patients/:patientId/allergies", handlers.AddPatientAllergy, scope("patients:write"))
	api.DELETE("/patient-allergies/:id", handlers.RemovePatientAllergy, scope("patients:write"))
}
