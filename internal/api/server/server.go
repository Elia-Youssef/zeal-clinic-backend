package server

import (
	"net/http"

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

	// Protected routes (auth required)
	api := e.Group("/api")
	api.Use(mw.AuthMiddleware)
	api.Use(mw.AuditLogger())

	routes.SetupAuthRoutes(authGroup, api)

	for _, register := range protectedRouteRegistrars {
		register(api)
	}
	routes.SetupBookingRoutes(api, pub)

	// SPA (Vite build embedded from client/dist)
	e.GET("/*", spaHandler())

	return e
}

var protectedRouteRegistrars = []func(*echo.Group){
	routes.SetupServerInfoRoutes,
	routes.SetupRoleRoutes,
	routes.SetupRoomRoutes,
	routes.SetupAllergyRoutes,
	routes.SetupProcedureTypeRoutes,
	routes.SetupProcedureCategoryRoutes,
	routes.SetupProcedureRoutes,
	routes.SetupProcedureAllergyConflictRoutes,
	routes.SetupPatientRoutes,
	routes.SetupPatientAllergyRoutes,
	routes.SetupMedicineRoutes,
	routes.SetupPatientMedicineRoutes,
	routes.SetupPatientProcedureRoutes,
	routes.SetupAppointmentRoutes,
	routes.SetupScheduleAvailabilityRoutes,
	routes.SetupProductRoutes,
	routes.SetupProductCategoryRoutes,
	routes.SetupProductAllergyConflictRoutes,
	routes.SetupUserRoutes,
	routes.SetupEmployeeRoutes,
	routes.SetupEmployeeSalaryRoutes,
	routes.SetupPrescriptionRoutes,
	routes.SetupBalanceRoutes,
	routes.SetupInvoiceRoutes,
	routes.SetupClientInvoiceRoutes,
	routes.SetupClientPaymentRoutes,
	routes.SetupSupplierRoutes,
	routes.SetupSupplierInvoiceRoutes,
	routes.SetupSupplierPaymentRoutes,
	routes.SetupExpenseRoutes,
	routes.SetupExpensePaymentRoutes,
	routes.SetupEmployeePaymentRoutes,
	routes.SetupCurrencyRoutes,
	routes.SetupCountryRoutes,
	routes.SetupLebanonCityRoutes,
	routes.SetupDiscountRoutes,
	routes.SetupInvoiceItemDiscountRoutes,
	routes.SetupNotificationRoutes,
	routes.SetupEventRoutes,
	routes.SetupAuditRoutes,
	routes.SetupAnalyticsRoutes,
	routes.SetupSearchRoutes,
}

func Start(e *echo.Echo, cfg *config.Config) {
	if err := e.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
		e.Logger.Fatal(err)
	}
}
