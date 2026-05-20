package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"time"

	"clinic-api/internal/api/middleware"
	"clinic-api/internal/api/server"
	"clinic-api/internal/browser"
	"clinic-api/internal/buildmode"
	"clinic-api/internal/config"
	"clinic-api/internal/database"
	"clinic-api/internal/monitor"
	syncpkg "clinic-api/internal/sync"
	"clinic-api/internal/systray"
	"clinic-api/internal/tracking"

	"github.com/getsentry/sentry-go"
)

type appOptions struct {
	seedOnly  bool
	demo      bool
	noBrowser bool
	dev       bool
	startup   bool
}

const (
	mainDelay    = 8 * time.Second
	syncDelay    = 2 * time.Second
	monitorDelay = 4 * time.Second
	serverDelay  = 2 * time.Second
)

func main() {
	log.Printf("Zeal Clinic %s starting...", buildmode.Version)

	opts := parseOptions()
	cfg := config.Load()

	tracking.Init(cfg.SentryDSN, buildmode.Version, environmentName(opts))
	defer tracking.Flush(2 * time.Second)
	defer tracking.Info(nil, "[main] Server shutting down")
	defer tracking.Recover()

	tracking.CaptureMessage(nil, sentry.LevelDebug, "[main] Server starting")

	if !buildmode.Cloud && !opts.seedOnly && alreadyRunning() {
		handoffToRunningInstance(opts, cfg)
		return
	}

	if opts.startup {
		time.Sleep(mainDelay)
	}

	db, err := database.Open("")
	if err != nil {
		tracking.Fatal("Failed to open database", err)
	}
	defer db.Close()

	if opts.seedOnly {
		runSeed(db, opts)
		return
	}

	if opts.startup {
		time.Sleep(syncDelay)
	}

	engine, syncCancel := startSync(db, cfg)
	defer syncCancel()
	defer engine.Stop()

	if opts.startup {
		time.Sleep(monitorDelay)
	}

	mon := startMonitor()
	defer mon.Stop()

	if opts.startup {
		time.Sleep(serverDelay)
	}

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
	flag.Parse()

	opts := appOptions{
		seedOnly:  *seedOnly,
		demo:      *demo,
		noBrowser: *noBrowser,
		dev:       *dev,
		startup:   *startup,
	}
	if opts.dev || opts.startup || buildmode.Cloud {
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

	ctx, cancel := context.WithCancel(context.Background())
	engine := syncpkg.New(db, syncpkg.Config{
		PeerURL: cfg.PeerURL,
		Secret:  cfg.SyncSecret,
	})
	engine.Start(ctx)
	return engine, cancel
}

func startMonitor() *monitor.Monitor {
	mon := monitor.New(1 * time.Minute)
	mon.Register(
		monitor.Action{Name: "expire-discounts", Fn: monitor.ExpireDiscounts},
		monitor.Action{Name: "expire-prescription-medicines", Fn: monitor.ExpirePrescriptionMedicines},
		monitor.Action{Name: "appointment-reminders", Fn: monitor.SendAppointmentReminders},
		monitor.Action{Name: "low-stock-alerts", Fn: monitor.SendLowStockAlerts},
		monitor.Action{Name: "cleanup-pdf-cache", Fn: monitor.CleanupPDFCache},
	)
	mon.Start()
	return mon
}

func runServer(cfg *config.Config, opts appOptions) {
	if !opts.noBrowser {
		go browser.WaitAndOpen()
	}

	e := server.CreateServer()

	if opts.dev || buildmode.Cloud {
		server.Start(e, cfg)
		return
	}

	go func() {
		server.Start(e, cfg)
		systray.Quit()
	}()

	systray.Run(func(ctx context.Context) error {
		return e.Shutdown(ctx)
	})
}
