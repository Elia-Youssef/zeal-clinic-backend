package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupPrescriptionRoutes(api *echo.Group) {
	api.GET("/patients/:patientId/prescriptions", handlers.GetPrescriptionsByPatient, scope("prescriptions:read"))
	api.POST("/prescriptions", handlers.CreatePrescription, scope("prescriptions:write"))
	api.PUT("/prescriptions/:id", handlers.UpdatePrescription, scope("prescriptions:write"))
	api.DELETE("/prescriptions/:id", handlers.DeletePrescription, scope("prescriptions:delete"))
}
