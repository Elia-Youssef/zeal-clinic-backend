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
	api.PUT("/appointments/:id/reminder", handlers.MarkAppointmentReminded, scope("appointments:write"))
	api.PUT("/appointments/:id/approve", handlers.ApproveAppointment, scope("appointments:write"))
	api.PUT("/appointments/:id/reject", handlers.RejectAppointment, scope("appointments:write"))

	// Appointment photos
	api.GET("/appointments/:id/photos", handlers.GetAppointmentPhotos, scope("appointments:read"))
	api.POST("/appointments/:id/photos", handlers.UploadAppointmentPhoto, scope("appointments:write"))
	api.DELETE("/appointments/:appointmentId/photos/:photoId", handlers.DeleteAppointmentPhoto, scope("appointments:write"))
}
