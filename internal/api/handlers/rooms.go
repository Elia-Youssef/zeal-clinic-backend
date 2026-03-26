package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllRooms(c echo.Context) error {
	rooms, err := (&models.Room{}).GetAll()
	if err != nil {
		log.Println("Error: [GetAllRooms] failed to fetch rooms:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch rooms"})
	}
	if rooms == nil {
		rooms = []models.Room{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: rooms})
}
