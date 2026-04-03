package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupProcedureTypeRoutes(api *echo.Group) {
	api.GET("/procedure-types", handlers.GetAllProcedureTypes, scope("services:read"), middleware.CacheMiddleware("procedure-types"))
	api.GET("/procedure-types/dropdown", handlers.GetProcedureTypeDropdown, scope("services:read"), middleware.CacheMiddleware("procedure-types"))
	api.POST("/procedure-types", handlers.CreateProcedureType, scope("services:write"), middleware.CacheMiddleware("procedure-types"))
	api.PUT("/procedure-types/:id", handlers.UpdateProcedureType, scope("services:write"), middleware.CacheMiddleware("procedure-types"))
	api.DELETE("/procedure-types/:id", handlers.DeleteProcedureType, scope("services:write"), middleware.CacheMiddleware("procedure-types"))
}
