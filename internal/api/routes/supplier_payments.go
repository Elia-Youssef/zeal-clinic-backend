package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupSupplierPaymentRoutes(api *echo.Group) {
	api.GET("/suppliers/:id/payments", handlers.GetSupplierPayments, scope("transactions:read"), cache("supplier-payments"))
	api.POST("/supplier-payments", handlers.CreateSupplierPayment, scope("transactions:write"), cache("supplier-payments", "balances"))
}
