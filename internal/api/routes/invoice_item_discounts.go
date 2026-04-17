package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupInvoiceItemDiscountRoutes(api *echo.Group) {
	api.GET("/invoice-items/:itemId/discounts", handlers.GetInvoiceItemDiscounts, scope("transactions:read"), middleware.CacheMiddleware("invoice-item-discounts"))
	api.POST("/invoice-items/:itemId/discounts", handlers.ApplyInvoiceItemDiscount, scope("transactions:write"), middleware.CacheMiddleware("invoice-item-discounts"))
	api.DELETE("/invoice-items/:itemId/discounts/:discountId", handlers.RemoveInvoiceItemDiscount, scope("transactions:delete"), middleware.CacheMiddleware("invoice-item-discounts"))
}
