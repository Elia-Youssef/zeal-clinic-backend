package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite doesn't support concurrent writes

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}

	log.Println("Database initialized at", dbPath)
	return db, nil
}

func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return err
	}

	// Incremental migrations, safe to re-run (ignore "duplicate column" errors)
	alterStatements := []string{
		`ALTER TABLE appointments ADD COLUMN reminder_sent INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE appointments ADD COLUMN reminder_sent_at TEXT NOT NULL DEFAULT ''`,
		// Multi-currency and payment methods
		`ALTER TABLE transactions ADD COLUMN currency TEXT NOT NULL DEFAULT 'USD'`,
		`ALTER TABLE transactions ADD COLUMN payment_method TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE transactions ADD COLUMN exchange_rate REAL NOT NULL DEFAULT 1.0`,
		`ALTER TABLE transactions ADD COLUMN notes TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range alterStatements {
		_, _ = db.Exec(stmt) // ignore errors (column already exists)
	}

	return nil
}
