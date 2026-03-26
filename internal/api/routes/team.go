package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupTeamRoutes(api *echo.Group) {
	api.GET("/team", handlers.GetAllTeamMembers, scope("team:read"))
	api.POST("/team", handlers.CreateTeamMember, scope("team:write"))
	api.PUT("/team/:id", handlers.UpdateTeamMember, scope("team:write"))
	api.DELETE("/team/:id", handlers.DeleteTeamMember, scope("team:delete"))
}
