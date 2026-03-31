package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupClientPaymentRoutes(api *echo.Group) {
	api.GET("/client-payments", handlers.GetClientPayments, scope("transactions:read"))
	api.POST("/client-payments", handlers.CreateClientPayment, scope("transactions:write"))
}
