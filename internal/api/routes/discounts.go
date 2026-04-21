package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupDiscountRoutes(api *echo.Group) {
	api.GET("/discounts", handlers.GetAllDiscounts, scope("services:read"), cache("discounts"))
	api.GET("/discounts/:id", handlers.GetDiscountByID, scope("services:read"), cache("discounts"))
	api.GET("/items/:itemId/discounts", handlers.GetItemDiscounts, scope("services:read"), cache("discounts"))
	api.POST("/discounts", handlers.CreateDiscount, scope("services:write"), cache("discounts"))
	api.PUT("/discounts/:id", handlers.UpdateDiscount, scope("services:write"), cache("discounts"))
	api.DELETE("/discounts/:id", handlers.DeleteDiscount, scope("services:delete"), cache("discounts"))
	// Discount items
	api.POST("/discounts/:id/items", handlers.AddDiscountItem, scope("services:write"), cache("discounts"))
	api.DELETE("/discounts/:id/items/:itemId", handlers.RemoveDiscountItem, scope("services:delete"), cache("discounts"))
	// Vouchers
	api.GET("/vouchers", handlers.GetAllVouchers, scope("services:read"), cache("discounts"))
	api.GET("/items/:itemId/vouchers", handlers.GetItemVouchers, scope("services:read"), cache("discounts"))
	api.GET("/discounts/:id/vouchers", handlers.GetVouchersByDiscount, scope("services:read"), cache("discounts"))
	api.GET("/vouchers/:code", handlers.GetVoucherByCode, scope("services:read"), cache("discounts"))
	api.POST("/discounts/:id/vouchers", handlers.CreateVoucher, scope("services:write"), cache("discounts"))
	api.PUT("/discounts/:id/vouchers/:voucherId/use", handlers.MarkVoucherUsed, scope("services:write"), cache("discounts"))
	api.DELETE("/discounts/:id/vouchers/:voucherId", handlers.DeleteVoucher, scope("services:delete"), cache("discounts"))
}
