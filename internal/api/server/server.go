package server

import (
	"net/http"

	mw "clinic-api/internal/api/middleware"
	"clinic-api/internal/api/routes"
	"clinic-api/internal/config"
	"clinic-api/internal/database/store"
	"clinic-api/internal/pdf"
	syncpkg "clinic-api/internal/sync"

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
	// Pokes the sync engine after any write request. Debounced: bursts of
	// writes coalesce into one fan-out ~1.5s later.
	e.Use(syncpkg.Middleware())

	// Health
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Public routes (no auth)
	authGroup := e.Group("/api")

	// Protected routes (auth required)
	api := e.Group("/api")
	api.Use(mw.AuthMiddleware)
	api.Use(mw.AuditLogger())

	routes.SetupAuthRoutes(authGroup, api)

	for _, register := range protectedRouteRegistrars {
		register(api)
	}

	// Sync (machine-to-machine, gated by SYNC_SECRET)
	// Mounted regardless of mode so both peers can serve push/pull when
	// asked. The middleware enforces the shared secret; if SYNC_SECRET is
	// unset, RegisterRoutes is a no-op.
	cfg := config.Current()
	syncAPI := &syncpkg.API{
		DB:     store.DB,
		Secret: cfg.SyncSecret,
	}
	syncAPI.RegisterRoutes(e)

	// Generated PDFs (served from local tmp dir)
	e.Static("/files", pdf.TmpDir())

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
	routes.SetupAppointmentRoutes,
	routes.SetupScheduleAvailabilityRoutes,
	routes.SetupHRRoutes,
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
	routes.SetupNotificationRoutes,
	routes.SetupEventRoutes,
	routes.SetupAuditRoutes,
	routes.SetupAnalyticsRoutes,
	routes.SetupReportsRoutes,
	routes.SetupSearchRoutes,
}

func Start(e *echo.Echo, cfg *config.Config) {
	if err := e.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
		e.Logger.Fatal(err)
	}
}
