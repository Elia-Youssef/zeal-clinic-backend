package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedSelfBalance, downSeedSelfBalance)
}

func upSeedSelfBalance(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO balances (id, entity_type, entity_id, entity_name, currency_id, amount, created_at, updated_at)
		 VALUES (?, 'self', 'self', 'Clinic', ?, 0, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))`,
		"54f4f6eb-826e-4254-84d5-b5d0cad56b87", seedUSDCurrencyID,
	)
	return err
}

func downSeedSelfBalance(ctx context.Context, tx *sql.Tx) error {
	return nil
}
