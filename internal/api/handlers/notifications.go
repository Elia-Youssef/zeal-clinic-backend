package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func currentUserID(c echo.Context) string {
	u, ok := c.Get("user").(models.User)
	if !ok {
		return ""
	}
	return u.ID
}

func GetAllNotifications(c echo.Context) error {
	userID := currentUserID(c)
	params := parseListParams(c)
	notifications := models.NotificationList{}
	total, err := notifications.GetAll(userID, params)
	if err != nil {
		log.Println("Error: [GetAllNotifications] failed to fetch notifications:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch notifications"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: notifications, Total: total}})
}

func GetUnreadNotificationCount(c echo.Context) error {
	userID := currentUserID(c)
	count, err := models.GetUnreadCount(userID)
	if err != nil {
		log.Println("Error: [GetUnreadNotificationCount] failed to get count:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to get unread count"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: count})
}

func CreateNotification(c echo.Context) error {
	var n models.Notification
	if err := c.Bind(&n); err != nil {
		log.Println("Error: [CreateNotification] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := n.IsValid(); err != nil {
		log.Println("Error: [CreateNotification] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	if err := n.Create(); err != nil {
		log.Println("Error: [CreateNotification] failed to create notification:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create notification"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: n})
}

func MarkNotificationRead(c echo.Context) error {
	id := c.Param("id")
	userID := currentUserID(c)

	ownerID, err := models.GetUserIDForNotification(id)
	if err != nil {
		return c.JSON(http.StatusNotFound, utils.Response{Error: "notification not found"})
	}
	if ownerID != userID {
		return c.JSON(http.StatusForbidden, utils.Response{Error: "not your notification"})
	}

	if err := models.MarkNotificationRead(id); err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, utils.Response{Error: "notification not found"})
	} else if err != nil {
		log.Println("Error: [MarkNotificationRead] failed:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to mark notification as read"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

func MarkAllNotificationsRead(c echo.Context) error {
	userID := currentUserID(c)
	if err := models.MarkAllNotificationsRead(userID); err != nil {
		log.Println("Error: [MarkAllNotificationsRead] failed:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to mark notifications as read"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

func DeleteNotification(c echo.Context) error {
	id := c.Param("id")
	userID := currentUserID(c)

	ownerID, err := models.GetUserIDForNotification(id)
	if err != nil {
		return c.JSON(http.StatusNotFound, utils.Response{Error: "notification not found"})
	}
	if ownerID != userID {
		return c.JSON(http.StatusForbidden, utils.Response{Error: "not your notification"})
	}

	n := models.Notification{ID: id}
	if err := n.Delete(); err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, utils.Response{Error: "notification not found"})
	} else if err != nil {
		log.Println("Error: [DeleteNotification] failed:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete notification"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

func DeleteAllNotifications(c echo.Context) error {
	userID := currentUserID(c)
	_, err := models.DeleteAllNotifications(userID)
	if err != nil {
		log.Println("Error: [DeleteAllNotifications] failed:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete notifications"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
