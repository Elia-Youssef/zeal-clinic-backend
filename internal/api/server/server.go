package server

import (
	"database/sql"
	"net/http"
	"os"

	mw "clinic-api/internal/api/middleware"
	"clinic-api/internal/api/routes"
	"clinic-api/internal/buildmode"
	"clinic-api/internal/cloudrestore"
	"clinic-api/internal/config"
	"clinic-api/internal/database/store"
	"clinic-api/internal/pdf"
	syncpkg "clinic-api/internal/sync"
	"clinic-api/internal/tracking"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Options are the few things a development run changes about the server.
type Options struct {
	// DevCORS allows cross-origin calls from the dashboard's Vite dev
	// server. The built binary serves the dashboard itself, so it needs no
	// CORS.
	DevCORS bool
	// DebugRoutes registers the routes that only serve development runs and
	// the automated tests (the test notification). A --dev run or the tests
	// set it; an ordinary built run never does.
	DebugRoutes bool
}

func CreateServerWithOptions(opts Options) *echo.Echo {
	e := echo.New()
	e.IPExtractor = clientIPExtractor()
	e.HTTPErrorHandler = apiErrorHandler(e)
	e.Server.ReadHeaderTimeout = readHeaderTimeout
	e.Server.IdleTimeout = idleTimeout
	cfg := config.Current()
	restoreAPI := cloudrestore.New(cloudrestore.Config{
		PeerURL:    cfg.PeerURL,
		Secret:     cfg.SyncSecret,
		Invalidate: mw.InvalidateCacheAll,
	})

	e.Use(requestLogger(os.Stdout))
	e.Use(middleware.Recover())
	e.Use(tracking.Middleware())
	e.Use(securityHeaders())
	if opts.DevCORS {
		e.Use(devCORS())
	}
	e.Use(bodyLimitMiddleware())
	e.Use(mw.UpdateGate())
	e.Use(restoreAPI.Middleware())
	e.Use(syncpkg.Middleware())

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok", "version": buildmode.Version})
	})

	e.GET("/robots.txt", func(c echo.Context) error {
		return c.String(http.StatusOK, "User-agent: *\nDisallow: /\n")
	})

	authGroup := e.Group("/api")
	api := e.Group("/api")
	api.Use(mw.AuthMiddleware())
	api.Use(mw.AuditLogger())

	routes.SetupAuthRoutes(authGroup, api)
	if !buildmode.Cloud {
		api.POST("/cloud-restore", restoreAPI.HandleLocal, mw.RequireScope("cloud-restore:write"))
	}

	sse := e.Group("/api")
	sse.Use(mw.AuthMiddleware())
	sse.Use(mw.AuditLogger())
	routes.SetupEventRoutes(sse)

	for _, register := range protectedRouteRegistrars {
		register(api)
	}
	if opts.DebugRoutes {
		routes.SetupDebugRoutes(api)
	}

	syncAPI := &syncpkg.API{
		DB:     store.DB,
		DBFunc: func() *sql.DB { return store.DB },
		Secret: cfg.SyncSecret,
	}
	syncAPI.RegisterRoutes(e)
	if buildmode.Cloud && cfg.SyncSecret != "" {
		e.POST("/api/cloud-restore", restoreAPI.HandleCloud)
	}

	routes.SetupUpdatePublicRoutes(e, cfg)

	pdfs := e.Group("/files")
	pdfs.Use(mw.AuthMiddleware())
	pdfs.Static("/", pdf.TmpDir())

	e.GET("/*", spaHandler())

	return e
}

var protectedRouteRegistrars = []func(*echo.Group){
	routes.SetupServerInfoRoutes,
	routes.SetupUpdateRoutes,
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
	routes.SetupEmployeeScheduleRoutes,
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
