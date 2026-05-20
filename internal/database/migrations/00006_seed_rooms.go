package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedRooms, downSeedRooms)
}

func upSeedRooms(ctx context.Context, tx *sql.Tx) error {
	rooms := []struct{ id, name, typ string }{
		{"99ca4a8e-9d61-415a-9afb-4f563b11242d", "Room 1", "Consultation"},
		{"25e6a41c-c76b-4b39-b959-07cf51f4305e", "Room 2", "Procedure"},
		{"27fa19ea-2e15-406b-b8af-a1f83ee9dd67", "Room 3", "General"},
		{"94214e6b-30c5-4173-b766-9765188d2102", "Room 4", "Consultation"},
		{"42c1c4e3-b14e-4aed-8742-43d1b0827fa4", "Room 5", "Procedure"},
		{"1f01c7de-f9f9-4f7b-bbf2-db051f2efda1", "Room 6", "General"},
		{"cd52a2a6-3e99-4e69-a51f-2c8289e6033d", "Room 7", "Consultation"},
		{"e7bf71a3-5fed-4cfb-a206-8e3b9524eb83", "Hospital", "Hospital"},
	}

	for _, r := range rooms {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO rooms (id, name, type, is_available, created_at)
			 SELECT ?, ?, ?, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
			 WHERE NOT EXISTS (SELECT 1 FROM rooms WHERE name = ?)`,
			r.id, r.name, r.typ, r.name,
		); err != nil {
			return err
		}
	}
	return nil
}

func downSeedRooms(ctx context.Context, tx *sql.Tx) error {
	return nil
}
