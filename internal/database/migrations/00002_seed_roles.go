package migrations

import (
	"context"
	"database/sql"
	"strings"

	"clinic-api/internal/scopes"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedRoles, downSeedRoles)
}

func upSeedRoles(ctx context.Context, tx *sql.Tx) error {
	allScopes := strings.Join(scopes.All, ",")

	staffScopes := "allergies:read,analytics:read," +
		"appointments:delete,appointments:read,appointments:write," +
		"balances:read,currencies:read," +
		"discounts:read,discounts:write," +
		"expenses:read,expenses:write," +
		"hr:read,invoices:read,invoices:write,medicines:read," +
		"patient-allergies:read,patient-allergies:write," +
		"patient-medicines:read,patient-medicines:write," +
		"patients:read,patients:write,payments:read,payments:write," +
		"prescriptions:read," +
		"procedure-allergy-conflicts:read,procedure-categories:read,procedure-types:read,procedures:read," +
		"product-allergy-conflicts:read,product-categories:read,products:read," +
		"rooms:read,suppliers:read,suppliers:write,update:read"

	nurseScopes := "allergies:read,analytics:read," +
		"appointments:read,medicines:read,medicines:write," +
		"patient-allergies:read,patient-allergies:write," +
		"patient-medicines:read,patient-medicines:write," +
		"patients:read,patients:write," +
		"prescriptions:read,prescriptions:write," +
		"procedure-allergy-conflicts:read,procedure-allergy-conflicts:write," +
		"procedure-categories:read,procedure-types:read,procedures:read," +
		"product-allergy-conflicts:read,product-categories:read,products:read," +
		"rooms:read"

	roles := []struct {
		name, label, scopes string
	}{
		{"super-admin", "Super Admin", allScopes},
		{"admin", "Admin", allScopes},
		{"staff", "Staff", staffScopes},
		{"nurse", "Nurse", nurseScopes},
	}

	for _, r := range roles {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO roles (name, label, scopes) VALUES (?, ?, ?) ON CONFLICT(name) DO NOTHING`,
			r.name, r.label, r.scopes,
		); err != nil {
			return err
		}
	}
	return nil
}

func downSeedRoles(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE name IN ('super-admin','admin','staff','nurse')`)
	return err
}
