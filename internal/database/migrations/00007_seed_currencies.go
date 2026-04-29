package migrations

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedCurrencies, downSeedCurrencies)
}

func upSeedCurrencies(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO currencies (id, code, name, symbol, exchange_rate, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		uuid.Must(uuid.NewV7()).String(), "USD", "US Dollar", "$", 1.0,
	)
	return err
}

func downSeedCurrencies(ctx context.Context, tx *sql.Tx) error {
	return nil
}
