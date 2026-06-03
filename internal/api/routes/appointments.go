package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupAppointmentRoutes(api *echo.Group) {
	api.GET("/appointments", handlers.GetAllAppointments, scope("appointments:read"), cacheF("appointments"))
	api.GET("/appointments/pdf", handlers.GetAllAppointmentsPDF, scope("appointments:read"))
	api.GET("/patients/:patientId/appointments", handlers.GetPatientAppointments, scope("appointments:read"), cache("appointments"))
	api.GET("/employees/:id/appointments", handlers.GetEmployeeAppointments, scopeOrSelf("appointments:read", selfEmployee), cache("appointments"))
	api.GET("/appointments/count-per-room", handlers.GetAppointmentCountPerRoom, scope("appointments:read"), cache("appointments"))
	api.POST("/appointments", handlers.CreateAppointment, scope("appointments:write"), cache("appointments", "analytics"))
	api.PUT("/appointments/:id", handlers.UpdateAppointment, scope("appointments:write"), cache("appointments", "analytics"))
	api.POST("/appointments/:id/reschedule", handlers.RescheduleAppointment, scope("appointments:write"), cache("appointments", "analytics"))
	api.DELETE("/appointments/:id", handlers.DeleteAppointment, scope("appointments:delete"), cache("appointments", "analytics"))
}
