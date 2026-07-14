package legacyimport

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"clinic-api/internal/config"
	"clinic-api/internal/database"
	"clinic-api/internal/database/store"
)

type Options struct {
	InputDir   string
	DBPath     string
	Backup     bool
	Force      bool
	ReportPath string
}

var preflightTables = []string{
	"patients", "appointments", "invoices", "invoice_items",
	"balance_transactions", "products",
}

func Run(opts Options) error {
	ctx := context.Background()

	inDir, err := resolveInputDir(opts.InputDir)
	if err != nil {
		return err
	}
	fmt.Printf("Input   : %s\n", inDir)

	db, err := database.Open(opts.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	if err := preflight(ctx, db, opts.Force); err != nil {
		return err
	}

	if opts.Backup {
		dir := backupDir(opts.DBPath)
		path, err := database.Snapshot(dir)
		if err != nil {
			return fmt.Errorf("backup before import: %w", err)
		}
		fmt.Printf("Backup  : %s\n", path)
	}

	c := newContext(inDir)

	for _, name := range []string{
		"patients.csv", "appointments.csv", "categories.csv",
		"products.csv", "invoices.csv", "invoice_items.csv", "transactions.csv",
	} {
		if _, err := c.File(name); err != nil {
			return err
		}
	}

	if err := loadReferences(ctx, db, c); err != nil {
		return err
	}

	roomsTopUp, err := scanExtraRooms(c)
	if err != nil {
		return err
	}
	codeLabels, err := loadProcedureCodeLabels(c)
	if err != nil {
		return err
	}
	prodCats, products, prodPrices, err := migrateProducts(c)
	if err != nil {
		return err
	}

	procedures, procedurePrices, err := migrateProcedures(c, codeLabels)
	if err != nil {
		return err
	}
	patients, err := loadPatients(c)
	if err != nil {
		return err
	}
	appointments, appointmentProcedures, err := migrateAppointments(c, codeLabels)
	if err != nil {
		return err
	}
	invoices, invNumToID, err := migrateInvoices(c)
	if err != nil {
		return err
	}
	invoiceItems, err := migrateInvoiceItems(c, invNumToID)
	if err != nil {
		return err
	}
	transactions, err := migrateTransactions(c, invNumToID)
	if err != nil {
		return err
	}
	patientsTable := patients.finalize(c)
	patientBalances := buildPatientBalances(c)

	if err := writeAll(ctx, db, c, importPlan{
		roomsTopUp:            roomsTopUp,
		prodCats:              prodCats,
		products:              products,
		prodPrices:            prodPrices,
		procedures:            procedures,
		procedurePrices:       procedurePrices,
		patients:              patientsTable,
		patientBalances:       patientBalances,
		appointments:          appointments,
		appointmentProcedures: appointmentProcedures,
		invoices:              invoices,
		invoiceItems:          invoiceItems,
		transactions:          transactions,
	}); err != nil {
		return err
	}

	if err := c.Report.WriteFile(opts.ReportPath); err != nil {
		return err
	}
	fmt.Println()
	c.Report.PrintSummary()
	fmt.Printf("\nImport committed to the database. Report: %s\n", opts.ReportPath)
	return nil
}

type importPlan struct {
	roomsTopUp            *rowset
	prodCats              *rowset
	products              *rowset
	prodPrices            *rowset
	procedures            *rowset
	procedurePrices       *rowset
	patients              *rowset
	patientBalances       *rowset
	appointments          *rowset
	appointmentProcedures *rowset
	invoices              *rowset
	invoiceItems          *rowset
	transactions          *rowset
}

func writeAll(ctx context.Context, db *sql.DB, c *Context, p importPlan) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin import tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `UPDATE _sync_applying SET applying = 1 WHERE rowid = 1`); err != nil {
		return fmt.Errorf("raise sync guard: %w", err)
	}

	steps := []struct {
		table string
		rows  *rowset
	}{
		{"rooms", p.roomsTopUp},
		{"product_categories", p.prodCats},
		{"products", p.products},
		{"product_prices", p.prodPrices},
		{"procedures", p.procedures},
		{"procedure_prices", p.procedurePrices},
		{"patients", p.patients},
		{"balances", p.patientBalances},
		{"appointments", p.appointments},
		{"appointment_procedures", p.appointmentProcedures},
		{"invoices", p.invoices},
		{"invoice_items", p.invoiceItems},
		{"balance_transactions", p.transactions},
	}
	for _, s := range steps {
		if err = s.rows.insert(ctx, tx, s.table); err != nil {
			return err
		}
	}

	balanceIDs := make([]string, 0, len(c.financialPatients)+1)
	balanceIDs = append(balanceIDs, c.selfBalanceID)
	for _, pid := range c.financialPatients {
		balanceIDs = append(balanceIDs, c.patientBalanceID(pid))
	}
	if err = store.RecalculateBalancesWithTx(tx, balanceIDs...); err != nil {
		return fmt.Errorf("recalculate balances: %w", err)
	}

	if _, err = tx.ExecContext(ctx, `UPDATE _sync_applying SET applying = 0 WHERE rowid = 1`); err != nil {
		return fmt.Errorf("lower sync guard: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit import: %w", err)
	}
	return nil
}

func preflight(ctx context.Context, db *sql.DB, force bool) error {
	for _, table := range preflightTables {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
			return fmt.Errorf("preflight %s: %w", table, err)
		}
		if n > 0 {
			if force {
				fmt.Printf("Warning : %s already has %d row(s); importing anyway (--force).\n", table, n)
				return nil
			}
			return fmt.Errorf("database already contains data (%s has %d rows); "+
				"import expects a freshly migrated DB. Re-run against an empty DB, or pass --force to override", table, n)
		}
	}
	return nil
}

func loadReferences(ctx context.Context, db *sql.DB, c *Context) error {
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM balances WHERE entity_type='self' AND entity_id='self' AND currency_id=? LIMIT 1`,
		usdCurrency,
	).Scan(&c.selfBalanceID); err != nil {
		return fmt.Errorf("resolve self balance (is the DB seeded?): %w", err)
	}

	if err := loadNameMap(ctx, db, `SELECT id, name FROM rooms`, c.roomID); err != nil {
		return fmt.Errorf("load rooms: %w", err)
	}
	if err := loadNameMap(ctx, db, `SELECT id, name FROM procedure_types`, c.procTypeID); err != nil {
		return fmt.Errorf("load procedure types: %w", err)
	}

	if err := loadNameMap(ctx, db, `SELECT id, name FROM procedures`, c.procByName); err != nil {
		return fmt.Errorf("load procedures: %w", err)
	}
	return nil
}

func loadNameMap(ctx context.Context, db *sql.DB, query string, dst map[string]string) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		if _, ok := dst[name]; !ok {
			dst[name] = id
		}
	}
	return rows.Err()
}

func backupDir(dbPath string) string {
	dbFile := dbPath
	if dbFile == "" {
		dbFile = filepath.Join(config.DataDir(), "clinic.db")
	}
	return filepath.Join(filepath.Dir(dbFile), "backup")
}

func resolveInputDir(in string) (string, error) {
	if st, err := os.Stat(in); err == nil && st.IsDir() {
		return in, nil
	}
	if exe, err := os.Executable(); err == nil {
		alt := filepath.Join(filepath.Dir(exe), in)
		if st, err := os.Stat(alt); err == nil && st.IsDir() {
			return alt, nil
		}
	}
	return "", fmt.Errorf("input directory %q not found", in)
}
