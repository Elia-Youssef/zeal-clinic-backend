package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupPatientProcedureRoutes(api *echo.Group) {
	api.GET("/patients/:patientId/procedures", handlers.GetPatientProceduresByPatient, scope("patients:read"))
	api.GET("/patient-procedures/:id", handlers.GetPatientProcedureByID, scope("patients:read"))
	// Appointment GETs hydrate patient_procedure data; invalidate them on write.
	api.POST("/patient-procedures", handlers.CreatePatientProcedure, scope("patients:write"), cache("appointments", "analytics"))
	api.PUT("/patient-procedures/:id", handlers.UpdatePatientProcedure, scope("patients:write"), cache("appointments", "analytics"))
	api.DELETE("/patient-procedures/:id", handlers.DeletePatientProcedure, scope("patients:delete"), cache("appointments", "analytics"))
}
