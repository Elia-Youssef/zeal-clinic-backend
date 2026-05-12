package main

import (
	"context"
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

	"github.com/labstack/echo/v4"
)

type trayServer struct {
	e    *echo.Echo
	port string
}

func (t *trayServer) Port() string                       { return t.port }
func (t *trayServer) Shutdown(ctx context.Context) error { return t.e.Shutdown(ctx) }

func main() {
	seedOnly := flag.Bool("seed-only", false, "Seed and exit; do not start the server (used by the installer)")
	demo := flag.Bool("demo", false, "Seed the database with a large demo dataset (includes --seed)")
	noBrowser := flag.Bool("no-browser", false, "Don't auto-open the browser on startup")
	dev := flag.Bool("dev", false, "Development mode: no tray, no browser, db at ./tmp/clinic.db")
	flag.Parse()

	if *dev || buildmode.Cloud {
		*noBrowser = true
	}

	// load env vars
	cfg := config.Load()

	// if it's not a seeding instance, check if an instance is already running.
	// Skipped in cloud builds: there's no peer process to hand off to and no
	// browser to open.
	if !buildmode.Cloud && !*seedOnly && browser.ProbeHealth() {
		url := "http://localhost:" + cfg.Port
		log.Printf("[main] Already running at %s — opening browser", url)
		if !*noBrowser {
			_ = browser.Open(url)
		}
		return
	}

	// open the db
	db, err := database.Open("")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()

	// seed and quit
	if *seedOnly {
		if *demo {
			if err := database.SeedDemo(db); err != nil {
				log.Fatal("Failed to seed demo data:", err)
			}
		}
		log.Println("[main] --seed-only complete; exiting")
		return
	}

	// Wire the sync package's cache-invalidation hook to the HTTP cache
	// middleware. Sync can't import middleware directly (cycle through
	// database/store), so the indirection lives in sync as a function var.
	syncpkg.InvalidateCache = middleware.InvalidateCache

	// start the sync engine. On local clinic servers PeerURL points to the
	// cloud; on the cloud it's empty (cloud is passive, local dials in).
	// Engine.Start runs the apply-guard reset, the sync_log watcher, and
	// (when PeerURL is set) the outbound loop and SSE listener.
	syncCtx, syncCancel := context.WithCancel(context.Background())
	defer syncCancel()
	engine := syncpkg.New(db, syncpkg.Config{
		PeerURL: cfg.PeerURL,
		Secret:  cfg.SyncSecret,
	})
	engine.Start(syncCtx)
	defer engine.Stop()

	// start monitoring
	mon := monitor.New(1 * time.Minute)
	mon.Register(
		monitor.Action{Name: "expire-discounts", Fn: monitor.ExpireDiscounts},
		monitor.Action{Name: "expire-prescription-medicines", Fn: monitor.ExpirePrescriptionMedicines},
		monitor.Action{Name: "appointment-reminders", Fn: monitor.SendAppointmentReminders},
		monitor.Action{Name: "low-stock-alerts", Fn: monitor.SendLowStockAlerts},
	)
	mon.Start()
	defer mon.Stop()

	// open the browser
	if !*noBrowser {
		go browser.WaitAndOpen()
	}

	e := server.CreateServer()

	// dev or cloud: no tray, run server in foreground
	if *dev || buildmode.Cloud {
		server.Start(e, cfg)
		return
	}

	// start the server; if it exits on its own, tear down the tray too
	go func() {
		server.Start(e, cfg)
		systray.Quit()
	}()

	// block on tray; Quit triggers graceful shutdown via the adapter
	systray.Run(&trayServer{e: e, port: cfg.Port})
}
