package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upFixAlandIslands, downFixAlandIslands)
}

func upFixAlandIslands(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE countries SET name = 'Åland Islands' WHERE id = 'a5fa8f1e-72e3-41a5-8ef1-22f3395e194d' AND name = 'Ã…land Islands'`,
	)
	return err
}

func downFixAlandIslands(ctx context.Context, tx *sql.Tx) error {
	return nil
}
