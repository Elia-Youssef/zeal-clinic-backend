package migrations

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedCountries, downSeedCountries)
}

func upSeedCountries(ctx context.Context, tx *sql.Tx) error {
	data, err := dataFS.ReadFile("data/countries.json")
	if err != nil {
		return err
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}

	for _, name := range names {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO countries (id, name) VALUES (?, ?)`,
			uuid.Must(uuid.NewV7()).String(), name,
		); err != nil {
			return err
		}
	}
	return nil
}

func downSeedCountries(ctx context.Context, tx *sql.Tx) error {
	return nil
}
