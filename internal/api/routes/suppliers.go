package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupSupplierRoutes(api *echo.Group) {
	api.GET("/suppliers", handlers.GetAllSuppliers, scope("suppliers:read"), cache("suppliers"))
	api.GET("/suppliers/dropdown", handlers.GetSupplierDropdown, scope("suppliers:read"), cache("suppliers"))
	api.GET("/suppliers/:id", handlers.GetSupplierByID, scope("suppliers:read"), cache("suppliers"))
	api.POST("/suppliers", handlers.CreateSupplier, scope("suppliers:write"), cache("suppliers"))
	api.PUT("/suppliers/:id", handlers.UpdateSupplier, scope("suppliers:write"), cache("suppliers"))
	api.DELETE("/suppliers/:id", handlers.DeleteSupplier, scope("suppliers:delete"), cache("suppliers"))
}
