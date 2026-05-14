package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type BalanceTransaction struct {
	ID                string  `json:"id"`
	FromBalanceID     string  `json:"fromBalanceId"`
	ToBalanceID       string  `json:"toBalanceId"`
	Amount            float64 `json:"amount"`
	CurrencyID        string  `json:"currencyId"`
	TransactionType   string  `json:"transactionType"`
	TransactionMethod string  `json:"transactionMethod"`
	// SourceType / SourceID link this transaction back to the entity that
	// produced it (e.g. "invoice", "supplier_invoice"). Empty for ad-hoc
	// transactions like manual payments / adjustments. Used by callers to
	// reverse side effects when the source entity is deleted.
	SourceType  string `json:"sourceType,omitempty"`
	SourceID    string `json:"sourceId,omitempty"`
	Description string `json:"description"`
	CreatedBy   string `json:"createdBy"`
	CreatedAt   Date   `json:"createdAt"`
	VoidedAt    Date   `json:"voidedAt,omitempty"`
	// Joined fields
	FromEntityName string `json:"fromEntityName,omitempty"`
	ToEntityName   string `json:"toEntityName,omitempty"`
}

func (bt *BalanceTransaction) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(bt.FromBalanceID, "From balance ID"); msg != "" {
		e["fromBalanceId"] = msg
	}
	if msg := validation.Required(bt.ToBalanceID, "To balance ID"); msg != "" {
		e["toBalanceId"] = msg
	}
	if msg := validation.Positive(bt.Amount, "Amount"); msg != "" {
		e["amount"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const balanceTransactionColumnsNoId = `from_balance_id, to_balance_id, amount, currency_id, transaction_type, transaction_method, source_type, source_id, description, created_by, created_at, voided_at`
const balanceTransactionColumns = `id, ` + balanceTransactionColumnsNoId

type BalanceTransactionList []BalanceTransaction

func (m *BalanceTransaction) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil BalanceTransaction row")
	}
	return row.Scan(&m.ID, &m.FromBalanceID, &m.ToBalanceID, &m.Amount, &m.CurrencyID,
		&m.TransactionType, &m.TransactionMethod, &m.SourceType, &m.SourceID,
		&m.Description, &m.CreatedBy, &m.CreatedAt, &m.VoidedAt,
		&m.FromEntityName, &m.ToEntityName)
}

func (l *BalanceTransactionList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil BalanceTransaction rows")
	}
	*l = BalanceTransactionList{}
	for rows.Next() {
		var item BalanceTransaction
		err := rows.Scan(&item.ID, &item.FromBalanceID, &item.ToBalanceID, &item.Amount, &item.CurrencyID,
			&item.TransactionType, &item.TransactionMethod, &item.SourceType, &item.SourceID,
			&item.Description, &item.CreatedBy, &item.CreatedAt, &item.VoidedAt,
			&item.FromEntityName, &item.ToEntityName)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

// Balance Transactions

func (bt *BalanceTransaction) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := bt.CreateWithTx(tx); err != nil {
		return err
	}

	return tx.Commit()
}

// CreateTwoWay records both the charge (incurrence) and payment (settlement)
// legs in a single DB transaction. Used for expense payments where the expense
// is both incurred and paid simultaneously, so both balances net to 0.
// bt is treated as the payment leg; the reverse charge leg is derived from it.
func (bt *BalanceTransaction) CreateTwoWay() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	charge := *bt
	charge.FromBalanceID, charge.ToBalanceID = bt.ToBalanceID, bt.FromBalanceID
	charge.TransactionType = "charge"
	pairTime := DateNow()
	charge.CreatedAt = pairTime
	bt.CreatedAt = pairTime
	if err := charge.CreateWithTx(tx); err != nil {
		return err
	}

	if err := bt.CreateWithTx(tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (bt *BalanceTransaction) CreateWithTx(tx *sql.Tx) error {
	bt.ID = uuid.Must(uuid.NewV7()).String()
	if bt.CreatedAt == "" {
		bt.CreatedAt = DateNow()
	}
	if bt.TransactionType == "" {
		bt.TransactionType = "payment"
	}
	if bt.TransactionMethod == "" {
		bt.TransactionMethod = "cash"
	}
	bt.VoidedAt = ""

	_, err := tx.Exec(`INSERT INTO balance_transactions (`+balanceTransactionColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		bt.ID, bt.FromBalanceID, bt.ToBalanceID, bt.Amount, bt.CurrencyID,
		bt.TransactionType, bt.TransactionMethod, bt.SourceType, bt.SourceID,
		bt.Description, bt.CreatedBy, bt.CreatedAt, bt.VoidedAt)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}

	if err := RecalculateBalancesWithTx(tx, bt.FromBalanceID, bt.ToBalanceID); err != nil {
		return fmt.Errorf("recalculate balances: %w", err)
	}

	return nil
}

