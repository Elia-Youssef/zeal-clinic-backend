package migrations

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedRooms, downSeedRooms)
}

func upSeedRooms(ctx context.Context, tx *sql.Tx) error {
	rooms := []struct{ name, typ string }{
		{"Room 1", "Consultation"},
		{"Room 2", "Procedure"},
		{"Room 3", "General"},
		{"Room 4", "Consultation"},
		{"Room 5", "Procedure"},
		{"Room 6", "General"},
		{"Room 7", "Consultation"},
		{"Hospital", "Hospital"},
	}

	for _, r := range rooms {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO rooms (id, name, type, is_available, created_at)
			 SELECT ?, ?, ?, 1, datetime('now')
			 WHERE NOT EXISTS (SELECT 1 FROM rooms WHERE name = ?)`,
			uuid.Must(uuid.NewV7()).String(), r.name, r.typ, r.name,
		); err != nil {
			return err
		}
	}
	return nil
}

func downSeedRooms(ctx context.Context, tx *sql.Tx) error {
	return nil
}
