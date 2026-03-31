package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupSupplierPaymentRoutes(api *echo.Group) {
	api.GET("/supplier-payments", handlers.GetSupplierPayments, scope("transactions:read"))
	api.POST("/supplier-payments", handlers.CreateSupplierPayment, scope("transactions:write"))
}
