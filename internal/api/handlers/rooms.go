package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllRooms(c echo.Context) error {
	params := parseListParams(c)
	rooms := store.RoomList{}
	total, err := rooms.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllRooms] failed to fetch rooms:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load rooms"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: rooms, Total: total}})
}

func GetRoomDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetRoomDropdown(params)
	if err != nil {
		log.Println("Error: [GetRoomDropdown] failed to fetch room dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateRoom(c echo.Context) error {
	var r store.Room
	if err := c.Bind(&r); err != nil {
		log.Println("Error: [CreateRoom] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := r.IsValid(); err != nil {
		log.Println("Error: [CreateRoom] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	if err := r.Create(); err != nil {
		log.Println("Error: [CreateRoom] failed to create room:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create room"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: r})
}

func UpdateRoom(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateRoom] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")

	r := store.Room{ID: c.Param("id")}
	if err := r.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [UpdateRoom] room not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Room not found"})
	} else if err != nil {
		log.Println("Error: [UpdateRoom] failed to update room:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update room"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: r})
}

func DeleteRoom(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"appointments": "room_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete room while it's in use"})
	}

	r := store.Room{ID: id}
	if err := r.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeleteRoom] room not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Room not found"})
	} else if err != nil {
		log.Println("Error: [DeleteRoom] failed to delete room:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete room"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
