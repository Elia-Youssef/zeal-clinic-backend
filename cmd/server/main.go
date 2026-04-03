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
	flag.Parse()

	config.Load()

	db, err := database.Open(config.DBPath)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()

	if *seed {
		database.SeedIfEmpty(db)
	}

	e := server.CreateServer()
	server.Start(e)
}
