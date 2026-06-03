package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedRoles, downSeedRoles)
}

func upSeedRoles(ctx context.Context, tx *sql.Tx) error {
	allScopes := "allergies:read,allergies:write,allergies:delete," +
		"analytics:read," +
		"appointments:read,appointments:write,appointments:delete," +
		"audit:read," +
		"balances:read," +
		"currencies:read,currencies:write,currencies:delete," +
		"discounts:read,discounts:write,discounts:delete," +
		"employees:read,employees:write,employees:delete," +
		"employee-payments:read,employee-payments:write,employee-payments:delete," +
		"employee-salaries:read,employee-salaries:write,employee-salaries:delete," +
		"expenses:read,expenses:write,expenses:delete," +
		"hr:read,hr:write,hr:delete," +
		"invoices:read,invoices:write,invoices:delete," +
		"medicines:read,medicines:write,medicines:delete," +
		"patients:read,patients:write,patients:delete," +
		"patient-allergies:read,patient-allergies:write," +
		"patient-medicines:read,patient-medicines:write," +
		"payments:read,payments:write,payments:delete," +
		"prescriptions:read,prescriptions:write,prescriptions:delete," +
		"procedures:read,procedures:write,procedures:delete," +
		"procedure-categories:read,procedure-categories:write," +
		"procedure-types:read,procedure-types:write," +
		"procedure-allergy-conflicts:read,procedure-allergy-conflicts:write," +
		"products:read,products:write,products:delete," +
		"product-categories:read,product-categories:write,product-categories:delete," +
		"product-allergy-conflicts:read,product-allergy-conflicts:write," +
		"reports:read," +
		"roles:read,roles:write," +
		"rooms:read,rooms:write,rooms:delete," +
		"employee-schedules:write,employee-schedules:delete," +
		"suppliers:read,suppliers:write,suppliers:delete," +
		"update:read,update:write," +
		"users:read,users:write"

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