// Delete voids the transaction and rebuilds affected balance projections from
// active transactions. If the transaction was created via CreateTwoWay (an
// expense payment, where a
// charge and a payment leg were inserted together), the paired leg is
// detected via matching created_at + reverse direction + same amount and
// voided atomically as well.
func (bt *BalanceTransaction) Delete() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Load the row we're deleting so we know what to reverse.
	var orig BalanceTransaction
	err = tx.QueryRow(`SELECT `+balanceTransactionColumns+` FROM balance_transactions WHERE id = ? AND voided_at = ''`, bt.ID).
		Scan(&orig.ID, &orig.FromBalanceID, &orig.ToBalanceID, &orig.Amount, &orig.CurrencyID,
			&orig.TransactionType, &orig.TransactionMethod, &orig.SourceType, &orig.SourceID,
			&orig.Description, &orig.CreatedBy, &orig.CreatedAt, &orig.VoidedAt)
	if err == sql.ErrNoRows {
		return ErrNotFound
	} else if err != nil {
		return err
	}

	voidedAt := DateNow()
	if err := orig.voidWithTx(tx, voidedAt); err != nil {
		return err
	}
	affected := []string{orig.FromBalanceID, orig.ToBalanceID}

	// Look for a paired leg (CreateTwoWay): same created_at, same amount,
	// reversed from/to. Different transaction_type (charge vs payment).
	var pair BalanceTransaction
	err = tx.QueryRow(`SELECT `+balanceTransactionColumns+` FROM balance_transactions
		WHERE id != ? AND from_balance_id = ? AND to_balance_id = ?
		AND amount = ? AND created_at = ? AND transaction_type IN ('charge','payment')
		AND voided_at = ''`,
		orig.ID, orig.ToBalanceID, orig.FromBalanceID, orig.Amount, orig.CreatedAt).
		Scan(&pair.ID, &pair.FromBalanceID, &pair.ToBalanceID, &pair.Amount, &pair.CurrencyID,
			&pair.TransactionType, &pair.TransactionMethod, &pair.SourceType, &pair.SourceID,
			&pair.Description, &pair.CreatedBy, &pair.CreatedAt, &pair.VoidedAt)
	if err == nil {
		if err := pair.voidWithTx(tx, voidedAt); err != nil {
			return err
		}
		affected = append(affected, pair.FromBalanceID, pair.ToBalanceID)
	} else if err != sql.ErrNoRows {
		return err
	}

	if err := RecalculateBalancesWithTx(tx, affected...); err != nil {
		return err
	}

	return tx.Commit()
}

