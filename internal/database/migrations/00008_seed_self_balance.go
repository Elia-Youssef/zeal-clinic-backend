package migrations

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedSelfBalance, downSeedSelfBalance)
}

func upSeedSelfBalance(ctx context.Context, tx *sql.Tx) error {
	var currencyID string
	if err := tx.QueryRowContext(ctx, "SELECT id FROM currencies WHERE code = 'USD'").Scan(&currencyID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO balances (id, entity_type, entity_id, entity_name, currency_id, amount, created_at, updated_at)
		 VALUES (?, 'self', 'self', 'Clinic', ?, 0, datetime('now'), datetime('now'))`,
		uuid.Must(uuid.NewV7()).String(), currencyID,
	)
	return err
}

func downSeedSelfBalance(ctx context.Context, tx *sql.Tx) error {
	return nil
}
