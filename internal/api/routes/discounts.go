package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupDiscountRoutes(api *echo.Group) {
	api.GET("/discounts", handlers.GetAllDiscounts, scope("discounts:read"), cache("discounts"))
	api.GET("/discounts/:id", handlers.GetDiscountByID, scope("discounts:read"), cache("discounts"))
	api.POST("/discounts", handlers.CreateDiscount, scope("discounts:write"), cache("discounts", "analytics"))
	api.PUT("/discounts/:id", handlers.UpdateDiscount, scope("discounts:write"), cache("discounts", "invoices", "analytics"))
	api.DELETE("/discounts/:id", handlers.DeleteDiscount, scope("discounts:delete"), cache("discounts", "invoices", "analytics"))
	// Standalone gift-card redemption: credits a patient's balance.
	api.POST("/gift-cards/redeem", handlers.RedeemGiftCode, criticalSync, scope("discounts:write"),
		cache("discounts", "balances", "client-payments", "analytics"))
}
