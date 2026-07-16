package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedDefaultAdmin, downSeedDefaultAdmin)
}

// upSeedDefaultAdmin creates a passwordless admin user when no users exist.
// The admin sets their password on first login. On any DB that already has
// users, this migration is a no-op.
func upSeedDefaultAdmin(ctx context.Context, tx *sql.Tx) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, display_name, role, is_active)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"b10829b3-19a0-4813-8e1c-df6f5990737b", "super-admin", "", "Super Admin", "super-admin", 1,
	)
	return err
}

func downSeedDefaultAdmin(ctx context.Context, tx *sql.Tx) error {
	return nil
}
