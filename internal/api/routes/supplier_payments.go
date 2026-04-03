package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupSupplierPaymentRoutes(api *echo.Group) {
	api.GET("/suppliers/:id/payments", handlers.GetSupplierPayments, scope("transactions:read"), middleware.CacheMiddleware("supplier-payments"))
	api.POST("/supplier-payments", handlers.CreateSupplierPayment, scope("transactions:write"), middleware.CacheMiddleware("supplier-payments"))
}
