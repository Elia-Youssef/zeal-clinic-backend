package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupProcedureCategoryRoutes(api *echo.Group) {
	api.GET("/procedure-categories", handlers.GetAllProcedureCategories, scope("services:read"), middleware.CacheMiddleware("procedure-categories"))
	api.GET("/procedure-categories/dropdown", handlers.GetProcedureCategoryDropdown, scope("services:read"), middleware.CacheMiddleware("procedure-categories"))
	api.POST("/procedure-categories", handlers.CreateProcedureCategory, scope("services:write"), middleware.CacheMiddleware("procedure-categories"))
	api.PUT("/procedure-categories/:id", handlers.UpdateProcedureCategory, scope("services:write"), middleware.CacheMiddleware("procedure-categories"))
	api.DELETE("/procedure-categories/:id", handlers.DeleteProcedureCategory, scope("services:write"), middleware.CacheMiddleware("procedure-categories"))
}
