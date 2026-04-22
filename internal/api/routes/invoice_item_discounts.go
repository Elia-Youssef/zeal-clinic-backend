package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupInvoiceItemDiscountRoutes(api *echo.Group) {
	api.GET("/invoice-items/:itemId/discounts", handlers.GetInvoiceItemDiscounts, scope("transactions:read"), cache("invoice-item-discounts"))
	// Applying/removing a discount on an invoice line also mutates discount usage counts
	// and is visible on invoice detail views.
	api.POST("/invoice-items/:itemId/discounts", handlers.ApplyInvoiceItemDiscount, scope("transactions:write"),
		cache("invoice-item-discounts", "discounts", "invoices", "client-invoices", "supplier-invoices"))
	api.DELETE("/invoice-items/:itemId/discounts/:discountId", handlers.RemoveInvoiceItemDiscount, scope("transactions:delete"),
		cache("invoice-item-discounts", "discounts", "invoices", "client-invoices", "supplier-invoices"))
}
