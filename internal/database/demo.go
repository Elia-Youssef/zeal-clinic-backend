package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"clinic-api/internal/database/demo"

	"github.com/pressly/goose/v3"
)

// SeedDemo applies the dummy demo dataset on top of the base schema + seeds.
// Implemented as a one-off goose migration tracked in goose_demo_version, kept
// separate from the main migration history so production and demo data don't
// share a tracker.
func SeedDemo(db *sql.DB) error {
	provider, err := goose.NewProvider(
		goose.DialectSQLite3,
		db,
		nil,
		goose.WithDisableGlobalRegistry(true),
		goose.WithTableName("goose_demo_version"),
		goose.WithGoMigrations(demo.Migrations()...),
	)
	if err != nil {
		return fmt.Errorf("demo provider: %w", err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		return fmt.Errorf("demo up: %w", err)
	}
	log.Println("Demo seed complete")
	return nil
}
