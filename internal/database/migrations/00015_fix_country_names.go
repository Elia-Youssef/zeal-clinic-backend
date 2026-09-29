package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upFixCountryNames, downFixCountryNames)
}

// countryNameFixes are the misspelled names the country list was seeded with.
// A row is renamed only while it still holds the misspelled name.
var countryNameFixes = []struct{ id, from, to string }{
	{"985e0564-4053-41e1-8b84-bbd44405b9f5", `AndorrA`, `Andorra`},
	{"71da3e8c-2168-4d84-8a92-0c7e4c040fc4", `Cote D"Ivoire`, `Cote d'Ivoire`},
	{"f5ad0814-ab63-41ed-8819-c7e64712828c", `Iran, Islamic Republic Of`, `Iran, Islamic Republic of`},
	{"d896daf4-8ff2-45a2-9cd5-7524736657d2", `Korea, Democratic People"S Republic of`, `Korea, Democratic People's Republic of`},
	{"dc8ed5c8-2b31-4a43-8a76-afe531277a1f", `Lao People"S Democratic Republic`, `Lao People's Democratic Republic`},
	{"66973910-d428-4b58-9c8b-023d3eca46fa", `RWANDA`, `Rwanda`},
}

func upFixCountryNames(ctx context.Context, tx *sql.Tx) error {
	for _, fix := range countryNameFixes {
		if _, err := tx.ExecContext(ctx,
			`UPDATE countries SET name = ? WHERE id = ? AND name = ?`,
			fix.to, fix.id, fix.from,
		); err != nil {
			return err
		}
	}
	return nil
}

func downFixCountryNames(ctx context.Context, tx *sql.Tx) error {
	return nil
}
