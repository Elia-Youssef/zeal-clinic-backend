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
}
