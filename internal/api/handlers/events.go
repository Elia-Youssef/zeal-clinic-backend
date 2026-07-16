package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"clinic-api/internal/api/httpx"
	"clinic-api/internal/cloudrestore"
	"clinic-api/internal/realtime"
	"clinic-api/internal/sync"

	"github.com/labstack/echo/v4"
)

const heartbeatInterval = 25 * time.Second

func StreamEvents(c echo.Context) error {
	userID := currentUserID(c)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, httpx.Response{Error: "Please sign in again"})
	}

	res := c.Response()
	h := res.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	res.WriteHeader(http.StatusOK)
	res.Flush()

	client := realtime.Register(userID)
	defer client.Close()
	cloudrestore.MarkLongLivedReady(c)

	if err := writeSSE(res, realtime.Event{Type: "hello"}); err != nil {
		return nil
	}
	if err := writeSSE(res, realtime.Event{Type: "cloud_connection", Data: sync.IsCloudConnected()}); err != nil {
		return nil
	}
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	ctx := c.Request().Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-client.Events():
			if !ok {
				return nil
			}
			if err := writeSSE(res, ev); err != nil {
				return nil
			}
		case <-heartbeat.C:
			if client.TakePending() {
				if err := writeSSE(res, realtime.Event{Type: "data_changed"}); err != nil {
					return nil
				}
			}
			if _, err := res.Write([]byte(": ping\n\n")); err != nil {
				return nil
			}
			res.Flush()
		}
	}
}

func writeSSE(w *echo.Response, ev realtime.Event) error {
	data, err := json.Marshal(ev.Data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data); err != nil {
		return err
	}
	w.Flush()
	return nil
}
