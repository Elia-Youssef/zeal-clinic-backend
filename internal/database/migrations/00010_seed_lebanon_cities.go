package migrations

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedLebanonCities, downSeedLebanonCities)
}

func upSeedLebanonCities(ctx context.Context, tx *sql.Tx) error {
	data, err := dataFS.ReadFile("data/lebanon_cities.json")
	if err != nil {
		return err
	}
	var cities []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Governorate string `json:"governorate"`
		District    string `json:"district"`
	}
	if err := json.Unmarshal(data, &cities); err != nil {
		return err
	}

	for _, c := range cities {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO lebanon_cities (id, name, governorate, district)
			 SELECT ?, ?, ?, ?
			 WHERE NOT EXISTS (SELECT 1 FROM lebanon_cities WHERE name = ? AND governorate = ? AND district = ?)`,
			c.ID, c.Name, c.Governorate, c.District, c.Name, c.Governorate, c.District,
		); err != nil {
			return err
		}
	}
	return nil
}

func downSeedLebanonCities(ctx context.Context, tx *sql.Tx) error {
	return nil
}
