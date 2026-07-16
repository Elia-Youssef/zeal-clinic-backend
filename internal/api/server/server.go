package server

import (
	"database/sql"
	"net/http"

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

func CreateServer() *echo.Echo {
	e := echo.New()
	cfg := config.Current()
	restoreAPI := cloudrestore.New(cloudrestore.Config{
		PeerURL:    cfg.PeerURL,
		Secret:     cfg.SyncSecret,
		Invalidate: mw.InvalidateCacheAll,
	})

	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "${time_rfc3339} | ${status} | ${latency_human} | ${method} ${uri}\n",
	}))
	e.Use(middleware.Recover())
	e.Use(tracking.Middleware())
	e.Use(middleware.CORS())
	e.Use(mw.UpdateGate())
	e.Use(restoreAPI.Middleware())
	e.Use(syncpkg.Middleware())
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("X-Robots-Tag", "noindex, nofollow")
			return next(c)
		}
	})

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
