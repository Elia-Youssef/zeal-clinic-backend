package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllNotifications(c echo.Context) error {
	unreadOnly := c.QueryParam("unread") == "true"

	n := models.Notification{}
	notifications, err := n.GetAll(unreadOnly)
	if err != nil {
		log.Println("Error: GetAllNotifications failed to fetch notifications")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch notifications"})
	}
	if notifications == nil {
		notifications = []models.Notification{}
	}

	count, _ := n.GetUnreadCount()

	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: map[string]interface{}{
		"notifications": notifications,
		"unreadCount":   count,
	}})
}

func MarkNotificationRead(c echo.Context) error {
	n := models.Notification{ID: c.Param("id")}
	if err := n.MarkRead(); err == sql.ErrNoRows {
		log.Println("Error: MarkNotificationRead notification not found")
		return c.JSON(http.StatusNotFound, utils.Response{Error: "notification not found"})
	} else if err != nil {
		log.Println("Error: MarkNotificationRead failed to mark read")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to mark read"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

func MarkAllNotificationsRead(c echo.Context) error {
	n := models.Notification{}
	if err := n.MarkAllRead(); err != nil {
		log.Println("Error: MarkAllNotificationsRead failed to mark all read")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to mark all read"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
