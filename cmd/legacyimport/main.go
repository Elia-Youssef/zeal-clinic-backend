package main

import (
	"flag"
	"fmt"
	"os"

	"clinic-api/internal/legacyimport"
)

func main() {
	in := flag.String("in", "old_data", "directory containing the legacy CSV files")
	db := flag.String("db", "", "path to the target clinic.db (default: the build's data path)")
	report := flag.String("report", "migration_report.txt", "path to write the migration report")
	backup := flag.Bool("backup", true, "take an encrypted DB snapshot before importing")
	force := flag.Bool("force", false, "import even if the database already holds business data")
	flag.Parse()

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
