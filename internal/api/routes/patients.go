package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupPatientRoutes(api *echo.Group) {
	api.GET("/patients", handlers.GetAllPatients, scope("patients:read"), cache("patients"))
	api.GET("/patients/dropdown", handlers.GetPatientDropdown, scope("patients:read"), cache("patients"))
	api.GET("/patients/:id", handlers.GetPatientByID, scope("patients:read"), cache("patients"))
	api.POST("/patients", handlers.CreatePatient, scope("patients:write"), cache("patients"))
	api.PUT("/patients/:id", handlers.UpdatePatient, scope("patients:write"), cache("patients"))
	api.DELETE("/patients/:id", handlers.DeletePatient, scope("patients:delete"), cache("patients"))
}
