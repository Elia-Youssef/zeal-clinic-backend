package main

import (
	"context"
	"flag"
	"log"
	"time"

	"clinic-api/internal/api/server"
	"clinic-api/internal/browser"
	"clinic-api/internal/config"
	"clinic-api/internal/database"
	"clinic-api/internal/monitor"
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
	dev := flag.Bool("dev", false, "Development mode: no tray, no browser, db at ./dist/clinic.db")
	flag.Parse()

	if *dev {
		*noBrowser = true
	}

	// load env vars
	cfg := config.Load()

	// if it's not a seeding instance, check if an instance is already running
	if !*seedOnly && browser.ProbeHealth() {
		url := "http://localhost:" + cfg.Port
		log.Printf("[main] Already running at %s — opening browser", url)
		if !*noBrowser {
			_ = browser.Open(url)
		}
		return
	}

	// open the db
	dbPath := ""
	if *dev {
		dbPath = "./dist/clinic.db"
	}
	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()

	// seed and quit
	if *seedOnly {
		database.SeedIfEmpty(db)
		if *demo {
			if err := database.SeedDemo(db); err != nil {
				log.Fatal("Failed to seed demo data:", err)
			}
		}
		log.Println("[main] --seed-only complete; exiting")
		return
	}

	// start monitoring
	mon := monitor.New(1 * time.Minute)
	mon.Register(
		monitor.Action{Name: "expire-discounts", Fn: monitor.ExpireDiscounts},
		monitor.Action{Name: "expire-prescription-medicines", Fn: monitor.ExpirePrescriptionMedicines},
		monitor.Action{Name: "appointment-reminders", Fn: monitor.SendAppointmentReminders},
	)
	mon.Start()
	defer mon.Stop()

	// open the browser
	if !*noBrowser {
		go browser.WaitAndOpen()
	}

	e := server.CreateServer()

	// dev mode: no tray, run server in foreground
	if *dev {
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
