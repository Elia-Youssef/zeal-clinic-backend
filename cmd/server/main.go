package main

import (
	"log"

	"clinic-api/internal/api/server"
	"clinic-api/internal/config"
	"clinic-api/internal/database"
	"clinic-api/internal/database/models"
)

func main() {
	config.Load()

	db, err := database.Open(config.DBPath)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()
	models.DB = db

	if err := database.SeedIfEmpty(db); err != nil {
		log.Fatal("Failed to seed database:", err)
	}
	if err := database.SeedUsersIfEmpty(db); err != nil {
		log.Fatal("Failed to seed users:", err)
	}

	e := server.CreateServer()
	server.Start(e)
}
