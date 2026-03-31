package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProcedureRoutes(api *echo.Group) {
	api.GET("/procedures", handlers.GetAllProcedures, scope("services:read"))
	api.GET("/procedures/:id", handlers.GetProcedureByID, scope("services:read"))
	api.POST("/procedures", handlers.CreateProcedure, scope("services:write"))
	api.PUT("/procedures/:id", handlers.UpdateProcedure, scope("services:write"))
	api.DELETE("/procedures/:id", handlers.DeleteProcedure, scope("services:write"))
}
