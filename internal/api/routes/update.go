package routes

import (
	"log"

	"clinic-api/internal/api/handlers"
	mw "clinic-api/internal/api/middleware"
	"clinic-api/internal/buildmode"
	"clinic-api/internal/config"

	"github.com/labstack/echo/v4"
)

// SetupUpdateRoutes mounts the JWT-protected self-update endpoints.
func SetupUpdateRoutes(api *echo.Group) {
	api.GET("/update/status", handlers.GetUpdateStatus, scope("update:read"))
	api.POST("/update/start", handlers.StartUpdate, scope("update:write"))
}

// SetupUpdatePublicRoutes mounts the cloud-only, key-authed publish and
// peer-update endpoints (outside JWT auth).
func SetupUpdatePublicRoutes(e *echo.Echo, cfg *config.Config) {
	if !buildmode.Cloud {
		return
	}
	if cfg.PublishSecret != "" {
		e.POST("/api/versions", handlers.PublishVersion, mw.RequirePublishSecret(cfg.PublishSecret))
		log.Printf("[update] publish API mounted at POST /api/versions")
	}
	if cfg.SyncSecret != "" {
		e.POST("/api/update/peer", handlers.PeerStartUpdate, mw.RequireSyncSecret(cfg.SyncSecret))
		log.Printf("[update] peer-update trigger mounted at POST /api/update/peer")
	}
}
