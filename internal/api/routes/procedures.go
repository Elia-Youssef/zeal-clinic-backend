package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProcedureRoutes(api *echo.Group) {
	api.GET("/procedures", handlers.GetAllProcedures, scope("procedures:read"), cache("procedures"))
	api.GET("/procedures/dropdown", handlers.GetProcedureDropdown, scope("procedures:read"), cache("procedures"))
	api.GET("/procedures/:id", handlers.GetProcedureByID, scope("procedures:read"), cache("procedures"))
	api.GET("/procedures/:id/prices", handlers.GetProcedurePrices, scope("procedures:read"))
	api.GET("/procedures/:id/appointments", handlers.GetProcedureAppointments, scope("appointments:read"), cache("appointments"))
	api.POST("/procedures", handlers.CreateProcedure, scope("procedures:write"), cache("procedures"))
	// Discount items reference procedure names; bust discounts on edit/delete.
	api.PUT("/procedures/:id", handlers.UpdateProcedure, scope("procedures:write"), cache("procedures", "discounts"))
	api.DELETE("/procedures/:id", handlers.DeleteProcedure, scope("procedures:delete"), cache("procedures", "discounts"))
}
