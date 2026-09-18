package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"clinic-api/internal/realtime"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

func currentUserID(c echo.Context) string {
	u, ok := c.Get("user").(store.User)
	if !ok {
		return ""
	}
	return u.ID
}

func GetAllNotifications(c echo.Context) error {
	userID := currentUserID(c)
	params := parseListParams(c)
	notifications := store.NotificationList{}
	total, err := notifications.GetAll(userID, params)
	if err != nil {
		log.Println("Error: [GetAllNotifications] failed to fetch notifications:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load notifications"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: notifications, Total: total}})
}

func GetUnreadNotificationCount(c echo.Context) error {
	userID := currentUserID(c)
	count, err := store.GetUnreadCount(userID)
	if err != nil {
		log.Println("Error: [GetUnreadNotificationCount] failed to get count:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load unread count"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: count})
}

func MarkNotificationRead(c echo.Context) error {
	id := c.Param("id")
	userID := currentUserID(c)

	ownerID, err := store.GetUserIDForNotification(id)
	if err != nil {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Notification not found"})
	}
	if ownerID != userID {
		return c.JSON(http.StatusForbidden, httpx.Response{Error: "This notification isn't yours"})
	}

	if err := store.MarkNotificationRead(id); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Notification not found"})
	} else if err != nil {
		log.Println("Error: [MarkNotificationRead] failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't mark as read"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}

func MarkAllNotificationsRead(c echo.Context) error {
	userID := currentUserID(c)
	if err := store.MarkAllNotificationsRead(userID); err != nil {
		log.Println("Error: [MarkAllNotificationsRead] failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't mark all as read"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}

// SendTestNotification creates a notification for the current user and pushes
// it over SSE. Intended for manually verifying the realtime pipeline from the
// frontend; registered only in development runs (routes.SetupDebugRoutes).
func SendTestNotification(c echo.Context) error {
	userID := currentUserID(c)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, httpx.Response{Error: "Please sign in again"})
	}

	n := store.Notification{
		UserID:      userID,
		Title:       "Test notification",
		Description: "Sent at " + time.Now().UTC().Format("15:04:05"),
		Action:      "test",
	}
	if err := n.Create(); err != nil {
		log.Println("Error: [SendTestNotification] failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't send test notification"})
	}

	realtime.SendTo(userID, realtime.Event{Type: "notification", Data: n})
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: n})
}

func DeleteNotification(c echo.Context) error {
	id := c.Param("id")
	userID := currentUserID(c)

	ownerID, err := store.GetUserIDForNotification(id)
	if err != nil {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Notification not found"})
	}
	if ownerID != userID {
		return c.JSON(http.StatusForbidden, httpx.Response{Error: "This notification isn't yours"})
	}

	n := store.Notification{ID: id}
	if err := n.Delete(); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Notification not found"})
	} else if err != nil {
		log.Println("Error: [DeleteNotification] failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete notification"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
