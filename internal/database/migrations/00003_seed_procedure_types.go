package migrations

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedProcedureTypes, downSeedProcedureTypes)
}

func upSeedProcedureTypes(ctx context.Context, tx *sql.Tx) error {
	types := []struct{ name, desc string }{
		{"Clinic Procedure", ""},
		{"Hospital Surgery", ""},
		{"Minor Surgery", ""},
	}

	for _, t := range types {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO procedure_types (id, name, description, created_at)
			 SELECT ?, ?, ?, datetime('now')
			 WHERE NOT EXISTS (SELECT 1 FROM procedure_types WHERE name = ?)`,
			uuid.Must(uuid.NewV7()).String(), t.name, t.desc, t.name,
		); err != nil {
			return err
		}
	}
	return nil
}

func downSeedProcedureTypes(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM procedure_types WHERE name IN ('Clinic Procedure','Hospital Surgery','Minor Surgery')`)
	return err
}
