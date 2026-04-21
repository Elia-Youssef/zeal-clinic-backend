package main

import (
	"flag"
	"log"

	"clinic-api/internal/api/server"
	"clinic-api/internal/config"
	"clinic-api/internal/database"
)

func main() {
	seed := flag.Bool("seed", false, "Seed the database with default data")
	demo := flag.Bool("demo", false, "Seed the database with a large demo dataset (includes --seed)")
	flag.Parse()

	cfg := config.Load()

	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()

	if *seed || *demo {
		database.SeedIfEmpty(db)
	}
	if *demo {
		if err := database.SeedDemo(db); err != nil {
			log.Fatal("Failed to seed demo data:", err)
		}
	}

	e := server.CreateServer()
	server.Start(e, cfg)
}
