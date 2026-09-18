package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"clinic-api/internal/api/middleware"
	"clinic-api/internal/api/server"
	"clinic-api/internal/browser"
	"clinic-api/internal/buildmode"
	"clinic-api/internal/cloudrestore"
	"clinic-api/internal/config"
	"clinic-api/internal/database"
	"clinic-api/internal/database/store"
	"clinic-api/internal/monitor"
	syncpkg "clinic-api/internal/sync"
	"clinic-api/internal/systray"
	"clinic-api/internal/tracking"
	"clinic-api/internal/updater"

	"github.com/labstack/echo/v4"
)

type appOptions struct {
	seedOnly   bool
	demo       bool
	noBrowser  bool
	dev        bool
	startup    bool
	postUpdate bool
}

const (
	mainDelay    = 8 * time.Second
	syncDelay    = 2 * time.Second
	monitorDelay = 4 * time.Second
	serverDelay  = 2 * time.Second
	shutdownWait = time.Second
)

func main() {
	log.Printf("Zeal Clinic %s starting...", buildmode.Version)

	opts := parseOptions()
	cfg := config.Load()

	tracking.Init(buildmode.Version, environmentName(opts))
	defer tracking.Flush(2 * time.Second)
	defer tracking.Info(nil, "[main] Server shutting down")
	defer tracking.Recover()

	tracking.Debug(nil, "[main] Server starting")

	// A release build never runs on the committed dev values.
	if err := cfg.Check(buildmode.Release()); err != nil {
		tracking.Fatal("Refusing to start", err)
	}

	if cfg.ClinicTimezone != "" {
		if err := store.SetClinicTimezone(cfg.ClinicTimezone); err != nil {
			tracking.Fatal("Invalid clinic timezone", err)
		}
	}
	if err := database.SetKey(cfg.DBEncryptionKey); err != nil {
		tracking.Fatal("Refusing to start", err)
	}

	if !buildmode.Cloud && !opts.seedOnly && !opts.postUpdate && alreadyRunning() {
		handoffToRunningInstance(opts, cfg)
		return
	}

	if opts.startup {
		time.Sleep(mainDelay)
	}

	updater.ReconcileBoot() // roll back a failed trial build before the DB opens (linux)

	db, err := database.Open("")
	if err != nil {
		tracking.Fatal("Failed to open database", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Database shutdown: %v", err)
			tracking.CaptureError(nil, err)
		}
	}()

	if opts.seedOnly {
		runSeed(db, opts)
		return
	}

	// If we just rebooted from a self-update, confirm it and reopen the API.
	updater.FinalizeOnBoot(opts.postUpdate)

	if opts.startup {
		time.Sleep(syncDelay)
	}

	services := &backgroundServices{cfg: cfg, opts: opts}
	services.Resume(db)
	defer services.Pause()
	cloudrestore.SetRuntime(services)
	defer cloudrestore.SetRuntime(nil)

	if opts.startup {
		time.Sleep(serverDelay)
	}

	go updater.ConfirmStartup() // linux: confirm a surviving trial build

	runServer(cfg, opts)
}

func environmentName(opts appOptions) string {
	switch {
	case buildmode.Cloud:
		return "cloud"
	case opts.dev:
		return "dev"
	default:
		return "local"
	}
}

func parseOptions() appOptions {
	seedOnly := flag.Bool("seed-only", false, "Seed and exit; do not start the server (used by the installer)")
	demo := flag.Bool("demo", false, "Seed the database with a large demo dataset (includes --seed)")
	noBrowser := flag.Bool("no-browser", false, "Don't auto-open the browser on startup")
	dev := flag.Bool("dev", false, "Development mode: no tray, no browser, db at ./tmp/clinic.db")
	startup := flag.Bool("startup", false, "Windows startup launch: delay heavy services and do not open the browser")
	postUpdate := flag.Bool("post-update", false, "Relaunch after a self-update: skip the single-instance handoff and the browser")
	flag.Parse()

	opts := appOptions{
		seedOnly:   *seedOnly,
		demo:       *demo,
		noBrowser:  *noBrowser,
		dev:        *dev,
		startup:    *startup,
		postUpdate: *postUpdate,
	}
	if opts.dev || opts.startup || opts.postUpdate || buildmode.Cloud {
		opts.noBrowser = true
	}
	return opts
}

