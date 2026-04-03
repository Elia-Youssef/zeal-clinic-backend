package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupClientPaymentRoutes(api *echo.Group) {
	api.GET("/patients/:id/payments", handlers.GetClientPayments, scope("transactions:read"), middleware.CacheMiddleware("client-payments"))
	api.POST("/client-payments", handlers.CreateClientPayment, scope("transactions:write"), middleware.CacheMiddleware("client-payments"))
}
