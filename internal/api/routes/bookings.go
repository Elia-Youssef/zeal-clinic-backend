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
	pub.POST("/bookings", handlers.CreatePublicBooking, cache("bookings"))

	// Staff booking endpoints
	api.GET("/bookings", handlers.GetAllBookings, scope("bookings:read"), cache("bookings"))
	// Confirm creates a patient (if new) and an appointment; invalidate those caches too.
	api.PUT("/bookings/:id/confirm", handlers.ConfirmBooking, scope("bookings:write"), cache("bookings", "appointments", "patients"))
	api.PUT("/bookings/:id/cancel", handlers.CancelBooking, scope("bookings:write"), cache("bookings"))
}
