package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupBookingRoutes(api *echo.Group, pub *echo.Group) {
	// Public booking endpoints (no auth)
	pub.GET("/services", handlers.GetPublicServices)
	pub.GET("/availability", handlers.GetBookingAvailability)
	pub.GET("/patient-check", handlers.CheckBookingPatient)
	pub.POST("/bookings", handlers.CreatePublicBooking)

	// Staff booking endpoints
	api.GET("/bookings", handlers.GetAllBookings, scope("bookings:read"))
	api.PUT("/bookings/:id/confirm", handlers.ConfirmBooking, scope("bookings:write"))
	api.PUT("/bookings/:id/cancel", handlers.CancelBooking, scope("bookings:write"))
}
