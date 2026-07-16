package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProcedureCategoryRoutes(api *echo.Group) {
	api.GET("/procedure-categories", handlers.GetAllProcedureCategories, scope("procedure-categories:read"), cache("procedure-categories"))
	api.GET("/procedure-categories/dropdown", handlers.GetProcedureCategoryDropdown, scope("procedure-categories:read"), cache("procedure-categories"))
	api.POST("/procedure-categories", handlers.CreateProcedureCategory, scope("procedure-categories:write"), cache("procedure-categories", "reports"))
	api.PUT("/procedure-categories/:id", handlers.UpdateProcedureCategory, scope("procedure-categories:write"), cache("procedure-categories", "procedures", "appointments", "invoices", "analytics", "reports"))
	api.DELETE("/procedure-categories/:id", handlers.DeleteProcedureCategory, scope("procedure-categories:write"), cache("procedure-categories", "procedures", "appointments", "invoices", "analytics", "reports"))
}
