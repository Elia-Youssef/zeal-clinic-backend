package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type BalanceTransaction struct {
	ID            string  `json:"id"`
	FromBalanceID string  `json:"fromBalanceId"`
	ToBalanceID   string  `json:"toBalanceId"`
	Amount        float64 `json:"amount"`
	CurrencyID    string  `json:"currencyId"`
	ExchangeRate  float64 `json:"exchangeRate"`
	Description   string  `json:"description"`
	CreatedBy     string  `json:"createdBy"`
	CreatedAt     Date    `json:"createdAt"`
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

const balanceTransactionColumnsNoId = `from_balance_id, to_balance_id, amount, currency_id, exchange_rate, description, created_by, created_at`
const balanceTransactionColumns = `id, ` + balanceTransactionColumnsNoId

type BalanceTransactionList []BalanceTransaction

func (m *BalanceTransaction) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil BalanceTransaction row")
	}
	return row.Scan(&m.ID, &m.FromBalanceID, &m.ToBalanceID, &m.Amount, &m.CurrencyID,
		&m.ExchangeRate, &m.Description, &m.CreatedBy, &m.CreatedAt,
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
			&item.ExchangeRate, &item.Description, &item.CreatedBy, &item.CreatedAt,
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

func (bt *BalanceTransaction) CreateWithTx(tx *sql.Tx) error {
	bt.ID = uuid.Must(uuid.NewV7()).String()
	bt.CreatedAt = DateNow()
	if bt.ExchangeRate == 0 {
		bt.ExchangeRate = 1.0
	}

	_, err := tx.Exec(`INSERT INTO balance_transactions (`+balanceTransactionColumns+`) VALUES (?,?,?,?,?,?,?,?,?)`,
		bt.ID, bt.FromBalanceID, bt.ToBalanceID, bt.Amount, bt.CurrencyID,
		bt.ExchangeRate, bt.Description, bt.CreatedBy, bt.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}

	now := DateNow()
	_, err = tx.Exec(`UPDATE balances SET amount = amount - ?, updated_at = ? WHERE id = ?`, bt.Amount, now, bt.FromBalanceID)
	if err != nil {
		return fmt.Errorf("from balance: %w", err)
	}
	_, err = tx.Exec(`UPDATE balances SET amount = amount + ?, updated_at = ? WHERE id = ?`, bt.Amount, now, bt.ToBalanceID)
	if err != nil {
		return fmt.Errorf("to balance: %w", err)
	}

	return nil
}

func (bt *BalanceTransaction) GetByBalanceID(balanceID string) ([]BalanceTransaction, error) {
	rows, err := DB.Query(`SELECT bt.id, bt.from_balance_id, bt.to_balance_id, bt.amount, bt.currency_id,
		bt.exchange_rate, bt.description, bt.created_by, bt.created_at,
		fb.entity_name, tb.entity_name
		FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		JOIN balances tb ON tb.id = bt.to_balance_id
		WHERE bt.from_balance_id = ? OR bt.to_balance_id = ?
		ORDER BY bt.created_at DESC`, balanceID, balanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list BalanceTransactionList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	return list, nil
}

func (bt *BalanceTransaction) GetClientPayments(patientID string) ([]BalanceTransaction, error) {
	query := `SELECT bt.id, bt.from_balance_id, bt.to_balance_id, bt.amount, bt.currency_id,
		bt.exchange_rate, bt.description, bt.created_by, bt.created_at,
		fb.entity_name, tb.entity_name
		FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		JOIN balances tb ON tb.id = bt.to_balance_id
		WHERE fb.entity_type = 'patient' AND tb.entity_type = 'self'`
	var args []interface{}
	if patientID != "" {
		query += ` AND fb.entity_id = ?`
		args = append(args, patientID)
	}
	query += ` ORDER BY bt.created_at DESC`

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list BalanceTransactionList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	return list, nil
}

func (bt *BalanceTransaction) GetSupplierPayments(supplierBalanceID string) ([]BalanceTransaction, error) {
	query := `SELECT bt.id, bt.from_balance_id, bt.to_balance_id, bt.amount, bt.currency_id,
		bt.exchange_rate, bt.description, bt.created_by, bt.created_at,
		fb.entity_name, tb.entity_name
		FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		JOIN balances tb ON tb.id = bt.to_balance_id
		WHERE fb.entity_type = 'self' AND tb.entity_type = 'supplier'`
	var args []interface{}
	if supplierBalanceID != "" {
		query += ` AND tb.id = ?`
		args = append(args, supplierBalanceID)
	}
	query += ` ORDER BY bt.created_at DESC`

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list BalanceTransactionList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	return list, nil
}

func (bt *BalanceTransaction) GetAll(limit int) ([]BalanceTransaction, error) {
	query := `SELECT bt.id, bt.from_balance_id, bt.to_balance_id, bt.amount, bt.currency_id,
		bt.exchange_rate, bt.description, bt.created_by, bt.created_at,
		fb.entity_name, tb.entity_name
		FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		JOIN balances tb ON tb.id = bt.to_balance_id
		ORDER BY bt.created_at DESC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list BalanceTransactionList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	return list, nil
}
