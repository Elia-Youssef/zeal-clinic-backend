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
	allScopes := "appointments:read,appointments:write,appointments:delete," +
		"patients:read,patients:write,patients:delete," +
		"team:read,team:write,team:delete," +
		"transactions:read,transactions:write,transactions:delete," +
		"inventory:read,inventory:write,inventory:delete," +
		"services:read,services:write,services:delete," +
		"rooms:read,rooms:write,rooms:delete," +
		"schedule:read,schedule:write,schedule:delete," +
		"roles:read,roles:write," +
		"reports:read"

	roles := []struct {
		name, label, scopes string
	}{
		{"super-admin", "Super Admin", allScopes},
		{"admin", "Admin", allScopes},
		{"user", "User", "appointments:read,patients:read,team:read,rooms:read,schedule:read,services:read,inventory:read,reports:read"},
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
