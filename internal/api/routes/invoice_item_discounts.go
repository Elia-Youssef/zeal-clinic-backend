package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupInvoiceItemDiscountRoutes(api *echo.Group) {
	api.GET("/invoice-items/:itemId/discounts", handlers.GetInvoiceItemDiscounts, scope("transactions:read"), cache("invoice-item-discounts"))
	api.POST("/invoice-items/:itemId/discounts", handlers.ApplyInvoiceItemDiscount, scope("transactions:write"), cache("invoice-item-discounts"))
	api.DELETE("/invoice-items/:itemId/discounts/:discountId", handlers.RemoveInvoiceItemDiscount, scope("transactions:delete"), cache("invoice-item-discounts"))
}
