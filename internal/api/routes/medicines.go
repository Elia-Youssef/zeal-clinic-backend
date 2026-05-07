package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupMedicineRoutes(api *echo.Group) {
	api.GET("/medicines", handlers.GetAllMedicines, scope("medicines:read"), cache("medicines"))
	api.GET("/medicines/dropdown", handlers.GetMedicineDropdown, scope("medicines:read"), cache("medicines"))
	api.POST("/medicines", handlers.CreateMedicine, scope("medicines:write"), cache("medicines"))
	api.PUT("/medicines/:id", handlers.UpdateMedicine, scope("medicines:write"), cache("medicines"))
	api.DELETE("/medicines/:id", handlers.DeleteMedicine, scope("medicines:delete"), cache("medicines"))
}
