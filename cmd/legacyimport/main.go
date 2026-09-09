package main

import (
	"flag"
	"fmt"
	"os"
	"slices"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/config"
	"clinic-api/internal/config/envfile"
	"clinic-api/internal/database"
	"clinic-api/internal/legacyimport"
)

func main() {
	in := flag.String("in", "old_data", "directory containing the legacy CSV files")
	db := flag.String("db", "", "path to the target clinic.db (default: the build's data path)")
	report := flag.String("report", "migration_report.txt", "path to write the migration report")
	backup := flag.Bool("backup", true, "take an encrypted DB snapshot before importing")
	force := flag.Bool("force", false, "import even if the database already holds business data")
	flag.Parse()

	if err := useConfigKey(config.Load()); err != nil {
		fmt.Fprintf(os.Stderr, "\nIMPORT FAILED: %v\n", err)
		os.Exit(1)
	}

	err := legacyimport.Run(legacyimport.Options{
		InputDir:   *in,
		DBPath:     *db,
		Backup:     *backup,
		Force:      *force,
		ReportPath: *report,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nIMPORT FAILED: %v\n", err)
		os.Exit(1)
	}
}

// useConfigKey gives the database package the DB key of this build's config
// (DB_ENCRYPTION_KEY of local.env, else the dev default), after the same
// config check as the server.
func useConfigKey(cfg *config.Config) error {
	if err := cfg.Check(buildmode.Release()); err != nil {
		return err
	}
	if err := database.SetKey(cfg.DBEncryptionKey); err != nil {
		return err
	}
	if slices.Contains(cfg.DevKeys(), envfile.KeyDBEncryptionKey) {
		fmt.Println("Note    : DB_ENCRYPTION_KEY is the dev default; a release server cannot open this database")
	}
	return nil
}
