package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupPatientMedicineRoutes(api *echo.Group) {
	api.GET("/patients/:patientId/medicines", handlers.GetPatientMedicines, scope("patient-medicines:read"))
	api.POST("/patients/:patientId/medicines", handlers.AddPatientMedicine, scope("patient-medicines:write"))
	api.PUT("/patient-medicines/:id", handlers.UpdatePatientMedicineNotes, scope("patient-medicines:write"))
	api.DELETE("/patient-medicines/:id", handlers.RemovePatientMedicine, scope("patient-medicines:write"))
}
