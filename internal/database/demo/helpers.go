package demo

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

func newID() string { return uuid.Must(uuid.NewV7()).String() }

func nowStr() string   { return time.Now().Format(time.RFC3339) }
func todayStr() string { return time.Now().Format("2006-01-02") }

func dateOffset(days int) string {
	return time.Now().AddDate(0, 0, days).Format("2006-01-02")
}

// timeAt returns an RFC3339 timestamp at `days` from today, at the given hour
// and minute in the local timezone.
func timeAt(days, hour, minute int) string {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()+days, hour, minute, 0, 0, time.Local).
		Format(time.RFC3339)
}

// demoCtx holds shared lookups + the singletons every section needs (currency,
// clinic balance, the admin user that records transactions).
type demoCtx struct {
	currencyID    string
	selfBalanceID string
	adminUserID   string

	now   string
	today string

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

	doctorIDs   []string
	patientIDs  []string
	employeeIDs []string
}

func newDemoCtx(ctx context.Context, tx *sql.Tx) (*demoCtx, error) {
	c := &demoCtx{
		now:         nowStr(),
		today:       todayStr(),
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
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM currencies WHERE code = 'USD'`).Scan(&c.currencyID); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM balances WHERE entity_type = 'self'`).Scan(&c.selfBalanceID); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username = 'admin'`).Scan(&c.adminUserID); err != nil {
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

// recordTransaction inserts a balance_transactions row and updates both
// balances in lockstep. Charges and adjustments don't bump total_in/total_out
// because those track money actually exchanged.
func recordTransaction(ctx context.Context, tx *sql.Tx, c *demoCtx, fromID, toID string, amount float64, txType, method, desc, at string) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO balance_transactions (id, from_balance_id, to_balance_id, amount, currency_id, transaction_type, transaction_method, description, created_by, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		newID(), fromID, toID, amount, c.currencyID, txType, method, desc, c.adminUserID, at,
	); err != nil {
		return err
	}
	flowDelta := amount
	if txType == "charge" || txType == "adjustment" {
		flowDelta = 0
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE balances SET amount = amount - ?, total_out = total_out + ?, updated_at = ? WHERE id = ?`,
		amount, flowDelta, at, fromID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE balances SET amount = amount + ?, total_in = total_in + ?, updated_at = ? WHERE id = ?`,
		amount, flowDelta, at, toID,
	); err != nil {
		return err
	}
	return nil
}
