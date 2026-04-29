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

const balanceTransactionColumnsNoId = `from_balance_id, to_balance_id, amount, currency_id, transaction_type, transaction_method, source_type, source_id, description, created_by, created_at`
const balanceTransactionColumns = `id, ` + balanceTransactionColumnsNoId

type BalanceTransactionList []BalanceTransaction

func (m *BalanceTransaction) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil BalanceTransaction row")
	}
	return row.Scan(&m.ID, &m.FromBalanceID, &m.ToBalanceID, &m.Amount, &m.CurrencyID,
		&m.TransactionType, &m.TransactionMethod, &m.SourceType, &m.SourceID,
		&m.Description, &m.CreatedBy, &m.CreatedAt,
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
			&item.Description, &item.CreatedBy, &item.CreatedAt,
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
	bt.CreatedAt = DateNow()
	if bt.TransactionType == "" {
		bt.TransactionType = "payment"
	}
	if bt.TransactionMethod == "" {
		bt.TransactionMethod = "cash"
	}

	_, err := tx.Exec(`INSERT INTO balance_transactions (`+balanceTransactionColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		bt.ID, bt.FromBalanceID, bt.ToBalanceID, bt.Amount, bt.CurrencyID,
		bt.TransactionType, bt.TransactionMethod, bt.SourceType, bt.SourceID,
		bt.Description, bt.CreatedBy, bt.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}

	now := DateNow()
	// total_in / total_out track real money flow only (payments, refunds,
	// write-offs). Charges and adjustments are bookkeeping entries and are
	// excluded from both sides.
	flowDelta := bt.Amount
	if bt.TransactionType == "charge" || bt.TransactionType == "adjustment" {
		flowDelta = 0
	}
	_, err = tx.Exec(`UPDATE balances SET amount = amount - ?, total_out = total_out + ?, updated_at = ? WHERE id = ?`,
		bt.Amount, flowDelta, now, bt.FromBalanceID)
	if err != nil {
		return fmt.Errorf("from balance: %w", err)
	}
	_, err = tx.Exec(`UPDATE balances SET amount = amount + ?, total_in = total_in + ?, updated_at = ? WHERE id = ?`,
		bt.Amount, flowDelta, now, bt.ToBalanceID)
	if err != nil {
		return fmt.Errorf("to balance: %w", err)
	}

	return nil
}

// Delete reverses the balance updates and removes the transaction. If the
// transaction was created via CreateTwoWay (an expense payment, where a
// charge and a payment leg were inserted together), the paired leg is
// detected via matching created_at + reverse direction + same amount and
// removed atomically as well.
func (bt *BalanceTransaction) Delete() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Load the row we're deleting so we know what to reverse.
	var orig BalanceTransaction
	err = tx.QueryRow(`SELECT `+balanceTransactionColumns+` FROM balance_transactions WHERE id = ?`, bt.ID).
		Scan(&orig.ID, &orig.FromBalanceID, &orig.ToBalanceID, &orig.Amount, &orig.CurrencyID,
			&orig.TransactionType, &orig.TransactionMethod, &orig.SourceType, &orig.SourceID,
			&orig.Description, &orig.CreatedBy, &orig.CreatedAt)
	if err == sql.ErrNoRows {
		return ErrNotFound
	} else if err != nil {
		return err
	}

	if err := orig.reverseAndDeleteWithTx(tx); err != nil {
		return err
	}

	// Look for a paired leg (CreateTwoWay): same created_at, same amount,
	// reversed from/to. Different transaction_type (charge vs payment).
	var pair BalanceTransaction
	err = tx.QueryRow(`SELECT `+balanceTransactionColumns+` FROM balance_transactions
		WHERE id != ? AND from_balance_id = ? AND to_balance_id = ?
		AND amount = ? AND created_at = ? AND transaction_type IN ('charge','payment')`,
		orig.ID, orig.ToBalanceID, orig.FromBalanceID, orig.Amount, orig.CreatedAt).
		Scan(&pair.ID, &pair.FromBalanceID, &pair.ToBalanceID, &pair.Amount, &pair.CurrencyID,
			&pair.TransactionType, &pair.TransactionMethod, &pair.SourceType, &pair.SourceID,
			&pair.Description, &pair.CreatedBy, &pair.CreatedAt)
	if err == nil {
		if err := pair.reverseAndDeleteWithTx(tx); err != nil {
			return err
		}
	} else if err != sql.ErrNoRows {
		return err
	}

	return tx.Commit()
}

// reverseAndDeleteWithTx reverses the balance updates this transaction made
// and removes the row. Caller is responsible for the surrounding tx.
func (bt *BalanceTransaction) reverseAndDeleteWithTx(tx *sql.Tx) error {
	now := DateNow()
	flowDelta := bt.Amount
	if bt.TransactionType == "charge" || bt.TransactionType == "adjustment" {
		flowDelta = 0
	}
	if _, err := tx.Exec(`UPDATE balances SET amount = amount + ?, total_out = total_out - ?, updated_at = ? WHERE id = ?`,
		bt.Amount, flowDelta, now, bt.FromBalanceID); err != nil {
		return fmt.Errorf("reverse from balance: %w", err)
	}
	if _, err := tx.Exec(`UPDATE balances SET amount = amount - ?, total_in = total_in - ?, updated_at = ? WHERE id = ?`,
		bt.Amount, flowDelta, now, bt.ToBalanceID); err != nil {
		return fmt.Errorf("reverse to balance: %w", err)
	}
	res, err := tx.Exec(`DELETE FROM balance_transactions WHERE id = ?`, bt.ID)
	if err != nil {
		return fmt.Errorf("delete transaction: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (bt *BalanceTransactionList) GetByBalanceID(balanceID string) error {
	rows, err := RDB.Query(`SELECT bt.id, bt.from_balance_id, bt.to_balance_id, bt.amount, bt.currency_id,
		bt.transaction_type, bt.transaction_method, bt.source_type, bt.source_id,
		bt.description, bt.created_by, bt.created_at,
		fb.entity_name, tb.entity_name
		FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		JOIN balances tb ON tb.id = bt.to_balance_id
		WHERE bt.from_balance_id = ? OR bt.to_balance_id = ?
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
		WHERE bt.transaction_type != 'charge'
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

	query := `SELECT bt.id, bt.from_balance_id, bt.to_balance_id, bt.amount, bt.currency_id,
		bt.transaction_type, bt.transaction_method, bt.source_type, bt.source_id,
		bt.description, bt.created_by, bt.created_at,
		fb.entity_name, tb.entity_name` + baseFrom + ` ORDER BY bt.created_at DESC` + params.PaginationClause()

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
	where := ""
	var args []any
	if fc, fa := params.FilterClause("bt.description", "fb.entity_name", "tb.entity_name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*)"+baseFrom+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT bt.id, bt.from_balance_id, bt.to_balance_id, bt.amount, bt.currency_id,
		bt.transaction_type, bt.transaction_method, bt.source_type, bt.source_id,
		bt.description, bt.created_by, bt.created_at,
		fb.entity_name, tb.entity_name` + baseFrom + where + ` ORDER BY bt.created_at DESC` + params.PaginationClause()
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
