package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupMedicineRoutes(api *echo.Group) {
	api.GET("/medicines", handlers.GetAllMedicines, scope("patients:read"), middleware.CacheMiddleware("medicines"))
	api.GET("/medicines/dropdown", handlers.GetMedicineDropdown, scope("patients:read"), middleware.CacheMiddleware("medicines"))
	api.POST("/medicines", handlers.CreateMedicine, scope("patients:write"), middleware.CacheMiddleware("medicines"))
	api.PUT("/medicines/:id", handlers.UpdateMedicine, scope("patients:write"), middleware.CacheMiddleware("medicines"))
	api.DELETE("/medicines/:id", handlers.DeleteMedicine, scope("patients:delete"), middleware.CacheMiddleware("medicines"))
}
