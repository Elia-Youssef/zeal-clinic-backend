package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedProcedureTypes, downSeedProcedureTypes)
}

func upSeedProcedureTypes(ctx context.Context, tx *sql.Tx) error {
	types := []struct{ id, name, desc string }{
		{"4347efc8-5f3d-46fc-8e1c-0620d8b6e4ec", "Clinic Procedure", ""},
		{"88c192e9-ab64-4537-9c71-af79936a30f7", "Hospital Surgery", ""},
		{"07881ca6-698b-444a-a45c-37cccbac2469", "Minor Surgery", ""},
	}

	for _, t := range types {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO procedure_types (id, name, description, created_at)
			 SELECT ?, ?, ?, datetime('now')
			 WHERE NOT EXISTS (SELECT 1 FROM procedure_types WHERE name = ?)`,
			t.id, t.name, t.desc, t.name,
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
