package migrations

import (
	"context"
	"database/sql"
	"runtime"

	"clinic-api/internal/buildmode"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedCurrentVersion, downSeedCurrentVersion)
}

// upSeedCurrentVersion records the running build in the versions table so update
// status reports a real ReleasedAt after a fresh install, before any update has
// been published or synced in. url is left empty (no artifact to download). When
// a row for this platform+version already exists (e.g. synced from cloud), this
// migration is a no-op.
func upSeedCurrentVersion(ctx context.Context, tx *sql.Tx) error {
	platform := runtime.GOOS
	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM versions WHERE platform = ? AND version = ?`,
		platform, buildmode.Version).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO versions (id, version, platform, url) VALUES (?, ?, ?, '')`,
		uuid.Must(uuid.NewV7()).String(), buildmode.Version, platform,
	)
	return err
}

func downSeedCurrentVersion(ctx context.Context, tx *sql.Tx) error {
	return nil
}
