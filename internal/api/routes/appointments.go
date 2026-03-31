package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupAppointmentRoutes(api *echo.Group) {
	api.GET("/appointments", handlers.GetAllAppointments, scope("appointments:read"))
	api.POST("/appointments", handlers.CreateAppointment, scope("appointments:write"))
	api.PUT("/appointments/:id", handlers.UpdateAppointment, scope("appointments:write"))
	api.DELETE("/appointments/:id", handlers.DeleteAppointment, scope("appointments:delete"))
}
