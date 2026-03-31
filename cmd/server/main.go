package main

import (
	"log"

	"clinic-api/internal/api/server"
	"clinic-api/internal/config"
	"clinic-api/internal/database"
)

func main() {
	config.Load()

	db, err := database.Open(config.DBPath)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()

	database.SeedIfEmpty(db)

	e := server.CreateServer()
	server.Start(e)
}
