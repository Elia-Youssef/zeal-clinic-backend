package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProcedureRoutes(api *echo.Group) {
	api.GET("/procedures", handlers.GetAllProcedures, scope("services:read"), cache("procedures"))
	api.GET("/procedures/dropdown", handlers.GetProcedureDropdown, scope("services:read"), cache("procedures"))
	api.GET("/procedures/:id", handlers.GetProcedureByID, scope("services:read"), cache("procedures"))
	api.GET("/procedures/:id/prices", handlers.GetProcedurePrices, scope("services:read"))
	api.GET("/procedures/:id/appointments", handlers.GetProcedureAppointments, scope("appointments:read"), cache("appointments"))
	api.POST("/procedures", handlers.CreateProcedure, scope("services:write"), cache("procedures"))
	// Discount items reference procedure names; bust discounts on edit/delete.
	api.PUT("/procedures/:id", handlers.UpdateProcedure, scope("services:write"), cache("procedures", "discounts"))
	api.DELETE("/procedures/:id", handlers.DeleteProcedure, scope("services:delete"), cache("procedures", "discounts"))
}
