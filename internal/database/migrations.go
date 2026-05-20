package database

import (
	"database/sql"
	"fmt"
	"log"

	"clinic-api/internal/database/migrations"
	"clinic-api/internal/tracking"

	"github.com/pressly/goose/v3"
)

// Migrate brings the database up to the latest schema and seeds. SQL schema
// migrations live in internal/database/migrations/*.sql; Go migrations (data
// backfills, reference seeds) live alongside them and self-register through
// goose.AddMigrationContext in their init(). Goose tracks applied versions in
// goose_db_version, so each migration runs exactly once per database.
func Migrate(db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(gooseLogger{})

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	before, _ := goose.GetDBVersion(db)
	if err := goose.Up(db, "."); err != nil {
		tracking.CaptureError(nil, fmt.Errorf("[migrations] failed at version %d: %w", before, err))
		return fmt.Errorf("goose up: %w", err)
	}
	if after, _ := goose.GetDBVersion(db); after > before {
		tracking.Info(nil, fmt.Sprintf("[migrations] applied %d -> %d", before, after))
	}
	return nil
}

// gooseLogger routes goose's chatter through the standard logger so migration
// progress lands in the same place as the rest of the server's output.
type gooseLogger struct{}

func (gooseLogger) Fatalf(format string, v ...interface{}) {
	tracking.Fatal(fmt.Sprintf(format, v...), nil)
}
func (gooseLogger) Printf(format string, v ...interface{}) { log.Printf(format, v...) }
