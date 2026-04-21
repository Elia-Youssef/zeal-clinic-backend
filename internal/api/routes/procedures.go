package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProcedureRoutes(api *echo.Group) {
	api.GET("/procedures", handlers.GetAllProcedures, scope("services:read"), cache("procedures"))
	api.GET("/procedures/dropdown", handlers.GetProcedureDropdown, scope("services:read"), cache("procedures"))
	api.GET("/procedures/:id", handlers.GetProcedureByID, scope("services:read"), cache("procedures"))
	api.POST("/procedures", handlers.CreateProcedure, scope("services:write"), cache("procedures"))
	api.PUT("/procedures/:id", handlers.UpdateProcedure, scope("services:write"), cache("procedures"))
	api.DELETE("/procedures/:id", handlers.DeleteProcedure, scope("services:delete"), cache("procedures"))
}
