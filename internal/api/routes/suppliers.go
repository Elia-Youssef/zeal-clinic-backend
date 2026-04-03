package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupSupplierRoutes(api *echo.Group) {
	api.GET("/suppliers", handlers.GetAllSuppliers, scope("inventory:read"), middleware.CacheMiddleware("suppliers"))
	api.GET("/suppliers/dropdown", handlers.GetSupplierDropdown, scope("inventory:read"), middleware.CacheMiddleware("suppliers"))
	api.GET("/suppliers/:id", handlers.GetSupplierByID, scope("inventory:read"), middleware.CacheMiddleware("suppliers"))
	api.POST("/suppliers", handlers.CreateSupplier, scope("inventory:write"), middleware.CacheMiddleware("suppliers"))
	api.PUT("/suppliers/:id", handlers.UpdateSupplier, scope("inventory:write"), middleware.CacheMiddleware("suppliers"))
	api.DELETE("/suppliers/:id", handlers.DeleteSupplier, scope("inventory:delete"), middleware.CacheMiddleware("suppliers"))
}
