package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupSupplierRoutes(api *echo.Group) {
	api.GET("/suppliers", handlers.GetAllSuppliers, scope("inventory:read"), cache("suppliers"))
	api.GET("/suppliers/dropdown", handlers.GetSupplierDropdown, scope("inventory:read"), cache("suppliers"))
	api.GET("/suppliers/:id", handlers.GetSupplierByID, scope("inventory:read"), cache("suppliers"))
	api.POST("/suppliers", handlers.CreateSupplier, scope("inventory:write"), cache("suppliers"))
	api.PUT("/suppliers/:id", handlers.UpdateSupplier, scope("inventory:write"), cache("suppliers"))
	api.DELETE("/suppliers/:id", handlers.DeleteSupplier, scope("inventory:delete"), cache("suppliers"))
}
