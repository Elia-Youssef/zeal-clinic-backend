package migrations

import (
	"context"
	"database/sql"
	"encoding/json"

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
	var entries []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	for _, e := range entries {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO countries (id, name) VALUES (?, ?)`,
			e.ID, e.Name,
		); err != nil {
			return err
		}
	}
	return nil
}

func downSeedCountries(ctx context.Context, tx *sql.Tx) error {
	return nil
}