// voidWithTx marks the transaction inactive. Caller is responsible for
// rebuilding the affected balance projections before committing.
func (bt *BalanceTransaction) voidWithTx(tx *sql.Tx, voidedAt Date) error {
	res, err := tx.Exec(`UPDATE balance_transactions SET voided_at = ? WHERE id = ? AND voided_at = ''`, voidedAt, bt.ID)
	if err != nil {
		return fmt.Errorf("void transaction: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	bt.VoidedAt = voidedAt
	return nil
}

// voidAndRecalculateWithTx voids one transaction and rebuilds the cached
// balance fields from active balance_transactions rows.
func (bt *BalanceTransaction) voidAndRecalculateWithTx(tx *sql.Tx) error {
	if err := bt.voidWithTx(tx, DateNow()); err != nil {
		return err
	}
	return RecalculateBalancesWithTx(tx, bt.FromBalanceID, bt.ToBalanceID)
}

func RecalculateBalanceWithTx(tx *sql.Tx, balanceID string) error {
	var amount, totalIn, totalOut float64
	if err := tx.QueryRow(`
		SELECT
			COALESCE(SUM(CASE
				WHEN to_balance_id = ? THEN amount
				WHEN from_balance_id = ? THEN -amount
				ELSE 0
			END), 0),
			COALESCE(SUM(CASE
				WHEN to_balance_id = ? AND transaction_type NOT IN ('charge','adjustment') THEN amount
				ELSE 0
			END), 0),
			COALESCE(SUM(CASE
				WHEN from_balance_id = ? AND transaction_type NOT IN ('charge','adjustment') THEN amount
				ELSE 0
			END), 0)
		FROM balance_transactions
		WHERE voided_at = '' AND (from_balance_id = ? OR to_balance_id = ?)`,
		balanceID, balanceID, balanceID, balanceID, balanceID, balanceID,
	).Scan(&amount, &totalIn, &totalOut); err != nil {
		return err
	}

	res, err := tx.Exec(`UPDATE balances SET amount = ?, total_in = ?, total_out = ?, updated_at = ? WHERE id = ?`,
		amount, totalIn, totalOut, DateNow(), balanceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func RecalculateBalancesWithTx(tx *sql.Tx, balanceIDs ...string) error {
	seen := map[string]struct{}{}
	for _, id := range balanceIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if err := RecalculateBalanceWithTx(tx, id); err != nil {
			return err
		}
	}
	return nil
}

func (bt *BalanceTransactionList) GetByBalanceID(balanceID string) error {
	rows, err := RDB.Query(`SELECT bt.id, bt.from_balance_id, bt.to_balance_id, bt.amount, bt.currency_id,
		bt.transaction_type, bt.transaction_method, bt.source_type, bt.source_id,
		bt.description, bt.created_by, bt.created_at, bt.voided_at,
		fb.entity_name, tb.entity_name
		FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		JOIN balances tb ON tb.id = bt.to_balance_id
		WHERE bt.voided_at = '' AND (bt.from_balance_id = ? OR bt.to_balance_id = ?)
		ORDER BY bt.created_at DESC`, balanceID, balanceID)
	if err != nil {
		return err
	}
	defer rows.Close()

	if err := bt.ScanRows(rows); err != nil {
		return err
	}
	return nil
}

// GetEntityPayments returns every transaction between self and the given
// entity, regardless of direction. Includes payments, refunds, adjustments,
// and write-offs (both incoming entity-to-self and outgoing self-to-entity).
// Charge transactions (sourced from invoices) are excluded since they belong
// to the invoice listing, not the payments listing.
func (bt *BalanceTransactionList) GetEntityPayments(entityType, entityID string, params ListParams) (int, error) {
	baseFrom := ` FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		JOIN balances tb ON tb.id = bt.to_balance_id
		WHERE bt.voided_at = ''
		AND bt.transaction_type != 'charge'
		AND (
			(fb.entity_type = ? AND tb.entity_type = 'self')
			OR (fb.entity_type = 'self' AND tb.entity_type = ?)
		)`
	args := []any{entityType, entityType}
	if entityID != "" {
		baseFrom += ` AND (
			(fb.entity_type = ? AND fb.entity_id = ?)
			OR (tb.entity_type = ? AND tb.entity_id = ?)
		)`
		args = append(args, entityType, entityID, entityType, entityID)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*)"+baseFrom, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"amount":            "bt.amount",
		"currencyId":        "bt.currency_id",
		"transactionType":   "bt.transaction_type",
		"transactionMethod": "bt.transaction_method",
		"sourceType":        "bt.source_type",
		"createdAt":         "bt.created_at",
		"fromEntityName":    "fb.entity_name",
		"toEntityName":      "tb.entity_name",
	}, "bt.created_at DESC")
	query := `SELECT bt.id, bt.from_balance_id, bt.to_balance_id, bt.amount, bt.currency_id,
		bt.transaction_type, bt.transaction_method, bt.source_type, bt.source_id,
		bt.description, bt.created_by, bt.created_at, bt.voided_at,
		fb.entity_name, tb.entity_name` + baseFrom + order + params.PaginationClause()

	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if err := bt.ScanRows(rows); err != nil {
		return 0, err
	}
	return total, nil
}

func (bt *BalanceTransactionList) GetAll(params ListParams) (int, error) {
	baseFrom := ` FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		JOIN balances tb ON tb.id = bt.to_balance_id`
	where := " WHERE bt.voided_at = ''"
	var args []any
	if fc, fa := params.FilterClause("bt.description", "fb.entity_name", "tb.entity_name"); fc != "" {
		where += " AND " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*)"+baseFrom+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"amount":            "bt.amount",
		"currencyId":        "bt.currency_id",
		"transactionType":   "bt.transaction_type",
		"transactionMethod": "bt.transaction_method",
		"sourceType":        "bt.source_type",
		"createdAt":         "bt.created_at",
		"fromEntityName":    "fb.entity_name",
		"toEntityName":      "tb.entity_name",
	}, "bt.created_at DESC")
	query := `SELECT bt.id, bt.from_balance_id, bt.to_balance_id, bt.amount, bt.currency_id,
		bt.transaction_type, bt.transaction_method, bt.source_type, bt.source_id,
		bt.description, bt.created_by, bt.created_at, bt.voided_at,
		fb.entity_name, tb.entity_name` + baseFrom + where + order + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if err := bt.ScanRows(rows); err != nil {
		return 0, err
	}
	return total, nil
}
