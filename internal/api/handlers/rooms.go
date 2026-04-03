package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllRooms(c echo.Context) error {
	params := parseListParams(c)
	rooms, total, err := (&models.Room{}).GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllRooms] failed to fetch rooms:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch rooms"})
	}
	if rooms == nil {
		rooms = []models.Room{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: rooms, Total: total}})
}

func GetRoomDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := models.GetRoomDropdown(params)
	if err != nil {
		log.Println("Error: [GetRoomDropdown] failed to fetch room dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch room dropdown"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateRoom(c echo.Context) error {
	var r models.Room
	if err := c.Bind(&r); err != nil {
		log.Println("Error: [CreateRoom] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := r.IsValid(); err != nil {
		log.Println("Error: [CreateRoom] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}

	if err := r.Create(); err != nil {
		log.Println("Error: [CreateRoom] failed to create room:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create room"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: r})
}

func UpdateRoom(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateRoom] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")

	r := models.Room{ID: c.Param("id")}
	if err := r.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: [UpdateRoom] room not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "room not found"})
	} else if err != nil {
		log.Println("Error: [UpdateRoom] failed to update room:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update room"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: r})
}

func DeleteRoom(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"appointments": "room_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete room: has related records"})
	}

	r := models.Room{ID: id}
	if err := r.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteRoom] room not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "room not found"})
	} else if err != nil {
		log.Println("Error: [DeleteRoom] failed to delete room:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete room"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
