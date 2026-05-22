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
		"schedule-availability:write,schedule-availability:delete," +
		"suppliers:read,suppliers:write,suppliers:delete," +
		"update:read,update:write," +
		"users:read,users:write"

	userScopes := "allergies:read,analytics:read," +
		"appointments:delete,appointments:read,appointments:write," +
		"currencies:read,discounts:read,expenses:delete,expenses:read,expenses:write," +
		"invoices:delete,invoices:read,invoices:write,medicines:read," +
		"patient-allergies:read,patient-allergies:write,patient-medicines:read,patient-medicines:write," +
		"patients:delete,patients:read,patients:write,payments:delete,payments:read,payments:write," +
		"prescriptions:delete,prescriptions:read,prescriptions:write," +
		"procedure-allergy-conflicts:read,procedure-categories:read,procedure-types:read,procedures:read," +
		"product-allergy-conflicts:read,product-categories:read,products:read," +
		"rooms:read,suppliers:delete,suppliers:read,suppliers:write"

	roles := []struct {
		name, label, scopes string
	}{
		{"super-admin", "Super Admin", allScopes},
		{"admin", "Admin", allScopes},
		{"user", "User", userScopes},
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
	_, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE name IN ('super-admin','admin','user')`)
	return err
}
