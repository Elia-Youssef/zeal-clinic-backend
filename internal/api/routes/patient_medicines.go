package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupPatientMedicineRoutes(api *echo.Group) {
	api.GET("/patients/:patientId/medicines", handlers.GetPatientMedicines, scope("patients:read"))
	api.POST("/patients/:patientId/medicines", handlers.AddPatientMedicine, scope("patients:write"))
	api.DELETE("/patient-medicines/:id", handlers.RemovePatientMedicine, scope("patients:write"))
}
