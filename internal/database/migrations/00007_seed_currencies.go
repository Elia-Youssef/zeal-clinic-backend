package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedCurrencies, downSeedCurrencies)
}

// Currencies are keyed by their ISO code (see store.USDCurrencyID): naturally
// stable across synced DBs, so no surrogate UUID is needed.
const seedUSDCurrencyID = "USD"
const seedLBPCurrencyID = "LBP"

func upSeedCurrencies(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO currencies (id, code, name, symbol, exchange_rate, created_at, updated_at)
		 VALUES
		 (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
		 (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))`,
		seedUSDCurrencyID, "USD", "US Dollar", "$", 1.0,
		seedLBPCurrencyID, "LBP", "Lebanese Pound", "LBP", 89500.0,
	)
	return err
}

func downSeedCurrencies(ctx context.Context, tx *sql.Tx) error {
	return nil
}
