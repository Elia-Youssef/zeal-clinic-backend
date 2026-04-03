package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupProcedureRoutes(api *echo.Group) {
	api.GET("/procedures", handlers.GetAllProcedures, scope("services:read"), middleware.CacheMiddleware("procedures"))
	api.GET("/procedures/dropdown", handlers.GetProcedureDropdown, scope("services:read"), middleware.CacheMiddleware("procedures"))
	api.GET("/procedures/:id", handlers.GetProcedureByID, scope("services:read"), middleware.CacheMiddleware("procedures"))
	api.POST("/procedures", handlers.CreateProcedure, scope("services:write"), middleware.CacheMiddleware("procedures"))
	api.PUT("/procedures/:id", handlers.UpdateProcedure, scope("services:write"), middleware.CacheMiddleware("procedures"))
	api.DELETE("/procedures/:id", handlers.DeleteProcedure, scope("services:write"), middleware.CacheMiddleware("procedures"))
}
