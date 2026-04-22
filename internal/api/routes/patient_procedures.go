package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupPatientProcedureRoutes(api *echo.Group) {
	api.GET("/patients/:patientId/procedures", handlers.GetPatientProceduresByPatient, scope("patients:read"))
	api.GET("/patient-procedures/:id", handlers.GetPatientProcedureByID, scope("patients:read"))
	// Appointment GETs hydrate patient_procedure + session data; invalidate them on write.
	api.POST("/patient-procedures", handlers.CreatePatientProcedure, scope("patients:write"), cache("appointments"))
	api.PUT("/patient-procedures/:id", handlers.UpdatePatientProcedure, scope("patients:write"), cache("appointments"))
	api.DELETE("/patient-procedures/:id", handlers.DeletePatientProcedure, scope("patients:delete"), cache("appointments"))
	api.PUT("/patient-procedure-sessions/:sessionId", handlers.UpdatePatientProcedureSession, scope("patients:write"), cache("appointments"))
}