func handoffToRunningInstance(opts appOptions, cfg *config.Config) {
	log.Printf("Another instance is already running")
	if !opts.noBrowser && browser.ProbeHealth() {
		log.Println("Opening browser")
		browser.Open()
	}
}

func runSeed(db *sql.DB, opts appOptions) {
	if opts.demo {
		if err := database.SeedDemo(db); err != nil {
			tracking.Fatal("Failed to seed demo data", err)
		}
	}
	log.Println("--seed-only complete; exiting")
}

func startSync(db *sql.DB, cfg *config.Config) (*syncpkg.Engine, context.CancelFunc) {
	syncpkg.InvalidateCache = middleware.InvalidateCache
	syncpkg.RecalcBalance = store.RecalculateBalanceWithTx

	ctx, cancel := context.WithCancel(context.Background())
	engine := syncpkg.New(db, syncpkg.Config{
		PeerURL: cfg.PeerURL,
		Secret:  cfg.SyncSecret,
	})
	engine.Start(ctx)
	return engine, cancel
}

func startMonitor(opts appOptions) *monitor.Monitor {
	mon := monitor.New(1 * time.Minute)
	mon.Register(
		monitor.Action{Name: "expire-discounts", Duration: 10 * time.Minute, Target: monitor.ActionLocal, Fn: monitor.ExpireDiscounts},
		monitor.Action{Name: "appointment-reminders", Duration: 1 * time.Minute, Target: monitor.ActionBoth, Fn: monitor.SendAppointmentReminders},
		monitor.Action{Name: "cleanup-pdf-cache", Duration: 5 * time.Minute, Target: monitor.ActionBoth, Fn: monitor.CleanupPDFCache},
		monitor.Action{Name: "cleanup-expired-tokens", Duration: time.Hour, Target: monitor.ActionBoth, Fn: monitor.CleanupExpiredTokens},
	)
	if buildmode.Cloud || !opts.dev {
		mon.Register(monitor.Action{Name: "backup-db", Duration: 3 * time.Hour, Target: monitor.ActionBoth, Fn: monitor.BackupDatabase})
	}
	mon.Start()
	return mon
}

// backgroundServices owns every component that can access the database outside
// an HTTP request. cloudrestore pauses it before closing the pools and resumes
// it with the newly-opened pool after commit or rollback.
type backgroundServices struct {
	mu         sync.Mutex
	cfg        *config.Config
	opts       appOptions
	engine     *syncpkg.Engine
	syncCancel context.CancelFunc
	monitor    *monitor.Monitor
}

func (s *backgroundServices) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.monitor != nil {
		s.monitor.Stop()
		s.monitor = nil
	}
	monitor.WaitAsync()
	if s.syncCancel != nil {
		s.syncCancel()
		s.syncCancel = nil
	}
	if s.engine != nil {
		s.engine.Stop()
		s.engine = nil
	}
}

func (s *backgroundServices) Resume(db *sql.DB) {
	if db == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine != nil || s.monitor != nil {
		return
	}
	s.engine, s.syncCancel = startSync(db, s.cfg)
	if s.opts.startup {
		time.Sleep(monitorDelay)
	}
	s.monitor = startMonitor(s.opts)
}

func runServer(cfg *config.Config, opts appOptions) {
	if !opts.noBrowser {
		go browser.WaitAndOpen()
	}

	e := server.CreateServerWithOptions(server.Options{Dev: opts.dev, DebugRoutes: opts.dev})

	if opts.dev || buildmode.Cloud {
		go server.Start(e, cfg)

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		<-ctx.Done()

		shutdownServer(e, shutdownWait)
		return
	}

	go func() {
		server.Start(e, cfg)
		systray.Quit()
	}()

	systray.Run(func(ctx context.Context) error {
		return shutdownServerContext(e, ctx)
	})
}

func shutdownServer(e *echo.Echo, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := shutdownServerContext(e, ctx); err != nil {
		log.Printf("HTTP server shutdown: %v", err)
	}
}

func shutdownServerContext(e *echo.Echo, ctx context.Context) error {
	err := e.Shutdown(ctx)
	if err == nil {
		return nil
	}
	// Long-lived SSE handlers can outlive the graceful window. Close their
	// sockets so the remaining component shutdown can finish immediately.
	if closeErr := e.Close(); closeErr != nil {
		return errors.Join(err, closeErr)
	}
	return nil
}
