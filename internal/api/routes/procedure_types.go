package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProcedureTypeRoutes(api *echo.Group) {
	api.GET("/procedure-types", handlers.GetAllProcedureTypes, scope("services:read"), cache("procedure-types"))
	api.GET("/procedure-types/dropdown", handlers.GetProcedureTypeDropdown, scope("services:read"), cache("procedure-types"))
	api.POST("/procedure-types", handlers.CreateProcedureType, scope("services:write"), cache("procedure-types"))
	api.PUT("/procedure-types/:id", handlers.UpdateProcedureType, scope("services:write"), cache("procedure-types"))
	api.DELETE("/procedure-types/:id", handlers.DeleteProcedureType, scope("services:write"), cache("procedure-types"))
}
