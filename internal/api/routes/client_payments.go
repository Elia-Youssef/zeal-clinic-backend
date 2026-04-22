package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupClientPaymentRoutes(api *echo.Group) {
	api.GET("/patients/:id/payments", handlers.GetClientPayments, scope("transactions:read"), cache("client-payments"))
	api.POST("/client-payments", handlers.CreateClientPayment, scope("transactions:write"), cache("client-payments", "balances"))
	api.POST("/client-refunds", handlers.CreateClientRefund, scope("transactions:write"), cache("client-payments", "balances"))
}
