package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupPatientRoutes(api *echo.Group) {
	api.GET("/patients", handlers.GetAllPatients, scope("patients:read"))
	api.GET("/patients/:id", handlers.GetPatientByID, scope("patients:read"))
	api.POST("/patients", handlers.CreatePatient, scope("patients:write"))
	api.PUT("/patients/:id", handlers.UpdatePatient, scope("patients:write"))
	api.DELETE("/patients/:id", handlers.DeletePatient, scope("patients:delete"))

	// Patient allergies
	api.GET("/patients/:patientId/allergies", handlers.GetPatientAllergies, scope("patients:read"))
	api.POST("/patients/:patientId/allergies", handlers.AddPatientAllergy, scope("patients:write"))
	api.DELETE("/patient-allergies/:id", handlers.RemovePatientAllergy, scope("patients:write"))

	// Patient procedures
	api.GET("/patients/:patientId/procedures", handlers.GetPatientProceduresByPatient, scope("patients:read"))
	api.GET("/patient-procedures/:id", handlers.GetPatientProcedureByID, scope("patients:read"))
	api.POST("/patient-procedures", handlers.CreatePatientProcedure, scope("patients:write"))
	api.PUT("/patient-procedures/:id", handlers.UpdatePatientProcedure, scope("patients:write"))
	api.DELETE("/patient-procedures/:id", handlers.DeletePatientProcedure, scope("patients:delete"))
	api.PUT("/patient-procedure-sessions/:sessionId", handlers.UpdatePatientProcedureSession, scope("patients:write"))
}
