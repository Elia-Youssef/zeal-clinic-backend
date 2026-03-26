package server

import (
	"log"
	"net/http"
	"os"

	mw "clinic-api/internal/api/middleware"
	"clinic-api/internal/api/routes"
	"clinic-api/internal/config"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func CreateServer() *echo.Echo {
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     config.CORSOrigins,
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{echo.HeaderContentType, echo.HeaderAuthorization},
		AllowCredentials: true,
	}))

	// Static & health
	e.Static("/uploads", "./uploads")
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Public routes (no auth)
	pub := e.Group("/api/public")
	authGroup := e.Group("/api")
	routes.SetupAuthRoutes(authGroup)

	// Protected routes (auth required)
	api := e.Group("/api")
	api.Use(mw.AuthMiddleware)
	api.Use(mw.AuditLogger())

	routes.SetupRoleRoutes(api)
	routes.SetupRoomRoutes(api)
	routes.SetupAllergyRoutes(api)
	routes.SetupProcedureRoutes(api)
	routes.SetupPatientRoutes(api)
	routes.SetupAppointmentRoutes(api)
	routes.SetupScheduleAvailabilityRoutes(api)
	routes.SetupInventoryRoutes(api)
	routes.SetupTeamRoutes(api)
	routes.SetupBookingRoutes(api, pub)
	routes.SetupPrescriptionRoutes(api)
	routes.SetupConsentRoutes(api)
	routes.SetupBalanceRoutes(api)
	routes.SetupExchangeRateRoutes(api)
	routes.SetupWaitlistRoutes(api)
	routes.SetupAuditRoutes(api)
	routes.SetupReportRoutes(api)
	routes.SetupNotificationRoutes(api)

	return e
}

func Start(e *echo.Echo) {
	certFile := config.TLSCert
	keyFile := config.TLSKey

	if _, errCert := os.Stat(certFile); errCert == nil {
		if _, errKey := os.Stat(keyFile); errKey == nil {
			log.Printf("Starting Clinic API server with TLS on :%s", config.Port)
			e.Logger.Fatal(e.StartTLS(":"+config.Port, certFile, keyFile))
		}
	}

	log.Printf("Starting Clinic API server on :%s (no TLS)", config.Port)
	e.Logger.Fatal(e.Start(":" + config.Port))
}
