package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupAppointmentPhotoRoutes(api *echo.Group) {
	api.GET("/appointments/:id/photos", handlers.GetAppointmentPhotos, scope("appointments:read"))
	api.POST("/appointments/:id/photos", handlers.UploadAppointmentPhoto, scope("appointments:write"))
	api.DELETE("/appointments/:appointmentId/photos/:photoId", handlers.DeleteAppointmentPhoto, scope("appointments:write"))
}
