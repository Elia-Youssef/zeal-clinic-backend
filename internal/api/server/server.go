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
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "${time_rfc3339} | ${status} | ${latency_human} | ${method} ${uri}\n",
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Health
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
	routes.SetupProcedureTypeRoutes(api)
	routes.SetupProcedureCategoryRoutes(api)
	routes.SetupProcedureRoutes(api)
	routes.SetupProcedureSessionRoutes(api)
	routes.SetupProcedureAllergyConflictRoutes(api)
	routes.SetupPatientRoutes(api)
	routes.SetupPatientAllergyRoutes(api)
	routes.SetupMedicineRoutes(api)
	routes.SetupPatientMedicineRoutes(api)
	routes.SetupPatientProcedureRoutes(api)
	routes.SetupAppointmentRoutes(api)
routes.SetupScheduleAvailabilityRoutes(api)
	routes.SetupProductRoutes(api)
	routes.SetupProductCategoryRoutes(api)
	routes.SetupProductAllergyConflictRoutes(api)
	routes.SetupUserRoutes(api)
	routes.SetupEmployeeRoutes(api)
	routes.SetupEmployeeSalaryRoutes(api)
	routes.SetupBookingRoutes(api, pub)
	routes.SetupPrescriptionRoutes(api)
	routes.SetupBalanceRoutes(api)
	routes.SetupInvoiceRoutes(api)
	routes.SetupClientInvoiceRoutes(api)
	routes.SetupClientPaymentRoutes(api)
	routes.SetupSupplierRoutes(api)
	routes.SetupSupplierInvoiceRoutes(api)
	routes.SetupSupplierPaymentRoutes(api)
	routes.SetupEmployeePaymentRoutes(api)
	routes.SetupCurrencyRoutes(api)
	routes.SetupCountryRoutes(api)
	routes.SetupLebanonCityRoutes(api)
	routes.SetupBalanceAdjustmentRoutes(api)
	routes.SetupDiscountRoutes(api)
	routes.SetupInvoiceItemDiscountRoutes(api)
	routes.SetupNotificationRoutes(api)
	routes.SetupAuditRoutes(api)
	routes.SetupReportRoutes(api)
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
