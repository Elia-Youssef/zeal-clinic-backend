package demo

import (
	"context"
	"database/sql"
	"math/rand"
	"time"

	"clinic-api/internal/database/store"

	"github.com/google/uuid"
)

func newID() string { return uuid.Must(uuid.NewV7()).String() }

func nowStr() string   { return string(store.DateNow()) }
func todayStr() string { return string(store.DateToday()) }

func dateOffset(days int) string { return string(store.DateOffsetDays(days)) }

// timeAt returns an RFC3339 UTC timestamp at `days` from today, at the given
// clinic-local hour and minute; "today" is the clinic's calendar day.
func timeAt(days, hour, minute int) string {
	loc := store.ClinicLocation()
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day()+days, hour, minute, 0, 0, loc).
		UTC().Format(time.RFC3339)
}

// bulkPatient tracks a generated patient and when (days before today) they were
// created, so historical appointments only reference patients that already
// existed on that day.
type bulkPatient struct {
	id, bal    string
	createdOff int
}

// demoCtx holds shared lookups + the singletons every section needs (currency,
// clinic balance, the admin user that records transactions).
type demoCtx struct {
	currencyID    string
	selfBalanceID string
	adminUserID   string

	now   string
	today string

	rng          *rand.Rand
	bulkPatients []bulkPatient

	allergies  map[string]string // name -> id
	medicines  map[string]string
	products   map[string]string
	rooms      map[string]string
	cities     map[string]string
	countries  map[string]string
	procedures map[string]string // name -> first matching id

	employeeBal map[string]string // employee id -> balance id
	patientBal  map[string]string // patient id -> balance id
	supplierBal map[string]string // supplier id -> balance id
	expenseBal  map[string]string // expense id -> balance id

	doctorIDs   []string
	patientIDs  []string
	employeeIDs []string
	supplierIDs []string
	expenseIDs  []string
}

func newDemoCtx(ctx context.Context, tx *sql.Tx) (*demoCtx, error) {
	c := &demoCtx{
		now:         nowStr(),
		today:       todayStr(),
		rng:         rand.New(rand.NewSource(20240601)),
		allergies:   map[string]string{},
		medicines:   map[string]string{},
		products:    map[string]string{},
		rooms:       map[string]string{},
		cities:      map[string]string{},
		countries:   map[string]string{},
		procedures:  map[string]string{},
		employeeBal: map[string]string{},
		patientBal:  map[string]string{},
		supplierBal: map[string]string{},
		expenseBal:  map[string]string{},
	}
	c.currencyID = store.USDCurrencyID
	if err := tx.QueryRowContext(ctx, `SELECT id FROM balances WHERE entity_type = 'self' AND currency_id = ?`, c.currencyID).Scan(&c.selfBalanceID); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username = 'super-admin'`).Scan(&c.adminUserID); err != nil {
		return nil, err
	}
	if err := loadNameMap(ctx, tx, `SELECT id, name FROM rooms`, c.rooms); err != nil {
		return nil, err
	}
	if err := loadNameMap(ctx, tx, `SELECT id, name FROM lebanon_cities`, c.cities); err != nil {
		return nil, err
	}
	if err := loadNameMap(ctx, tx, `SELECT id, name FROM countries`, c.countries); err != nil {
		return nil, err
	}
	if err := loadNameMap(ctx, tx, `SELECT id, name FROM procedures`, c.procedures); err != nil {
		return nil, err
	}
	return c, nil
}

func loadNameMap(ctx context.Context, tx *sql.Tx, query string, dst map[string]string) error {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		if _, exists := dst[name]; !exists {
			dst[name] = id
		}
	}
	return rows.Err()
}

// createBalance inserts a balance row and returns its id.
func createBalance(ctx context.Context, tx *sql.Tx, c *demoCtx, entityType, entityID, name string) (string, error) {
	id := newID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO balances (id, entity_type, entity_id, entity_name, currency_id, amount, created_at, updated_at)
		 VALUES (?,?,?,?,?,0,?,?)`,
		id, entityType, entityID, name, c.currencyID, c.now, c.now,
	); err != nil {
		return "", err
	}
	return id, nil
}

// recordTransaction inserts a balance_transactions row and rebuilds the
// affected cached balance fields. Charges and adjustments don't bump
// total_in/total_out, which track money actually exchanged.
func recordTransaction(ctx context.Context, tx *sql.Tx, c *demoCtx, fromID, toID string, amount float64, txType, method, desc, at string) error {
	return recordTransactionSource(ctx, tx, c, fromID, toID, amount, txType, method, "", "", desc, at)
}

func recordTransactionSource(ctx context.Context, tx *sql.Tx, c *demoCtx, fromID, toID string, amount float64, txType, method, sourceType, sourceID, desc, at string) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO balance_transactions (id, from_balance_id, to_balance_id, amount, currency_id, transaction_type, transaction_method, source_type, source_id, description, created_by, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		newID(), fromID, toID, amount, c.currencyID, txType, method, sourceType, sourceID, desc, c.adminUserID, at,
	); err != nil {
		return err
	}
	return store.RecalculateBalancesWithTx(tx, fromID, toID)
}

// insertTxn inserts a transaction WITHOUT recalculating balances. Bulk seeding
// uses this and recomputes every balance once at the end (recalcAllBalances),
// avoiding O(n²) re-summing of the heavily-shared self balance.
func insertTxn(ctx context.Context, tx *sql.Tx, c *demoCtx, fromID, toID string, amount float64, txType, method, sourceType, sourceID, desc, at string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO balance_transactions (id, from_balance_id, to_balance_id, amount, currency_id, transaction_type, transaction_method, source_type, source_id, description, created_by, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		newID(), fromID, toID, amount, c.currencyID, txType, method, sourceType, sourceID, desc, c.adminUserID, at,
	)
	return err
}

// recalcAllBalances rebuilds the cached amount/total_in/total_out for every
// balance the demo touched, from the full transaction history.
func recalcAllBalances(tx *sql.Tx, c *demoCtx) error {
	ids := []string{c.selfBalanceID}
	for _, m := range []map[string]string{c.patientBal, c.supplierBal, c.expenseBal, c.employeeBal} {
		for _, id := range m {
			ids = append(ids, id)
		}
	}
	return store.RecalculateBalancesWithTx(tx, ids...)
}

// random helpers (demo only)

func (c *demoCtx) pick(list []string) string { return list[c.rng.Intn(len(list))] }

func (c *demoCtx) chance(p float64) bool { return c.rng.Float64() < p }

// between returns a random int in [lo, hi].
func (c *demoCtx) between(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + c.rng.Intn(hi-lo+1)
}

// weekdayOf returns the weekday of the clinic day `off` days from today.
func weekdayOf(off int) time.Weekday {
	return store.ClinicNow().AddDate(0, 0, off).Weekday()
}
