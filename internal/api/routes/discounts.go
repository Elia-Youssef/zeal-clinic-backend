package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupDiscountRoutes(api *echo.Group) {
	api.GET("/discounts", handlers.GetAllDiscounts, scope("services:read"), middleware.CacheMiddleware("discounts"))
	api.GET("/discounts/:id", handlers.GetDiscountByID, scope("services:read"), middleware.CacheMiddleware("discounts"))
	api.GET("/items/:itemId/discounts", handlers.GetItemDiscounts, scope("services:read"), middleware.CacheMiddleware("discounts"))
	api.POST("/discounts", handlers.CreateDiscount, scope("services:write"), middleware.CacheMiddleware("discounts"))
	api.PUT("/discounts/:id", handlers.UpdateDiscount, scope("services:write"), middleware.CacheMiddleware("discounts"))
	api.DELETE("/discounts/:id", handlers.DeleteDiscount, scope("services:delete"), middleware.CacheMiddleware("discounts"))
	// Discount items
	api.POST("/discounts/:id/items", handlers.AddDiscountItem, scope("services:write"), middleware.CacheMiddleware("discounts"))
	api.DELETE("/discounts/:id/items/:itemId", handlers.RemoveDiscountItem, scope("services:delete"), middleware.CacheMiddleware("discounts"))
	// Vouchers
	api.GET("/vouchers", handlers.GetAllVouchers, scope("services:read"), middleware.CacheMiddleware("discounts"))
	api.GET("/items/:itemId/vouchers", handlers.GetItemVouchers, scope("services:read"), middleware.CacheMiddleware("discounts"))
	api.GET("/discounts/:id/vouchers", handlers.GetVouchersByDiscount, scope("services:read"), middleware.CacheMiddleware("discounts"))
	api.GET("/vouchers/:code", handlers.GetVoucherByCode, scope("services:read"), middleware.CacheMiddleware("discounts"))
	api.POST("/discounts/:id/vouchers", handlers.CreateVoucher, scope("services:write"), middleware.CacheMiddleware("discounts"))
	api.PUT("/discounts/:id/vouchers/:voucherId/use", handlers.MarkVoucherUsed, scope("services:write"), middleware.CacheMiddleware("discounts"))
	api.DELETE("/discounts/:id/vouchers/:voucherId", handlers.DeleteVoucher, scope("services:delete"), middleware.CacheMiddleware("discounts"))
}
